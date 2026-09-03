package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCurrentBrowserStatusChecksEndpointWithoutAttachingOrCreatingATab(t *testing.T) {
	requests := 0
	var webSocketURL string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/json/version" {
			t.Fatalf("unexpected passive status request: %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(map[string]string{"webSocketDebuggerUrl": webSocketURL})
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	webSocketURL = "ws://127.0.0.1:" + strconv.Itoa(port) + "/devtools/browser/synthetic-token"
	userDataDir := t.TempDir()
	if err := os.Chmod(userDataDir, 0o700); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDataDir, devToolsActivePortFilename), []byte(strconv.Itoa(port)+"\n/devtools/browser/synthetic-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "synthetic-browser")
	if err := os.WriteFile(executable, []byte("synthetic browser fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	native := NewNativeCurrentBrowser()
	native.getenv = func(name string) string {
		switch name {
		case "COUPANGCTL_BROWSER_PATH":
			return executable
		case "COUPANGCTL_CURRENT_BROWSER_USER_DATA_DIR":
			return userDataDir
		default:
			return ""
		}
	}

	got, err := native.CurrentBrowserStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != core.CurrentBrowserStatusSchemaVersion || got.State != core.CurrentBrowserEndpointAvailable || !got.EndpointAvailable || got.ConnectionApprovalVerified {
		t.Fatalf("unexpected current-browser status: %#v", got)
	}
	if requests != 1 {
		t.Fatalf("passive status HTTP requests = %d, want 1", requests)
	}
}

func TestReadDevToolsActivePortAcceptsOnlyPrivateLoopbackEndpoint(t *testing.T) {
	userDataDir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(userDataDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(userDataDir, devToolsActivePortFilename)
	if err := os.WriteFile(path, []byte("49152\n/devtools/browser/synthetic-token_123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	baseURL, webSocketURL, err := readDevToolsActivePort(userDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "http://127.0.0.1:49152" || webSocketURL != "ws://127.0.0.1:49152/devtools/browser/synthetic-token_123" {
		t.Fatalf("unexpected endpoints base=%q websocket=%q", baseURL, webSocketURL)
	}

	for _, content := range []string{
		"0\n/devtools/browser/token\n",
		"65536\n/devtools/browser/token\n",
		"49152\n/devtools/page/token\n",
		"49152\n/devtools/browser/../page/token\n",
		"49152\n/devtools/browser/token?query=1\n",
		"49152\n/devtools/browser/token\nextra\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readDevToolsActivePort(userDataDir); !errors.Is(err, ErrCurrentBrowserUnavailable) {
			t.Fatalf("content %q error = %v, want ErrCurrentBrowserUnavailable", content, err)
		}
	}
}

func TestReadDevToolsActivePortRejectsSymlinkAndWritableUnixFile(t *testing.T) {
	userDataDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("49152\n/devtools/browser/synthetic-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(userDataDir, devToolsActivePortFilename)
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDevToolsActivePort(userDataDir); !errors.Is(err, ErrCurrentBrowserUnavailable) {
		t.Fatalf("symlink error = %v, want ErrCurrentBrowserUnavailable", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("49152\n/devtools/browser/synthetic-token\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDevToolsActivePort(userDataDir); !errors.Is(err, ErrCurrentBrowserUnavailable) {
		t.Fatalf("writable file error = %v, want ErrCurrentBrowserUnavailable", err)
	}
}

func TestReadDevToolsActivePortRejectsPublicUnixProfileDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission test")
	}
	userDataDir := t.TempDir()
	path := filepath.Join(userDataDir, devToolsActivePortFilename)
	if err := os.WriteFile(path, []byte("49152\n/devtools/browser/synthetic-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(userDataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDevToolsActivePort(userDataDir); !errors.Is(err, ErrCurrentBrowserUnavailable) {
		t.Fatalf("public profile directory error = %v, want ErrCurrentBrowserUnavailable", err)
	}
}

func TestReadDevToolsActivePortRejectsSymlinkProfileDirectory(t *testing.T) {
	actual := t.TempDir()
	parent := t.TempDir()
	linked := filepath.Join(parent, "linked-profile")
	if err := os.Symlink(actual, linked); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, _, err := readDevToolsActivePort(linked); !errors.Is(err, ErrCurrentBrowserUnavailable) {
		t.Fatalf("symlink profile directory error = %v, want ErrCurrentBrowserUnavailable", err)
	}
}

func TestCurrentBrowserUserDataDirFollowsBrowserFamilyAndOS(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "LOCALAPPDATA":
			return `C:\Users\synthetic\AppData\Local`
		case "XDG_CONFIG_HOME":
			return "/synthetic/config"
		default:
			return ""
		}
	}
	for _, test := range []struct {
		goos       string
		executable string
		home       string
		want       string
	}{
		{"darwin", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Users/synthetic", "/Users/synthetic/Library/Application Support/Google/Chrome"},
		{"darwin", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "/Users/synthetic", "/Users/synthetic/Library/Application Support/Microsoft Edge"},
		{"linux", "/usr/bin/google-chrome-stable", "/home/synthetic", "/synthetic/config/google-chrome"},
		{"linux", "/usr/bin/chromium", "/home/synthetic", "/synthetic/config/chromium"},
		{"windows", `C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Users\synthetic`, `C:\Users\synthetic\AppData\Local\Google\Chrome\User Data`},
	} {
		got, err := currentBrowserUserDataDir(test.goos, test.executable, test.home, getenv)
		if err != nil || got != filepath.Clean(test.want) {
			t.Fatalf("currentBrowserUserDataDir(%q, %q) = %q, %v; want %q", test.goos, test.executable, got, err, filepath.Clean(test.want))
		}
	}
	if _, err := currentBrowserUserDataDir("darwin", "/Applications/Unknown.app/unknown", "/Users/synthetic", getenv); !errors.Is(err, ErrCurrentBrowserUnavailable) {
		t.Fatalf("unknown browser error = %v, want ErrCurrentBrowserUnavailable", err)
	}
}

type closeTrackingProtocol struct {
	calls  []string
	closed bool
}

func (protocol *closeTrackingProtocol) Call(_ context.Context, method string, _ any, _ any) error {
	protocol.calls = append(protocol.calls, method)
	return nil
}

func (protocol *closeTrackingProtocol) Close() error {
	protocol.closed = true
	return nil
}

func TestAttachedChromeSessionDisconnectsWithoutClosingUsersBrowser(t *testing.T) {
	protocol := &closeTrackingProtocol{}
	session := &chromeSession{browser: protocol, httpClient: &http.Client{}, ownsBrowser: false}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if !protocol.closed || len(protocol.calls) != 0 {
		t.Fatalf("attached close state = closed:%t calls:%#v", protocol.closed, protocol.calls)
	}
}

func TestManagedChromeSessionClosesOwnedBrowser(t *testing.T) {
	protocol := &closeTrackingProtocol{}
	session := &chromeSession{browser: protocol, httpClient: &http.Client{}, ownsBrowser: true}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if !protocol.closed || len(protocol.calls) != 1 || protocol.calls[0] != "Browser.close" {
		t.Fatalf("owned close state = closed:%t calls:%#v", protocol.closed, protocol.calls)
	}
}
