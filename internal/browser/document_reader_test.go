package browser

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Execute the bundled program, not just a canned successful transport reply.
// The caller sees one source read; its authentication gate stays in that runtime.
func TestCamofoxOrdersVerifyBeforeExtractionInSameTab(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for bundled reader")
	}
	for _, tc := range []struct {
		name, value string
		want        error
	}{
		{"authenticated", "true", nil},
		{"signed_out", "false", core.ErrAuthenticationRequired},
		{"unknown", "null", core.ErrAuthenticationStatusUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &Camofox{run: func(ctx context.Context, script, mode string, presenter core.QRLinkPresenter) ([]byte, error) {
				calls++
				if mode != "orders" || presenter != nil {
					t.Fatal("protected read changed browser mode or requested interactive login")
				}
				const harness = `const vm=require('node:vm');
const target='https://mc.coupang.com/ssr/desktop/order/list';
const endpoint='https://mc.coupang.com/ssr/api/member/auth';
const value=process.argv[1];
let opened=0,closed=0,authCalls=0,orderReads=0;
const openTab=async url=>{
 if(url!==target||++opened!==1)throw Error('unexpected tab');
 const context=vm.createContext({URL,TextDecoder,TextEncoder,AbortController,setTimeout,clearTimeout,
  location:new URL(target),window:{},performance:{getEntriesByType:()=>[]},
  document:{title:'Synthetic',readyState:'complete',body:{innerText:''},querySelector:s=>{
   if(s==='input[type="password"]')return null;
   if(s!=='script#__NEXT_DATA__'||authCalls!==1||value!=='true')throw Error('premature order read');
   orderReads++;return {textContent:'{"props":{"pageProps":{"domains":{"desktopOrder":{"orderList":[],"hasNext":false}}}}}'};
  }},
  fetch:async(url,opts)=>{
   if(url!==endpoint||opts.method!=='GET'||++authCalls!==1)throw Error('unexpected request');
   const r=new Response(value,{headers:{'content-type':'application/json'}});
   Object.defineProperty(r,'url',{value:endpoint});return r;
  },
 });
 return {evaluate:async expression=>vm.runInContext(expression,context),close:async()=>closed++};
};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
new AsyncFunction('openTab','console',require('node:fs').readFileSync(0,'utf8'))(openTab,console).then(()=>{
 if(opened!==1||closed!==1||authCalls!==1||orderReads!==(value==='true'?1:0))process.exitCode=1;
}).catch(()=>process.exitCode=1);`
				command := exec.CommandContext(ctx, node, "-e", harness, tc.value)
				command.Stdin = strings.NewReader(script)
				output, err := command.Output()
				if err != nil {
					t.Fatal("bundled protected reader violated authentication or lifecycle contract", err)
				}
				return consumeCamofoxOutput(ctx, bytes.NewReader(output), nil)
			}}
			page, err := c.FetchPage(context.Background(), nil)
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("protected read: %v; runtime calls=%d", err, calls)
			}
			if (tc.want == nil) != (page.Orders != nil) {
				t.Fatal("authentication outcome did not gate order evidence")
			}
		})
	}
}

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
