package browser

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/coupang/categories"
)

func TestCamofoxCategoryExecutesBundledReaderWithExactProduct(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for document reader")
	}
	c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		if mode != "category" {
			t.Fatal("wrong category operation")
		}
		const harness = `const vm=require('node:vm');
const target='https://www.coupang.com/vp/products/101?vendorItemId=201';
const breadcrumb={'@type':'BreadcrumbList',secret:'synthetic-private-value',itemListElement:[{'@type':'ListItem',position:1,name:'Synthetic category',item:'https://www.coupang.com/np/categories/100'}]};
const context=vm.createContext({URL,location:{href:target},performance:{getEntriesByType:()=>[]},document:{readyState:'complete',title:'Synthetic',body:{innerText:''},querySelector:()=>null,querySelectorAll:()=>[{textContent:JSON.stringify(breadcrumb)}]}});
let closed=false,result;
const openTab=async url=>{if(url!==target)throw Error('wrong target');return {evaluate:async e=>vm.runInContext(e,context),close:async()=>{closed=true}}};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
new AsyncFunction('openTab','console',process.argv[1])(openTab,{log:line=>{result=line}}).then(()=>{if(!closed||!result||result.includes('synthetic-private-value'))throw Error('invalid output');process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));}).catch(()=>process.exitCode=1);`
		return exec.CommandContext(ctx, node, "-e", harness, script).Output()
	}}
	doc, err := c.FetchProductCategory(context.Background(), core.ProductReference{ProductID: "101", VendorItemID: "201"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := categories.ParseProductCategory(doc)
	if err != nil || result.Source != core.CategorySourceProductJSONLDBreadcrumb || len(result.Path) != 1 || result.Path[0].ID != "100" {
		t.Fatal("category evidence lost")
	}
}

func TestCamofoxCategoryRejectsInvalidIdentityAndDistinguishesAccessFailures(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("invalid input acquired browser")
		return nil, nil
	}}
	for _, ref := range []core.ProductReference{{}, {ProductID: "../101"}, {ProductID: "101", VendorItemID: "x"}} {
		if _, err := c.FetchProductCategory(context.Background(), ref); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	for _, tc := range []struct {
		wire string
		want error
	}{
		{`{"status":"authentication_required"}`, core.ErrAuthenticationRequired},
		{`{"status":"access_denied"}`, core.ErrBrowserAccessDenied},
		{`{"status":"category_unavailable"}`, core.ErrProductCategoryUnavailable},
		{`{"status":"category_data_missing"}`, ErrStructuredCategoryDataMissing},
		{`{"status":"ok","category":{"json_ld":[]},"inspection":{}}`, ErrDocumentProtocol},
		{`{"status":"ok"}`, ErrDocumentProtocol},
	} {
		c.run = func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(tc.wire), nil
		}
		if data, err := c.FetchProductCategory(context.Background(), core.ProductReference{ProductID: "101"}); !errors.Is(err, tc.want) || len(data) > 0 {
			t.Fatal("category failure became evidence")
		}
	}
}
