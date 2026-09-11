package browser

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrAuthenticationStatusUnavailable = core.ErrAuthenticationStatusUnavailable

//go:embed authentication_document_poll.js
var authenticationDocumentPoll string

// Verify checks only the authenticated session. Orders remain independently
// parsed by FetchPage; neither success establishes a stable account identity.
func (c *Camofox) Verify(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	expression, _ := json.Marshal("(" + authenticationDocumentPoll + ")('__coupangctl_auth')")
	script := `const p=await openTab('https://mc.coupang.com/ssr/desktop/order/list');try{
let r={status:'loading'};
for(let i=0;i<30;i++){r=await p.evaluate(` + string(expression) + `);if(typeof r==='string')r=JSON.parse(r);if(r.status!=='loading')break;await new Promise(r=>setTimeout(r,500));}
console.log('COUPANGCTL_RESULT '+JSON.stringify(r));
}finally{await p.close();}`
	data, err := c.run(ctx, script, "orders", nil)
	if err != nil {
		return err
	}
	var result struct {
		Status        string `json:"status"`
		Authenticated *bool  `json:"authenticated"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if len(data) > 4096 || decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrDocumentProtocol
	}
	if result.Status == "ok" {
		if result.Authenticated == nil || !*result.Authenticated {
			return ErrDocumentProtocol
		}
		return nil
	}
	if result.Authenticated != nil {
		return ErrDocumentProtocol
	}
	switch result.Status {
	case "authentication_required":
		return core.ErrAuthenticationRequired
	case "access_denied":
		return core.ErrBrowserAccessDenied
	default:
		return ErrAuthenticationStatusUnavailable
	}
}
