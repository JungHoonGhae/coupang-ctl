package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	parser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

// Exercise the generated program and its shipped readers, not just a fabricated
// success envelope. Only the transport and source document are synthetic.
func TestCamofoxCategorySearchWithSidebar(t *testing.T) {
	for _, scenario := range []string{"category", "category_default", "query", "other_category", "other_sort", "blocked", "login", "category_transition", "category_transition_no_breadcrumb", "category_transition_wrong_breadcrumb", "category_transition_wrong_sort"} {
		t.Run(scenario, func(t *testing.T) {
			node, err := exec.LookPath("node")
			if err != nil {
				t.Skip("Node required for bundled-reader test")
			}
			const harness = `
const vm=require('node:vm'),scenario=process.argv[2];
const transition=scenario.startsWith('category_transition');
let result,closed=false,clicks=0;
global.setTimeout=fn=>{fn();return 0};
const openTab=async target=>{
 const expected=scenario==='query'?'https://www.coupang.com/np/search?q=synthetic&sorter=salePriceAsc':scenario==='category_default'||transition?'https://www.coupang.com/np/categories/123456':'https://www.coupang.com/np/categories/123456?sorter=salePriceAsc';
 if(target!==expected)throw Error('wrong initial document');
 let selected=false;
 const location={href:target};
 const option={textContent:transition?'Synthetic child':'32 GB',classList:{contains:c=>c==='selected'&&selected},getAttribute:()=>null,parentElement:{classList:{contains:()=>false}}};
 const group={querySelectorAll:()=>[option],querySelector:()=>null};
 const root={querySelectorAll:s=>s==='label'?[option]:[{textContent:transition?'카테고리':'Memory',parentElement:group}]};
 const document={title:'Synthetic',body:{innerText:''},readyState:'complete',scripts:[],querySelector:()=>null,querySelectorAll:s=>s==='.filter-function-bar'?[root]:s.includes('ld+json')?[{textContent:JSON.stringify({'@type':'CollectionPage',url:location.href,mainEntity:{'@type':'ItemList',itemListElement:[{'@type':'Product',name:'Synthetic computer',url:'https://www.coupang.com/vp/products/111',offers:{price:123000,priceCurrency:'KRW'}}]}})},...(transition&&scenario!=='category_transition_no_breadcrumb'?[{textContent:JSON.stringify({'@type':'BreadcrumbList',itemListElement:[{'@type':'ListItem',position:1,name:scenario==='category_transition_wrong_breadcrumb'?'Different category':'Synthetic child',item:'https://www.coupang.com/np/categories/654321'}]})}]:[])]:[]};
 if(scenario==='blocked')document.title='Access Denied';
 if(scenario==='login')location.href='https://login.coupang.com/login/login.pang';
 const context=vm.createContext({URL,location,document});
 return {evaluate:expression=>vm.runInContext(expression,context),locator:selector=>({nth:index=>({click:async()=>{
  if(selector!=='.filter-function-bar label'||index!==0||selected)throw Error('unsafe/repeated click');
  clicks++;selected=true;location.href=target+(target.includes('?')?'&':'?')+'filter=synthetic'+(scenario==='category_default'?'&sorter=bestAsc':'');
  if(scenario==='other_category')location.href=location.href.replace('123456','654321');
  if(scenario==='other_sort')location.href=location.href.replace('salePriceAsc','saleCountDesc');
  if(transition)location.href='https://www.coupang.com/np/categories/654321'+(scenario==='category_transition_wrong_sort'?'?sorter=salePriceAsc':'');
 }})}),close:async()=>{closed=true;}};
};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
new AsyncFunction('openTab','console',process.argv[1])(openTab,{log:line=>result=line}).then(()=>{
 if(!closed||!result||clicks>1)throw Error('owned-tab lifecycle failed');
 process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));
}).catch(()=>process.exitCode=1);`
			calls := 0
			c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
				calls++
				if mode != "search" {
					t.Fatal("wrong runtime mode")
				}
				return exec.CommandContext(ctx, node, "-e", harness, script, scenario).Output()
			}}
			r := core.ProductSearchRequest{CategoryID: "123456", Sort: core.ProductSortPriceAsc, FacetSelections: []core.ProductFacetSelection{{Name: "Memory", Label: "32 GB"}}}
			if scenario == "category_default" {
				r.Sort = core.ProductSortRelevance
			}
			if strings.HasPrefix(scenario, "category_transition") {
				r.Sort = core.ProductSortRelevance
				r.FacetSelections = []core.ProductFacetSelection{{Name: "카테고리", Label: "Synthetic child"}}
			}
			if scenario == "query" {
				r.CategoryID = ""
				r.Query = "synthetic"
			}
			if scenario == "blocked" || scenario == "login" {
				r.FacetSelections = nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			document, err := c.FetchProductSearch(ctx, r)
			if calls != 1 {
				t.Fatal("category search never reached the shared reader")
			}
			switch scenario {
			case "other_category", "other_sort", "category_transition_no_breadcrumb", "category_transition_wrong_breadcrumb", "category_transition_wrong_sort":
				if !errors.Is(err, ErrSearchFacetUnavailable) {
					t.Fatalf("changed document was accepted: %v", err)
				}
			case "blocked":
				if !errors.Is(err, core.ErrBrowserAccessDenied) {
					t.Fatalf("blocked result: %v", err)
				}
			case "login":
				if !errors.Is(err, core.ErrAuthenticationRequired) {
					t.Fatalf("login result: %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				items, _, err := parser.ParseSearchDocument(document)
				if err != nil || len(items) != 1 || items[0].Reference.ProductID != "111" || items[0].Price.CurrentAmount != 123000 {
					t.Fatal("category evidence did not reach the product parser")
				}
				var d struct {
					Coverage core.ProductCoverage `json:"coverage"`
				}
				if json.Unmarshal(document, &d) != nil || d.Coverage.Source != "camofox_search_document" || len(d.Coverage.Facets) != 1 || !d.Coverage.Facets[0].Options[0].Selected {
					t.Fatal("category sidebar selection lost")
				}
				if scenario == "category_transition" {
					var scope struct {
						Coverage struct {
							AppliedCategoryID string `json:"applied_category_id"`
						} `json:"coverage"`
					}
					if json.Unmarshal(document, &scope) != nil || scope.Coverage.AppliedCategoryID != "654321" {
						t.Fatal("verified category destination was not returned with the products")
					}
				}
			}
		})
	}
}

func TestCamofoxCategorySearchRejectsInvalidIdentityBeforeTransport(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("invalid identity reached transport")
		return nil, nil
	}}
	for _, id := range []string{"123/other", "123?x=y", strings.Repeat("1", 25)} {
		if _, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{CategoryID: id}); err == nil {
			t.Fatal("invalid category accepted")
		}
	}
}
