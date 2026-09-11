package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	productparser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

func TestCamofoxInspectionRunsSharedReaderWithExactIdentityAndBudget(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for embedded reader test")
	}
	c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		if mode != "inspect" {
			t.Fatal("detail used wrong operation")
		}
		const harness = `const vm=require('node:vm');
const url='https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301';
const product={'@type':'Product',name:'Synthetic detail',offers:{url,price:0,priceCurrency:'KRW'}};
const document={readyState:'complete',title:'Synthetic',body:{innerText:''},querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(product)}]:[]};
const context=vm.createContext({URL,location:new URL(url),document,window:{},AbortController,setTimeout:(fn,ms)=>ms===10000?0:queueMicrotask(fn),clearTimeout:()=>{},fetch:async()=>{throw Error('unexpected endpoint request')}});
let closed=false,result;
const openTab=async target=>{if(target!==url)throw Error('wrong target');return {evaluate:async expression=>vm.runInContext(expression,context),close:async()=>{closed=true}}};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
new AsyncFunction('openTab','console',process.argv[1])(openTab,{log:line=>{result=line}}).then(()=>{if(!closed||!result)throw Error('page leaked');process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));}).catch(()=>process.exitCode=1);`
		return exec.CommandContext(ctx, node, "-e", harness, script).Output()
	}}
	req := core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301", DocumentReadLimit: 1}
	doc, err := c.FetchProductInspection(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := productparser.ParseInspectionDocument(doc, req)
	if err != nil {
		t.Fatal(err)
	}
	proof, ok := result.Product.EvidenceFor("price.current_amount")
	if !ok || proof.Scope != "selected_option" || result.Product.Price.CurrentAmount != 0 || result.Product.Reference.VendorItemID != "301" {
		t.Fatal("option evidence lost")
	}
	if result.Coverage.Source != "camofox_product_document" || len(result.Coverage.BudgetOmittedFields) != 2 {
		t.Fatal("source or read budget lost")
	}
}

func TestCamofoxInspectionRejectsUnverifiedIdentityAndDenial(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`{"status":"access_denied"}`, core.ErrBrowserAccessDenied},
		{`{"status":"authentication_required"}`, core.ErrAuthenticationRequired},
		{`{"status":"ok","inspection":{"product":{"product_id":"999","name":"Synthetic","url":"https://www.coupang.com/vp/products/999"}}}`, ErrStructuredProductDataMissing},
		{`{"status":"ok","inspection":{}}`, ErrStructuredProductDataMissing},
	} {
		c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(tc.body), nil
		}}
		data, err := c.FetchProductInspection(context.Background(), core.ProductInspectRequest{ProductID: "101"})
		if !errors.Is(err, tc.want) || len(data) != 0 {
			t.Fatal("invalid detail became success")
		}
	}
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("invalid request acquired browser")
		return nil, nil
	}}
	for _, id := range []string{"", "../101", strings.Repeat("1", 25)} {
		if _, err := c.FetchProductInspection(context.Background(), core.ProductInspectRequest{ProductID: id}); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
