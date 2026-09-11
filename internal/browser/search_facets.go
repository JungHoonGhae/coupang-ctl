package browser

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

//go:embed search_facets.js
var searchFacetReader string

var ErrSearchFacetUnavailable = core.WithErrorCode("search_facet_unavailable", errors.New("sidebar filter unavailable or its selection could not be verified; run a plain search to rediscover options"))

func (a *documentBrowser) readSearchWithFacets(ctx context.Context, target, reader string, request core.ProductSearchRequest) (documentPageResult, core.ProductCoverage, error) {
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	urlJSON, _ := json.Marshal(target)
	selections, _ := json.Marshal(request.FacetSelections)
	trail, _ := json.Marshal(request.CategoryTrail)
	facetFunction := `(function(expected,selection,transition){` + searchFacetReader + `;return readSearchFacets(expected,selection,transition);})`
	facetCode, _ := json.Marshal(facetFunction)
	searchCode, _ := json.Marshal(`(function(expected){` + strings.Replace(reader, "export function", "function", 1) + `;return readSelectedSearchPage(expected);})`)
	// Locator indices are read from the current sidebar, never caller-supplied.
	// Each click is checked before continuing; no repeated toggle on timeout.
	script := `const p=await openTab(` + string(urlJSON) + `);try{
 const selections=` + string(selections) + `||[];let baseURL=` + string(urlJSON) + `,appliedCategoryID='';
 const navigation=(` + string(trail) + `||[]).map(label=>({name:'카테고리',label})).concat(selections);
 const facetCode=` + string(facetCode) + `, searchCode=` + string(searchCode) + `;
 const facets=async(selection,transition=false)=>{const f=await p.evaluate('('+facetCode+')('+JSON.stringify(baseURL)+','+JSON.stringify(selection)+','+JSON.stringify(transition)+')');if(['access_denied','authentication_required'].includes(f.status))throw Error(f.status);return f;};
 await new Promise(r=>setTimeout(r,1800));
 for(const selection of navigation){
  let f=await facets(selection);if(f.status!=='ok')throw Error('filter_unavailable');
  if(!f.target.selected){await p.locator('.filter-function-bar label').nth(f.target.index).click();
   let ok=false;for(let i=0;i<16;i++){await new Promise(r=>setTimeout(r,350));f=await facets(selection,true);if(f.status==='ok'&&f.target.selected){ok=true;break;}}if(!ok)throw Error('filter_unavailable');
   if(f.category_id){const next=new URL(baseURL);next.pathname='/np/categories/'+f.category_id;baseURL=next.href;appliedCategoryID=f.category_id;}}
 }
 let f=await facets(null);if(f.status!=='ok'&&selections.length)throw Error('filter_unavailable');
 for(const s of selections){const g=f.facets.find(g=>g.name===s.name);if(!g?.options.some(o=>o.label===s.label&&o.selected))throw Error('filter_unavailable');}
 const bound=await p.evaluate('(function(base){const a=new URL(base),b=new URL(location.href),defaultSort=a.pathname.startsWith("/np/categories/")?"bestAsc":"scoreDesc";const valid=!b.username&&!b.password&&!b.hash&&a.origin===b.origin&&a.pathname===b.pathname&&["q","sorter","page"].every(k=>b.searchParams.getAll(k).length<=1&&(a.searchParams.get(k)??(k==="sorter"?defaultSort:k==="page"?"1":""))===(b.searchParams.get(k)??(k==="sorter"?defaultSort:k==="page"?"1":"")));return valid?b.href:"";})('+JSON.stringify(baseURL)+')');
 if(!bound)throw Error('filter_unavailable');
 let r;for(let i=0;i<20;i++){r=await p.evaluate('('+searchCode+')('+JSON.stringify(bound)+')');if(r.status!=='loading')break;await new Promise(r=>setTimeout(r,350));}
 console.log('COUPANGCTL_RESULT '+JSON.stringify({result:r,facets:f.status==='ok'?f.facets:[],...(appliedCategoryID?{applied_category_id:appliedCategoryID}:{})}));
 }catch(e){console.log('COUPANGCTL_RESULT '+JSON.stringify(['access_denied','authentication_required'].includes(e.message)?{result:{status:e.message}}:{error:'filter_unavailable'}));}finally{await p.close();}`
	data, err := a.run(ctx, script)
	if err != nil {
		return documentPageResult{}, core.ProductCoverage{}, err
	}
	var payload struct {
		Result            documentPageResult  `json:"result"`
		Facets            []core.ProductFacet `json:"facets"`
		Error             string              `json:"error"`
		AppliedCategoryID string              `json:"applied_category_id"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.Error != "" {
		return payload.Result, core.ProductCoverage{}, ErrSearchFacetUnavailable
	}
	if payload.Result.Status != "ok" {
		if payload.Result.Status == "access_denied" {
			return payload.Result, core.ProductCoverage{}, ErrBrowserAccessDenied
		}
		if payload.Result.Status == "authentication_required" {
			return payload.Result, core.ProductCoverage{}, core.ErrAuthenticationRequired
		}
		return payload.Result, core.ProductCoverage{}, ErrStructuredProductDataMissing
	}
	if len(payload.Facets) > 50 {
		return payload.Result, core.ProductCoverage{}, ErrSearchFacetUnavailable
	}
	for _, f := range payload.Facets {
		if f.Name == "" || len(f.Name) > 200 || len(f.ID) > 100 || len(f.Options) > 100 {
			return payload.Result, core.ProductCoverage{}, ErrSearchFacetUnavailable
		}
		for _, o := range f.Options {
			if o.Label == "" || len(o.Label) > 400 || len(o.ID) > 100 {
				return payload.Result, core.ProductCoverage{}, ErrSearchFacetUnavailable
			}
		}
	}
	coverage := core.ProductCoverage{Facets: payload.Facets, AppliedCategoryID: payload.AppliedCategoryID}
	if err := coverage.ValidateCategoryScope(request); err != nil {
		return payload.Result, core.ProductCoverage{}, ErrSearchFacetUnavailable
	}
	return payload.Result, coverage, nil
}
