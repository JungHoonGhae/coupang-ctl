package browser

import (
	"errors"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"net/url"
	"strconv"
	"strings"
)

const orderListURL = "https://mc.coupang.com/ssr/desktop/order/list"

var ErrAuthenticationRequired = core.ErrAuthenticationRequired

func productSearchURL(request core.ProductSearchRequest) (string, error) {
	if len(request.FacetSelections) > 0 || len(request.CategoryTrail) > 0 {
		return "", ErrSearchFacetUnavailable
	}
	if err := core.ValidateRequest(request); err != nil {
		return "", err
	}
	path := "/np/search"
	query := url.Values{}
	if request.Page > 1 {
		query.Set("page", strconv.Itoa(request.Page))
	}
	if request.CategoryID != "" {
		path = "/np/categories/" + request.CategoryID
	} else {
		query.Set("q", strings.TrimSpace(request.Query))
	}
	sorter := map[core.ProductSort]string{
		core.ProductSortCoupangRanking: "scoreDesc",
		core.ProductSortSales:          "saleCountDesc",
		core.ProductSortLatest:         "latestAsc",
		core.ProductSortPriceAsc:       "salePriceAsc",
		core.ProductSortPriceDesc:      "salePriceDesc",
	}[request.Sort]
	// The category UI labels bestAsc as 쿠팡 랭킹순; query search uses
	// scoreDesc. Preserve the source's route-specific sorting contract.
	if request.CategoryID != "" && sorter == "scoreDesc" {
		sorter = "bestAsc"
	}
	if sorter != "" {
		query.Set("sorter", sorter)
	}
	target := "https://www.coupang.com" + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	if err := validateProductSearchTarget(target); err != nil {
		return "", err
	}
	return target, nil
}

var ErrStructuredOrderDataMissing = core.WithErrorCode("structured_order_data_missing", errors.New("structured order data missing"))

var ErrStructuredCategoryDataMissing = core.WithErrorCode("structured_category_data_missing", errors.New("structured category data missing"))

var ErrStructuredProductDataMissing = core.WithErrorCode("structured_product_data_missing", errors.New("structured product data missing"))

var ErrStructuredAccountBenefitsDataMissing = core.WithErrorCode("structured_account_benefits_data_missing", errors.New("structured account benefits data missing"))

var ErrStructuredReceiptDataMissing = core.WithErrorCode("structured_receipt_data_missing", errors.New("structured receipt data missing"))

var ErrBrowserAccessDenied = core.ErrBrowserAccessDenied

func validateProductSearchTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("blocked product search target")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return errors.New("blocked product search query")
	}
	searchPath := parsed.Path == "/np/search"
	categoryID := strings.TrimPrefix(parsed.Path, "/np/categories/")
	categoryPath := categoryID != parsed.Path && numericURLValue(categoryID)
	if !searchPath && !categoryPath {
		return errors.New("blocked product search target")
	}
	for key, values := range query {
		if len(values) != 1 || (key != "q" && key != "sorter" && key != "page") {
			return errors.New("blocked product search query")
		}
	}
	search := strings.TrimSpace(query.Get("q"))
	if values, ok := query["page"]; ok {
		page, err := strconv.Atoi(query.Get("page"))
		if len(values) != 1 || err != nil || page < 1 || page > 100 || strconv.Itoa(page) != query.Get("page") {
			return errors.New("blocked product search page")
		}
	}
	if (searchPath && (search == "" || len([]rune(search)) > 200)) || (categoryPath && search != "") {
		return errors.New("blocked product search query")
	}
	if sorter := query.Get("sorter"); query.Has("sorter") {
		switch sorter {
		case "scoreDesc":
			if categoryPath {
				return errors.New("blocked product category sorter")
			}
		case "bestAsc":
			if !categoryPath {
				return errors.New("blocked product search sorter")
			}
		case "saleCountDesc", "latestAsc", "salePriceAsc", "salePriceDesc":
		default:
			return errors.New("blocked product search query")
		}
	}
	return nil
}

func numericURLValue(value string) bool {
	if value == "" || len(value) > 24 {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
