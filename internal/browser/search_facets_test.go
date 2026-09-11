package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Execute the production-generated navigation program against a synthetic tab,
// including catalog rediscovery, click ordering and its final active-state check.
func TestQueryCategoryTrailNavigationProgram(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for browser program tests")
	}
	for _, mode := range []string{"ok", "quoted_data", "missing_child", "access_denied", "lost_filter"} {
		t.Run(mode, func(t *testing.T) {
			a := &documentBrowser{run: func(ctx context.Context, script string) ([]byte, error) {
				harness := `import assert from 'node:assert/strict';
let script='';for await(const b of process.stdin)script+=b;
const mode=process.argv[1],clicks=[];let category='',memory=false,closed=0;
const memoryLabel=mode==='quoted_data'?String.fromCharCode(39,34,92,10,0x2028,0x2029)+';globalThis.syntheticInjected=true;//':'32 GB';
const root='https://www.coupang.com/np/search?q=synthetic';
const p={
 evaluate:async expression=>{
  if(expression.includes('readSearchFacets')){
   const [expected,selection]=JSON.parse('['+expression.slice(expression.lastIndexOf(')(')+2,-1)+']');assert.equal(expected,root);
   if(mode==='access_denied'&&category==='Child')return {status:'access_denied'};
   const options=[{label:'Parent',selected:category==='Parent'}];
   if(category&&mode!=='missing_child')options.push({label:'Child',selected:category==='Child'});
   const facets=[{name:'카테고리',options},{name:'Memory',options:[{label:memoryLabel,selected:memory}]}];
   if(!selection)return {status:'ok',facets};
   const option=facets.find(f=>f.name===selection.name)?.options.find(o=>o.label===selection.label);
   if(!option)return {status:'filter_unavailable',facets};
   return {status:'ok',facets,target:{index:selection.name==='Memory'?2:selection.label==='Parent'?0:1,selected:option.selected}};
  }
  if(expression.includes('readSelectedSearchPage'))return {status:'ok',search:{items:[],no_results:true}};
  return root+'&filter=synthetic';
 },
 locator:selector=>({nth:index=>({click:async()=>{assert.equal(selector,'.filter-function-bar label');clicks.push(index);if(index===2)memory=true;else {category=index===0?'Parent':'Child';if(mode==='lost_filter'&&category==='Child')memory=false;}}})}),
 close:async()=>{closed++;}
};
let output;const originalTimeout=globalThis.setTimeout;globalThis.setTimeout=f=>originalTimeout(f,0);
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
await new AsyncFunction('openTab','console',script)(async url=>{assert.equal(url,root);return p;},{log:value=>{assert.equal(output,undefined);output=value;}});
assert.equal(closed,1);assert.deepEqual(clicks,mode==='missing_child'?[0,2]:[0,2,1]);
assert.equal(globalThis.syntheticInjected,undefined,'facet text executed as code');
process.stdout.write(output.slice('COUPANGCTL_RESULT '.length));`
				cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", harness, mode)
				cmd.Stdin = strings.NewReader(script)
				return cmd.Output()
			}}
			r := core.ProductSearchRequest{Query: "synthetic", CategoryTrail: []string{"Parent"}, FacetSelections: []core.ProductFacetSelection{{Name: "Memory", Label: "32 GB"}, {Name: "카테고리", Label: "Child"}}, DocumentReadLimit: 4}
			if mode == "quoted_data" {
				r.FacetSelections[0].Label = "'\"\\\n\u2028\u2029;globalThis.syntheticInjected=true;//"
			}
			_, coverage, err := a.readSearchWithFacets(context.Background(), "https://www.coupang.com/np/search?q=synthetic", searchPageReader, r)
			switch mode {
			case "ok", "quoted_data":
				if err != nil || len(coverage.Facets) != 2 || coverage.Facets[0].Options[0].Selected || !coverage.Facets[0].Options[1].Selected {
					t.Fatalf("final active scope invalid: %v", err)
				}
			case "access_denied":
				if !errors.Is(err, core.ErrBrowserAccessDenied) {
					t.Fatalf("denial lost: %v", err)
				}
			default:
				if !errors.Is(err, ErrSearchFacetUnavailable) {
					t.Fatalf("unverified trail accepted: %v", err)
				}
			}
		})
	}
}

func TestDocumentFacetEnvelope(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    error
	}{
		{`{"error":"filter_unavailable"}`, ErrSearchFacetUnavailable},
		{`{`, ErrSearchFacetUnavailable},
		{`{"result":{"status":"access_denied"}}`, ErrBrowserAccessDenied},
		{`{"result":{"status":"authentication_required"}}`, core.ErrAuthenticationRequired},
		{`{"result":{"status":"loading"}}`, ErrStructuredProductDataMissing},
		{`{"result":{"status":"ok"},"facets":[{"name":"","options":[]}]}`, ErrSearchFacetUnavailable},
		{`{"result":{"status":"ok"},"facets":[{"name":"Synthetic group","options":[{"label":"Synthetic option","selected":true}]}]}`, nil},
	} {
		a := &documentBrowser{run: func(_ context.Context, script string) ([]byte, error) {
			if !strings.Contains(script, "finally{await p.close();}") || strings.Contains(script, "bringToFront") {
				t.Fatal("owned-tab cleanup or quiet contract changed")
			}
			return []byte(tc.payload), nil
		}}
		_, coverage, err := a.readSearchWithFacets(context.Background(), "https://www.coupang.com/np/search?q=synthetic", "", core.ProductSearchRequest{Query: "synthetic"})
		if !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
		if tc.want == nil && (len(coverage.Facets) != 1 || !coverage.Facets[0].Options[0].Selected) {
			t.Fatal("observed facet lost")
		}
	}
}

func TestDocumentMultipleFacetAllowanceReachesTransport(t *testing.T) {
	reached := errors.New("synthetic transport reached")
	a := &documentBrowser{run: func(context.Context, string) ([]byte, error) { return nil, reached }}
	_, err := a.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic", DocumentReadLimit: 3, FacetSelections: []core.ProductFacetSelection{{Name: "category", Label: "computer"}, {Name: "memory", Label: "32GB"}}})
	if !errors.Is(err, reached) {
		t.Fatalf("valid two-filter request failed before transport: %v", err)
	}
}
