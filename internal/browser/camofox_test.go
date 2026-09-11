package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func TestCamofoxRejectsPhoneLoginBeforeBrowserAcquisition(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("unsupported phone mode acquired a browser")
		return nil, nil
	}}
	err := c.Login(context.Background(), core.LoginRequest{Mode: core.LoginModePhone, Phone: "01000000000", ReadOTP: func(context.Context) (string, error) { t.Fatal("unsupported phone mode requested OTP"); return "", nil }})
	if !errors.Is(err, ErrCamofoxUnsupported) {
		t.Fatalf("unsupported phone mode: %v", err)
	}
	for _, kind := range []string{"phone", "otp"} {
		if _, err := consumeCamofoxOutput(context.Background(), strings.NewReader("COUPANGCTL_INPUT "+kind+"\n"), nil); !errors.Is(err, ErrDocumentProtocol) {
			t.Fatal("runtime requested unsupported private input")
		}
	}
}

func TestCamofoxQRImageIsDecodedOnlyForExplicitPresenter(t *testing.T) {
	const syntheticURL = "https://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic-test-only"
	for _, tc := range []struct {
		payload, code string
		valid         bool
	}{
		{syntheticURL, "42", true},
		{"https://example.invalid/not-login", "42", false},
		{syntheticURL, "invalid", false},
	} {
		matrix, err := qrcode.NewQRCodeWriter().Encode(tc.payload, gozxing.BarcodeFormat_QR_CODE, 320, 320, nil)
		if err != nil {
			t.Fatal("synthetic QR encoding failed")
		}
		var pngBytes bytes.Buffer
		var img image.Image = matrix
		if tc.valid {
			// Wide desktop viewport, with a small synthetic QR in its login area.
			viewport := image.NewRGBA(image.Rect(0, 0, 2319, 1091))
			draw.Draw(viewport, viewport.Bounds(), image.White, image.Point{}, draw.Src)
			draw.Draw(viewport, image.Rect(1000, 100, 1320, 420), matrix, image.Point{}, draw.Src)
			img = viewport
		}
		if png.Encode(&pngBytes, img) != nil {
			t.Fatal("synthetic PNG encoding failed")
		}
		value, _ := json.Marshal(map[string]string{"png": base64.StdEncoding.EncodeToString(pngBytes.Bytes()), "approvalCode": tc.code})
		stream := "COUPANGCTL_QR_IMAGE " + string(value) + "\nCOUPANGCTL_RESULT {\"status\":\"ok\"}\n"
		if _, err := consumeCamofoxOutput(context.Background(), strings.NewReader(stream), nil); err == nil {
			t.Fatal("unsolicited image accepted")
		}
		calls := 0
		data, err := consumeCamofoxOutput(context.Background(), strings.NewReader(stream), func(_ context.Context, link core.QRLoginLink) error {
			calls++
			if link.URL != syntheticURL || link.ApprovalCode != "42" {
				t.Fatal("incorrect decoded synthetic link")
			}
			return nil
		})
		if tc.valid {
			if err != nil || calls != 1 || string(data) != "{\"status\":\"ok\"}" {
				t.Fatal("valid QR image was not privately presented")
			}
		} else if err == nil || calls != 0 {
			t.Fatal("unsafe QR accepted")
		}
	}
}

func TestCamofoxQRImageRejectsMalformedOrOversizedInput(t *testing.T) {
	for _, value := range []string{"", "invalid base64", strings.Repeat("A", 700001), base64.StdEncoding.EncodeToString([]byte("not a PNG"))} {
		if _, err := decodeCamofoxQRImage(value); !errors.Is(err, ErrQRLinkUnavailable) {
			t.Fatal("invalid image accepted")
		}
	}
}

func TestCamofoxSearchPreservesEvidenceAndRejectsFalseSuccess(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`{"result":{"status":"access_denied"}}`, ErrBrowserAccessDenied},
		{`{"result":{"status":"authentication_required"}}`, core.ErrAuthenticationRequired},
		{`{"result":{"status":"ok","search":{"items":[]}}}`, ErrDocumentProtocol},
		{`{"result":{"status":"structured_data_missing"}}`, ErrStructuredProductDataMissing},
		{`{"error":"filter_unavailable"}`, ErrSearchFacetUnavailable},
		{`{"result":{"status":"ok","search":{"items":[],"no_results":true}}}`, nil},
	} {
		c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(tc.body), nil
		}}
		data, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
		if !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
		if err == nil {
			var d struct {
				Coverage core.ProductCoverage `json:"coverage"`
			}
			if json.Unmarshal(data, &d) != nil || d.Coverage.Source != "camofox_search_document" {
				t.Fatal("incorrect transport provenance")
			}
		}
	}
}

