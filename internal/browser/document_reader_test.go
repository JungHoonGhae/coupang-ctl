package browser

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCamofoxPartialOrderDocumentPreservesTypedFailure(t *testing.T) {
	for _, cursor := range []*core.OrderCursor{nil, {Year: 2026, Page: 2}} {
		calls := 0
		c := NewCamofox(t.TempDir(), CamofoxConfig{})
		c.run = func(_ context.Context, _ string, mode string, _ core.QRLinkPresenter) ([]byte, error) {
			calls++
			if mode != "orders" {
				t.Fatal("partial order read selected a different browser operation")
			}
			return []byte(`{"status":"order_partial","page":{"orders":[]}}`), nil
		}
		page, err := c.FetchPage(context.Background(), cursor)
		if !errors.Is(err, core.ErrPartialOrderData) || page.Orders != nil || page.Next != nil || calls != 1 {
			t.Fatal("partial order evidence was accepted, reclassified, or automatically retried", err)
		}
	}
}

func TestDocumentReadRejectsFalseProtectedSuccess(t *testing.T) {
	for _, data := range []string{`{"status":"authentication_required"}`, `{"status":"access_denied"}`, `{"status":"ok","page":{}}`, `{"status":"loading"}`, `{"status":"ok"}`} {
		d := &documentBrowser{run: func(context.Context, string) ([]byte, error) { return []byte(data), nil }}
		if _, err := d.FetchPage(context.Background(), nil); err == nil {
			t.Fatal("invalid order evidence accepted")
		}
	}
}
func TestDocumentReadScriptClosesOwnedTabOnSuccessAndFailure(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node required for JS transport contract")
	}
	for _, throws := range []bool{false, true} {
		a := &documentBrowser{run: func(ctx context.Context, script string) ([]byte, error) {
			body := `let closed=0,opened=0;global.openTab=async()=>{opened++;return {evaluate:async()=>({status:'ok',page:{orders:[]}}),close:async()=>{closed++}}};`
			if throws {
				body = `let closed=0,opened=0;global.openTab=async()=>{opened++;return {evaluate:async()=>{throw Error('synthetic')},close:async()=>{closed++}}};`
			}
			body += `(async()=>{try{` + script + `}catch{}if(closed!==1||opened!==1)process.exit(2)})()`
			out, err := exec.CommandContext(ctx, "node", "-e", body).Output()
			if err != nil {
				t.Fatal("JS transport cleanup failed")
			}
			if throws {
				return nil, ErrCamofoxUnavailable
			}
			return consumeCamofoxOutput(ctx, bytes.NewReader(out), nil)
		}}
		_, err := a.FetchPage(context.Background(), nil)
		if throws != (err != nil) {
			t.Fatal("unexpected result")
		}
	}
}
