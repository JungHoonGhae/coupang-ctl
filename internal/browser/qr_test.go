package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

type qrCaptureProtocol struct {
	clip map[string]any
}

type qrLinkProtocol struct {
	params map[string]any
}

const syntheticQRBindURL = "https://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic"
const syntheticQRAppLink = "https://applink.coupang.com/open?url=https%3A%2F%2Flogin.coupang.com%2Flogin%2Fm%2Fqrcode%2Fbind.pang%3FqrCode%3Dsynthetic"

func (p *qrLinkProtocol) Call(_ context.Context, method string, params, result any) error {
	if method != "Runtime.evaluate" {
		return nil
	}
	p.params, _ = params.(map[string]any)
	encoded, _ := json.Marshal(map[string]string{"url": syntheticQRAppLink, "approvalCode": "42"})
	payload, _ := json.Marshal(map[string]any{
		"result": map[string]any{"value": string(encoded)},
	})
	return json.Unmarshal(payload, result)
}

func (p *qrCaptureProtocol) Call(_ context.Context, method string, params, result any) error {
	switch method {
	case "Runtime.evaluate":
		payload, _ := json.Marshal(map[string]any{
			"result": map[string]any{"value": `{"x":100,"y":50,"width":180,"height":180,"scale":2}`},
		})
		return json.Unmarshal(payload, result)
	case "Page.captureScreenshot":
		request, _ := params.(map[string]any)
		p.clip, _ = request["clip"].(map[string]any)
		payload, _ := json.Marshal(map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("synthetic-png"))})
		return json.Unmarshal(payload, result)
	default:
		return nil
	}
}

func TestClassifyQRPage(t *testing.T) {
	for _, test := range []struct {
		name  string
		state qrPageState
		want  qrPagePhase
	}{
		{name: "ready", state: qrPageState{Href: loginURL, QRReady: true}, want: qrPhaseReady},
		{name: "expired", state: qrPageState{Href: loginURL, QRExpired: true}, want: qrPhaseExpired},
		{name: "approved", state: qrPageState{Href: orderListURL, Status: 200, DocumentReady: true, StructuredOrderData: true}, want: qrPhaseApproved},
		{name: "order still loading", state: qrPageState{Href: orderListURL, Status: 200, DocumentReady: true}, want: qrPhaseWaiting},
		{name: "wrong destination", state: qrPageState{Href: "https://www.coupang.com/"}, want: qrPhaseUnexpected},
		{name: "still loading", state: qrPageState{Href: loginURL}, want: qrPhaseWaiting},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyQRPage(test.state); got != test.want {
				t.Fatalf("phase = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWriteSensitiveQRFileUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login-qr.png")
	if err := writeSensitiveQRFile(path, []byte("synthetic-png")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestCaptureQRPageCropsAndEnlargesQRCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login-qr.png")
	protocol := &qrCaptureProtocol{}
	if err := captureQRPage(context.Background(), protocol, path); err != nil {
		t.Fatal(err)
	}
	if protocol.clip == nil {
		t.Fatal("QR screenshot was not clipped")
	}
	if protocol.clip["scale"] != float64(2) && protocol.clip["scale"] != 2 {
		t.Fatalf("QR screenshot scale = %#v, want 2", protocol.clip["scale"])
	}
	if protocol.clip["width"] != float64(180) || protocol.clip["height"] != float64(180) {
		t.Fatalf("QR screenshot clip = %#v", protocol.clip)
	}
}

func TestReadQRLoginLinkUsesEphemeralBrowserDecoder(t *testing.T) {
	protocol := &qrLinkProtocol{}
	link, err := readQRLoginLink(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != syntheticQRAppLink || link.ApprovalCode != "42" {
		t.Fatalf("unexpected QR link metadata")
	}
	if protocol.params["awaitPromise"] != true || protocol.params["returnByValue"] != true {
		t.Fatalf("QR decoder did not await the browser BarcodeDetector")
	}
}

func TestQRLoginLinkValidationRejectsUntrustedOrIncompleteMaterial(t *testing.T) {
	valid := []core.QRLoginLink{
		{URL: syntheticQRAppLink, ApprovalCode: "42"},
		{URL: syntheticQRBindURL, ApprovalCode: "09"},
	}
	for _, link := range valid {
		if !validQRLoginLink(link) {
			t.Fatalf("valid synthetic QR link rejected")
		}
	}
	for _, link := range []core.QRLoginLink{
		{URL: "https://evil.example/open?url=" + syntheticQRBindURL, ApprovalCode: "42"},
		{URL: "http://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic", ApprovalCode: "42"},
		{URL: syntheticQRBindURL, ApprovalCode: "4"},
		{URL: "https://login.coupang.com/login/m/qrcode/bind.pang", ApprovalCode: "42"},
	} {
		if validQRLoginLink(link) {
			t.Fatalf("untrusted QR login material was accepted")
		}
	}
}

func TestWriteSensitiveQRFileRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "login-qr.png")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := writeSensitiveQRFile(path, []byte("synthetic-png")); err == nil {
		t.Fatal("symlink output was accepted")
	}
}