func TestCamofoxLoginKeepsOneProfileAndVerifiesAfterPersistence(t *testing.T) {
	for _, linkMode := range []bool{false, true} {
		var modes []string
		c := &Camofox{run: func(_ context.Context, script, mode string, present core.QRLinkPresenter) ([]byte, error) {
			modes = append(modes, mode)
			if len(modes) == 1 {
				return []byte(`{"status":"authentication_required"}`), nil
			}
			if len(modes) == 2 {
				if (present != nil) != linkMode {
					t.Fatal("presenter not confined to requested link mode")
				}
				return []byte(`{"status":"ok"}`), nil
			}
			return []byte(`{"status":"ok","authenticated":true}`), nil
		}}
		r := core.LoginRequest{Mode: core.LoginModeQR}
		want := "orders,login,orders"
		if linkMode {
			r.PresentQRLink = func(context.Context, core.QRLoginLink) error { return nil }
			want = "orders,login_link,orders"
		}
		if err := c.Login(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		if strings.Join(modes, ",") != want {
			t.Fatalf("wrong lifecycle: %v", modes)
		}
	}
}

func TestCamofoxVerifiedOrBlockedLoginDoesNotOpenAWindow(t *testing.T) {
	for _, status := range []string{"ok", "access_denied"} {
		calls := 0
		c := &Camofox{run: func(_ context.Context, _, mode string, _ core.QRLinkPresenter) ([]byte, error) {
			calls++
			if mode != "orders" {
				t.Fatal("unexpected interactive acquisition")
			}
			if status == "ok" {
				return []byte(`{"status":"ok","authenticated":true}`), nil
			}
			return []byte(`{"status":"access_denied"}`), nil
		}}
		err := c.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR})
		if (status == "ok") != (err == nil) || calls != 1 {
			t.Fatal("incorrect authenticated or blocked handling")
		}
	}
}

func TestCamofoxQRMaterialOnlyReachesExplicitPresenter(t *testing.T) {
	const event = "COUPANGCTL_QR {\"url\":\"https://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic\",\"approvalCode\":\"42\"}\n"
	const result = "COUPANGCTL_RESULT {\"status\":\"ok\"}\n"
	for _, input := range []string{event + result, event + event + result, strings.Replace(event, "login.coupang.com", "example.invalid", 1) + result} {
		if _, err := consumeCamofoxOutput(context.Background(), strings.NewReader(input), nil); err == nil {
			t.Fatal("unsolicited QR accepted")
		}
	}
	calls := 0
	data, err := consumeCamofoxOutput(context.Background(), strings.NewReader(event+result), func(context.Context, core.QRLoginLink) error { calls++; return nil })
	if err != nil || calls != 1 || strings.Contains(string(data), "synthetic") {
		t.Fatal("QR escaped presenter or result missing")
	}
	for _, input := range []string{result + result, result + event, "private upstream log\n" + result} {
		if _, err := consumeCamofoxOutput(context.Background(), strings.NewReader(input), func(context.Context, core.QRLoginLink) error { return nil }); err == nil {
			t.Fatal("invalid protocol accepted")
		}
	}
}

func TestCamofoxSetupDoesNotSwitchTheProfileBehindAnExistingLedger(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"node", "server.js", "version.json", "plugins/persistence/index.js", "mcp/lib/cookies.mjs"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := CamofoxConfig{SchemaVersion: 1, Node: filepath.Join(dir, "node"), Server: filepath.Join(dir, "server.js"), EngineDir: dir, UserID: "synthetic", DefaultBrowser: true}
	if err := SaveCamofoxConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "camofox.json"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("configuration file missing")
	}
	// Windows FileMode does not expose DACLs; 0600 is a POSIX assertion.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("configuration not private")
	}
	cfg.UserID = "different-synthetic"
	if err := SaveCamofoxConfig(dir, cfg); err == nil {
		t.Fatal("profile identity silently switched")
	}
	stored, err := ReadCamofoxConfig(dir)
	if err != nil || stored.UserID != "synthetic" || !stored.DefaultBrowser {
		t.Fatal("original selection damaged")
	}
}

func TestCamofoxUnsupportedAndInvalidRequestsNeverAcquire(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("unexpected browser acquisition")
		return nil, nil
	}}
	if _, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{}); err == nil {
		t.Fatal("invalid query accepted")
	}
	if _, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{CategoryID: "123", Query: "synthetic"}); err == nil {
		t.Fatal("ambiguous category and query accepted")
	}
	if _, err := c.AddProductToCart(context.Background(), core.CartAddRequest{}); !errors.Is(err, ErrCamofoxUnsupported) {
		t.Fatal(err)
	}
	if _, err := c.FetchProductInspection(context.Background(), core.ProductInspectRequest{}); err == nil {
		t.Fatal(err)
	}
}

func TestCamofoxConfigurationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	for _, data := range []string{`{}`, `{"schema_version":1,"node":"relative"}`, `{"schema_version":1,"node":"/node","server":"/server","engine_dir":"/engine","user_id":"synthetic","token":"do-not-read"}`} {
		if err := os.WriteFile(filepath.Join(dir, "camofox.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadCamofoxConfig(dir); !errors.Is(err, ErrCamofoxUnavailable) {
			t.Fatal("invalid configuration accepted")
		}
	}
}
