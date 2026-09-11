package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// CheckCamofoxInstallation checks local files only: status/setup never need to
// launch a browser just to discover whether a runtime is installed.
func CheckCamofoxInstallation(cfg CamofoxConfig) error {
	base := filepath.Dir(cfg.Server)
	for _, path := range []string{cfg.Node, cfg.Server, filepath.Join(cfg.EngineDir, "version.json"), filepath.Join(base, "plugins", "persistence", "index.js"), filepath.Join(base, "mcp", "lib", "cookies.mjs")} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ErrCamofoxUnavailable
		}
	}
	return nil
}

func (c *Camofox) Inspect(context.Context) (auth.BrowserStatus, error) {
	if err := CheckCamofoxInstallation(c.cfg); err != nil {
		return auth.BrowserStatus{}, err
	}
	hash := sha256.Sum256([]byte(c.cfg.UserID))
	path := filepath.Join(c.stateDir, "camofox", "profiles", hex.EncodeToString(hash[:])[:32], "storage-state.json")
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return auth.BrowserStatus{}, ErrCamofoxUnavailable
	}
	return auth.BrowserStatus{Name: "Camofox", ProfilePresent: err == nil && info.Mode().IsRegular(), AccessBlockedAction: "retry later in the dedicated Camofox profile; no alternate browser is selected"}, nil
}

func (c *Camofox) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	return c.reader("orders").FetchPage(ctx, cursor)
}

func (c *Camofox) Login(ctx context.Context, request core.LoginRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Mode == core.LoginModePhone || request.QROutputPath != "" {
		return ErrCamofoxUnsupported
	}
	if err := c.Verify(ctx); err == nil {
		return nil
	} else if !errors.Is(err, core.ErrAuthenticationRequired) {
		return err
	}
	poll := strings.Replace(loginDocumentPoll, "/* AUTH_READER */", authenticationDocumentPoll, 1)
	linkMode := request.PresentQRLink != nil
	activate := "false"
	mode := "login"
	if linkMode {
		activate = "true"
		mode = "login_link"
	}
	expression, _ := json.Marshal("(" + poll + ")('__coupangctl_camofox_login'," + activate + ")")
	linkExpression, _ := json.Marshal(qrLoginLinkExpression)
	// Login owns one page in the same isolated profile. A QR link is streamed
	// only to the explicit presenter, never to normal command/MCP JSON.
	script := `const p=await openTab('https://mc.coupang.com/ssr/desktop/order/list');try{
 let sent=false,r={status:'loading'};
 for(let i=0;i<360;i++){
  r=await p.evaluate(` + string(expression) + `);if(typeof r==='string')r=JSON.parse(r);
  if(['ok','access_denied','authentication_data_missing','qr_expired','unexpected_destination'].includes(r.status))break;
  if(r.status==='qr_ready'&&!sent){
   let v=await p.evaluate(` + string(linkExpression) + `);if(typeof v==='string')v=JSON.parse(v);
   if(v?.url&&v?.approvalCode){console.log('COUPANGCTL_QR '+JSON.stringify(v));sent=true;}
   else {v=await p.captureQR();if(v){console.log('COUPANGCTL_QR_IMAGE '+JSON.stringify(v));sent=true;}}
  }
  await new Promise(r=>setTimeout(r,500));
 }
 console.log('COUPANGCTL_RESULT '+JSON.stringify({status:r.status}));
 }finally{await p.close();}`
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	if request.Progress != nil {
		if err := request.Progress(ctx, core.LoginWaitingForApproval); err != nil {
			return err
		}
	}
	data, err := c.run(ctx, script, mode, request.PresentQRLink)
	if err != nil {
		return err
	}
	var result struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil {
		return ErrDocumentProtocol
	}
	switch result.Status {
	case "ok":
		return c.Verify(ctx) // New headless process verifies persisted login.
	case "access_denied":
		return core.ErrBrowserAccessDenied
	case "authentication_data_missing":
		return ErrAuthenticationStatusUnavailable
	case "qr_expired":
		return ErrQRExpired
	case "unexpected_destination":
		return ErrQRUnexpectedDestination
	default:
		return ErrQRLoginTimedOut
	}
}
