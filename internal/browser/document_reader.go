package browser

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// documentBrowser is the shared typed document reader. Its transport owns the
// browser lifecycle; neither the reader nor its callers select an installed app.
type documentBrowser struct {
	run func(context.Context, string) ([]byte, error)
}

func (a *documentBrowser) read(ctx context.Context, target, expression string) (documentPageResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	u, _ := json.Marshal(target)
	e, _ := json.Marshal(expression)
	polls := 30
	if target == orderListURL {
		// The same tab verifies authentication before parsing orders. Allow
		// both bounded 10-second readers to finish inside the operation deadline.
		polls = 60
	}
	// No bringToFront, window manipulation, cookie API, or arbitrary user JS.
	script := `const p=await openTab(` + string(u) + `);try{let r;for(let i=0;i<` + strconv.Itoa(polls) + `;i++){r=await p.evaluate(` + string(e) + `);if(typeof r==='string')r=JSON.parse(r);if(r.status!=='loading')break;await new Promise(r=>setTimeout(r,500));}console.log('COUPANGCTL_RESULT '+JSON.stringify(r));}finally{await p.close();}`
	data, err := a.run(ctx, script)
	if err != nil {
		return documentPageResult{}, err
	}
	var result documentPageResult
	if json.Unmarshal(data, &result) != nil {
		return result, ErrDocumentProtocol
	}
	if target == orderListURL && result.Status == "loading" {
		return result, core.ErrAuthenticationStatusUnavailable
	}
	switch result.Status {
	case "ok":
		return result, nil
	case "authentication_required":
		return result, core.ErrAuthenticationRequired
	case "authentication_data_missing":
		return result, core.ErrAuthenticationStatusUnavailable
	case "access_denied":
		return result, core.ErrBrowserAccessDenied
	case "order_data_missing":
		return result, ErrStructuredOrderDataMissing
	case "order_partial":
		return result, core.ErrPartialOrderData
	case "category_unavailable":
		return result, core.ErrProductCategoryUnavailable
	case "category_data_missing":
		return result, ErrStructuredCategoryDataMissing
	default:
		return result, ErrStructuredProductDataMissing
	}
}

func (a *documentBrowser) FetchProductSearch(ctx context.Context, request core.ProductSearchRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	base := request
	base.FacetSelections = nil
	base.CategoryTrail = nil
	// This copy describes only the initial URL; the original request keeps
	// the full reservation for sidebar navigation in readSearchWithFacets.
	base.DocumentReadLimit = 1
	target, err := productSearchURL(base)
	if err != nil {
		return nil, err
	}
	r, coverage, err := a.readSearchWithFacets(ctx, target, searchPageReader, request)
	if err != nil {
		return nil, err
	}
	if r.Search == nil || r.Search.Validate() != nil || r.Page != nil || len(r.Inspection) != 0 || len(r.Category) != 0 {
		return nil, ErrDocumentProtocol
	}
	coverage.Source = "camofox_search_document"
	coverage.ObservedFields = []string{"identity", "name", "search_position"}
	coverage.UnavailableFields = []string{"rating", "reviews", "delivery_badges"}
	return json.Marshal(struct {
		searchDocument
		Coverage core.ProductCoverage `json:"coverage"`
	}{*r.Search, coverage})
}

func orderDocumentExpression(cursor *core.OrderCursor) (string, error) {
	poll := strings.Replace(orderDocumentPoll, "/* SHARED_READER */", strings.Replace(orderPageReader, "export async function", "async function", 1), 1)
	poll = strings.Replace(poll, "/* AUTH_READER */", authenticationDocumentPoll, 1)
	c, _ := json.Marshal(cursor)
	return "(" + poll + ")(" + string(c) + ",'__coupangctl_orders')", nil
}

func (a *documentBrowser) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	if cursor != nil && (cursor.Year < 2000 || cursor.Year > 2100 || cursor.Page < 0 || cursor.Page > 1000) {
		return core.OrderPage{}, ErrDocumentProtocol
	}
	expression, err := orderDocumentExpression(cursor)
	if err != nil {
		return core.OrderPage{}, err
	}
	r, err := a.read(ctx, orderListURL, expression)
	if err != nil {
		return core.OrderPage{}, err
	}
	if r.Page == nil || r.Page.Orders == nil || r.Search != nil || len(r.Inspection) != 0 || len(r.Category) != 0 || validateOrderDocument(*r.Page) != nil {
		return core.OrderPage{}, ErrDocumentProtocol
	}
	return *r.Page, nil
}
