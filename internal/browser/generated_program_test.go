package browser

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Programs cross two JavaScript parsing boundaries: the Node operation and
// the expression evaluated inside the tab. Exercise both without a real page.
func TestGeneratedAuthenticationProgramExecutesBundledExpression(t *testing.T) {
	c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		if mode != "orders" {
			t.Fatal("verification changed operation")
		}
		return runGeneratedProgram(t, ctx, script, `
const target='https://mc.coupang.com/ssr/desktop/order/list';
const endpoint='https://mc.coupang.com/ssr/api/member/auth';
let reads=0,closed=0,result;
const context=vm.createContext({URL,window:{},location:new URL(target),AbortController,TextDecoder,
 document:{title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null},
 performance:{getEntriesByType:()=>[]},setTimeout:()=>0,clearTimeout:()=>{},
 fetch:async(url,options)=>{assert.equal(url,endpoint);assert.equal(options.method,'GET');reads++;const r=new Response('true',{headers:{'content-type':'application/json'}});Object.defineProperty(r,'url',{value:endpoint});return r;}
});
const openTab=async url=>{assert.equal(url,target);return {evaluate:async expression=>vm.runInContext(expression,context),close:async()=>{closed++;}}};
await new AsyncFunction('openTab','console','setTimeout',script)(openTab,{log:line=>result=line},fn=>setImmediate(fn));
assert.equal(reads,1);assert.equal(closed,1);
process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));`)
	}}
	if err := c.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedLoginProgramPreservesPollAndQRExpressions(t *testing.T) {
	checks, logins := 0, 0
	c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		if mode == "orders" {
			checks++
			if checks == 1 {
				return []byte(`{"status":"authentication_required"}`), nil
			}
			return []byte(`{"status":"ok","authenticated":true}`), nil
		}
		if mode != "login_link" {
			t.Fatal("explicit link mode lost")
		}
		logins++
		return runGeneratedProgram(t, ctx, script, `
let polls=0,links=0,closed=0,result;
const openTab=async url=>{
 assert.equal(url,'https://mc.coupang.com/ssr/desktop/order/list');
 return {
  evaluate:async expression=>{
   assert.equal(typeof expression,'string');new vm.Script(expression);
   if(expression.includes('BarcodeDetector')){links++;return JSON.stringify({});}
   assert.match(expression,/pollSelectedLogin/);assert.match(expression,/pollAuthenticationPage/);
   assert.match(expression,/\('__coupangctl_camofox_login',true\)$/);
   return JSON.stringify({status:++polls===1?'qr_ready':'ok'});
  },captureQR:async()=>null,close:async()=>{closed++;}
 };
};
await new AsyncFunction('openTab','console','setTimeout',script)(openTab,{log:line=>{assert.equal(result,undefined);result=line;}},fn=>setImmediate(fn));
assert.equal(polls,2);assert.equal(links,1);assert.equal(closed,1);
process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));`)
	}}
	err := c.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR, PresentQRLink: func(context.Context, core.QRLoginLink) error { return nil }})
	if err != nil || checks != 2 || logins != 1 {
		t.Fatalf("login generation or persisted verification failed: %v; checks=%d, logins=%d", err, checks, logins)
	}
}

func runGeneratedProgram(t *testing.T, ctx context.Context, script, harness string) ([]byte, error) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for generated program integration")
	}
	const prefix = `import assert from 'node:assert/strict';import vm from 'node:vm';
let script='';for await(const part of process.stdin)script+=part;
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
`
	cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", prefix+harness)
	cmd.Stdin = strings.NewReader(script)
	return cmd.Output()
}
