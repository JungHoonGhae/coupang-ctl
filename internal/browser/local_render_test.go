package browser

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalPageScreenshotArgumentsStayLocalAndBounded(t *testing.T) {
	arguments := localPageScreenshotArguments(
		"/private/tmp/synthetic share.html",
		"/private/tmp/synthetic share.png",
		"/private/tmp/synthetic profile",
		1080,
		1350,
	)
	want := []string{
		"--headless", "--new-instance", "--profile", "/private/tmp/synthetic profile",
		"--window-size", "1080,1350", "--screenshot", "/private/tmp/synthetic share.png",
		"file:///private/tmp/synthetic%20share.html",
	}
	if !reflect.DeepEqual(arguments, want) {
		t.Fatalf("screenshot arguments = %q, want %q", arguments, want)
	}
	joined := strings.Join(arguments, "\n")
	for _, forbidden := range []string{"http://", "https://", "--remote-debugging", "--user-data-dir", "--headless=new"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("local screenshot arguments contain %q: %s", forbidden, joined)
		}
	}
}

func TestLocalPageRendererValidatesRequestsBeforeLaunching(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "card.html")
	if err := os.WriteFile(source, []byte("<!doctype html><title>Synthetic card</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewLocalPageRenderer()
	r.executable = func() (string, error) { t.Fatal("invalid request launched a browser"); return "", nil }
	for _, tc := range []struct {
		source, output string
		width, height  int
	}{
		{"relative.html", filepath.Join(dir, "out.png"), 320, 400},
		{source, "relative.png", 320, 400},
		{source, filepath.Join(dir, "out.jpg"), 320, 400},
		{source, filepath.Join(dir, "out.png"), 319, 400},
		{source, filepath.Join(dir, "out.png"), 320, 4097},
		{dir, filepath.Join(dir, "out.png"), 320, 400},
		{filepath.Join(dir, "missing.html"), filepath.Join(dir, "out.png"), 320, 400},
	} {
		if err := r.RenderPNG(context.Background(), tc.source, tc.output, tc.width, tc.height); err == nil {
			t.Fatalf("invalid render request accepted: %#v", tc)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.RenderPNG(ctx, source, filepath.Join(dir, "out.png"), 320, 400); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestLocalPageRendererCommitsOnlyCompleteSuccessfulImage(t *testing.T) {
	for _, mode := range []string{"ok", "failure", "missing", "corrupt", "wrong_size", "truncated", "cancel", "output_race"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			source, target := filepath.Join(dir, "card.html"), filepath.Join(dir, "card.png")
			if err := os.WriteFile(source, []byte("<!doctype html><title>Synthetic card</title>"), 0o600); err != nil {
				t.Fatal(err)
			}
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			var workspace string
			t.Setenv("COUPANGCTL_RENDER_SYNTHETIC_SECRET", "do-not-inherit")
			r := &LocalPageRenderer{
				executable: func() (string, error) { return "/synthetic/camoufox", nil },
				command: func(ctx context.Context, executable string, args ...string) *exec.Cmd {
					if executable != "/synthetic/camoufox" {
						t.Fatalf("wrong engine: %s", executable)
					}
					profile := args[3]
					workspace = filepath.Dir(profile)
					for _, path := range []string{workspace, profile} {
						if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o700 {
							t.Fatal("temporary renderer workspace is not private")
						}
					}
					prefs, err := os.ReadFile(filepath.Join(profile, "user.js"))
					if err != nil || string(prefs) != localPagePreferences {
						t.Fatal("missing isolated renderer preferences")
					}
					if info, err := os.Stat(filepath.Join(profile, "user.js")); err != nil || info.Mode().Perm() != 0o600 {
						t.Fatal("renderer preferences are not private")
					}
					if mode == "output_race" {
						if err := os.WriteFile(target, []byte("preserve synthetic output"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					return exec.CommandContext(ctx, bin, append([]string{"-test.run=^TestLocalPageRenderHelper$", "--", mode}, args...)...)
				},
			}
			ctx := context.Background()
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancel()
			}
			err = r.RenderPNG(ctx, source, target, 320, 400)
			if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("renderer workspace was not removed")
			}
			if mode == "ok" {
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(target)
				if err != nil || info.Size() < 1 || info.Mode().Perm() != 0o600 {
					t.Fatal("renderer did not create a private PNG")
				}
				// A second run must not even launch, regardless of existing pixels.
				r.executable = func() (string, error) { t.Fatal("existing output caused a launch"); return "", nil }
				if err := r.RenderPNG(ctx, source, target, 320, 400); err == nil {
					t.Fatal("existing image was overwritten")
				}
			} else {
				if !errors.Is(err, ErrLocalPageRenderFailed) {
					t.Fatalf("failed renderer reported %v", err)
				}
				if mode == "output_race" {
					data, err := os.ReadFile(target)
					if err != nil || string(data) != "preserve synthetic output" {
						t.Fatal("output created during rendering was overwritten")
					}
				} else if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed renderer left a final image")
				}
			}
		})
	}
}

// This subprocess replaces only the engine executable, exercising the real
// command lifecycle, environment, PNG validation, commit, and cleanup paths.
func TestLocalPageRenderHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	mode, args := os.Args[separator+1], os.Args[separator+2:]
	if os.Getenv("COUPANGCTL_RENDER_SYNTHETIC_SECRET") != "" || os.Getenv("MOZ_HEADLESS") != "1" {
		os.Exit(7)
	}
	output := args[7]
	if mode == "missing" {
		os.Exit(0)
	}
	if mode == "corrupt" {
		_ = os.WriteFile(output, []byte("not a PNG"), 0o600)
		os.Exit(0)
	}
	width := 320
	if mode == "wrong_size" {
		width = 321
	}
	f, err := os.Create(output)
	if err != nil {
		os.Exit(8)
	}
	if png.Encode(f, image.NewRGBA(image.Rect(0, 0, width, 400))) != nil || f.Close() != nil {
		os.Exit(9)
	}
	if mode == "truncated" {
		_ = os.Truncate(output, 40)
	}
	if mode == "failure" {
		os.Exit(3)
	}
	if mode == "cancel" {
		// A stable image is not success while the engine is still running.
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

func TestLocalPageEngineUsesOnlyConfiguredCamoufox(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", dir)
	t.Setenv("COUPANGCTL_BROWSER_PATH", "/synthetic/chrome")
	if _, err := localPageEngine(); !errors.Is(err, ErrCamofoxUnavailable) {
		t.Fatal("missing Camofox configuration allowed fallback")
	}
	cfg := CamofoxConfig{SchemaVersion: 1, Node: filepath.Join(dir, "node"), Server: filepath.Join(dir, "server.js"), EngineDir: filepath.Join(dir, "engine"), UserID: "synthetic-render"}
	b, err := json.Marshal(cfg)
	if err != nil || os.WriteFile(filepath.Join(dir, "camofox.json"), b, 0o600) != nil {
		t.Fatal("write synthetic config")
	}
	var relative string
	switch runtime.GOOS {
	case "darwin":
		relative = "Camoufox.app/Contents/MacOS/camoufox"
	case "linux":
		relative = "camoufox-bin"
	case "windows":
		relative = "camoufox.exe"
	default:
		t.Skip("no Camoufox engine for this platform")
	}
	want := filepath.Join(cfg.EngineDir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(want), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("synthetic executable marker, never executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := localPageEngine(); !errors.Is(err, ErrCamofoxUnavailable) {
		t.Fatal("incomplete engine accepted")
	}
	if err := os.WriteFile(filepath.Join(cfg.EngineDir, "version.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := localPageEngine(); err != nil || got != want {
		t.Fatalf("configured engine = %q, %v", got, err)
	}
}
