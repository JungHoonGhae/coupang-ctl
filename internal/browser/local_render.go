package browser

import (
	"context"
	"errors"
	"fmt"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

const localPageRenderTimeout = 20 * time.Second
const maxLocalPagePNGBytes = 25 << 20

var ErrLocalPageRenderFailed = core.WithErrorCode("recap_image_render_failed", errors.New("local page render failed"))

// LocalPageRenderer uses the installed Camoufox engine in a disposable profile.
// It does not connect to the authenticated Camofox server or a user's browser.
type LocalPageRenderer struct {
	executable func() (string, error)
	command    func(context.Context, string, ...string) *exec.Cmd
}

func NewLocalPageRenderer() *LocalPageRenderer {
	return &LocalPageRenderer{executable: localPageEngine, command: exec.CommandContext}
}

func (r *LocalPageRenderer) RenderPNG(ctx context.Context, htmlPath, outputPath string, width, height int) error {
	if !filepath.IsAbs(htmlPath) || !filepath.IsAbs(outputPath) || filepath.Ext(outputPath) != ".png" || width < 320 || width > 4096 || height < 320 || height > 4096 {
		return errors.New("invalid local page render request")
	}
	info, err := os.Stat(htmlPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("local page render source unavailable")
	}
	// Reject an existing target before launching, then use O_EXCL again when
	// committing the validated image. A stale image can never count as success.
	if _, err := os.Lstat(outputPath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("local page render output already exists or is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	executable, err := r.executable()
	if err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(filepath.Dir(outputPath), "camofox-local-render-")
	if err != nil {
		return errors.New("create local page browser profile")
	}
	defer os.RemoveAll(workspace)
	profileDirectory := filepath.Join(workspace, "profile")
	if os.Chmod(workspace, 0o700) != nil || os.Mkdir(profileDirectory, 0o700) != nil {
		return errors.New("secure local page browser profile")
	}
	if os.WriteFile(filepath.Join(profileDirectory, "user.js"), []byte(localPagePreferences), 0o600) != nil {
		return errors.New("configure local page browser profile")
	}
	renderedPath := filepath.Join(workspace, "render.png")
	renderContext, cancel := context.WithTimeout(ctx, localPageRenderTimeout)
	defer cancel()
	command := r.command(renderContext, executable, localPageScreenshotArguments(htmlPath, renderedPath, profileDirectory, width, height)...)
	// Do not inherit debugging, logging, proxy, or browser-profile overrides.
	command.Env = []string{"MOZ_HEADLESS=1", "MOZ_NO_REMOTE=1", "MOZ_CRASHREPORTER_DISABLE=1"}
	for _, key := range []string{"HOME", "PATH", "LANG", "USER", "LOGNAME", "SYSTEMROOT"} {
		if value, ok := os.LookupEnv(key); ok {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	command.Env = append(command.Env, "TMPDIR="+workspace, "TMP="+workspace, "TEMP="+workspace)
	command.WaitDelay = 2 * time.Second
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return ErrLocalPageRenderFailed
	}
	if renderContext.Err() != nil {
		return ErrLocalPageRenderFailed
	}
	return commitLocalPagePNG(renderedPath, outputPath, width, height)
}

func localPageEngine() (string, error) {
	paths, err := platform.DefaultPaths()
	if err != nil {
		return "", ErrCamofoxUnavailable
	}
	cfg, err := ReadCamofoxConfig(paths.StateDir)
	if err != nil {
		return "", ErrCamofoxUnavailable
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
		return "", ErrCamofoxUnavailable
	}
	executable := filepath.Join(cfg.EngineDir, filepath.FromSlash(relative))
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return "", ErrCamofoxUnavailable
	}
	version, err := os.Stat(filepath.Join(cfg.EngineDir, "version.json"))
	if err != nil || !version.Mode().IsRegular() {
		return "", ErrCamofoxUnavailable
	}
	return executable, nil
}

func commitLocalPagePNG(source, target string, width, height int) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxLocalPagePNGBytes {
		return ErrLocalPageRenderFailed
	}
	f, err := os.Open(source)
	if err != nil {
		return ErrLocalPageRenderFailed
	}
	defer f.Close()
	config, err := png.DecodeConfig(io.LimitReader(f, maxLocalPagePNGBytes+1))
	if err != nil || config.Width != width || config.Height != height {
		return ErrLocalPageRenderFailed
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return ErrLocalPageRenderFailed
	}
	// A valid header is not enough: reject incomplete or corrupt pixel data.
	if _, err := png.Decode(io.LimitReader(f, maxLocalPagePNGBytes+1)); err != nil {
		return ErrLocalPageRenderFailed
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return ErrLocalPageRenderFailed
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ErrLocalPageRenderFailed
	}
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(target)
		}
	}()
	n, err := io.Copy(out, io.LimitReader(f, maxLocalPagePNGBytes+1))
	if err != nil || n != info.Size() || out.Sync() != nil || out.Close() != nil {
		return ErrLocalPageRenderFailed
	}
	committed = true
	return nil
}

func localPageScreenshotArguments(htmlPath, outputPath, profileDirectory string, width, height int) []string {
	target := (&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String()
	return []string{
		"--headless", "--new-instance",
		"--profile", profileDirectory,
		"--window-size", fmt.Sprintf("%d,%d", width, height),
		"--screenshot", outputPath,
		target,
	}
}

// Only generated, self-contained recap HTML is rendered. CSP on that document
// permits embedded fonts/images/styles; this profile also disables scripts,
// prefetching, and direct HTTP(S) connections. It is discarded after every run.
const localPagePreferences = `user_pref("javascript.enabled", false);
user_pref("layout.css.devPixelsPerPx", "1.0");
user_pref("network.proxy.type", 1);
user_pref("network.proxy.http", "127.0.0.1");
user_pref("network.proxy.http_port", 9);
user_pref("network.proxy.ssl", "127.0.0.1");
user_pref("network.proxy.ssl_port", 9);
user_pref("network.proxy.no_proxies_on", "");
user_pref("network.proxy.failover_direct", false);
user_pref("network.dns.disablePrefetch", true);
user_pref("network.prefetch-next", false);
user_pref("network.http.speculative-parallel-limit", 0);
user_pref("toolkit.telemetry.enabled", false);
user_pref("datareporting.healthreport.uploadEnabled", false);
user_pref("datareporting.policy.dataSubmissionEnabled", false);
`
