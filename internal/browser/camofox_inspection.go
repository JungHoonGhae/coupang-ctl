package browser

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	productparser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

func (c *Camofox) FetchProductInspection(ctx context.Context, request core.ProductInspectRequest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	query := url.Values{}
	if request.ItemID != "" {
		query.Set("itemId", request.ItemID)
	}
	if request.VendorItemID != "" {
		query.Set("vendorItemId", request.VendorItemID)
	}
	target := "https://www.coupang.com/vp/products/" + request.ProductID
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	encoded, _ := json.Marshal(request)
	poll := strings.Replace(productDetailPoll, "/* SHARED_READER */", productInspectionReader, 1)
	// Each read owns its page. Closing it cancels its bounded reader and any
	// auxiliary requests; an inspection never changes a selected browser mode.
	expression := "(" + poll + ")(" + string(encoded) + ",'__coupangctl_detail')"
	r, err := c.reader("inspect").read(ctx, target, expression)
	if err != nil {
		return nil, err
	}
	if r.Search != nil || r.Page != nil || len(r.Category) != 0 || len(r.Inspection) == 0 || len(r.Inspection) > 128000 {
		return nil, ErrDocumentProtocol
	}
	parsed, err := productparser.ParseInspectionDocument(r.Inspection, request)
	if err != nil {
		return nil, ErrStructuredProductDataMissing
	}
	// Preserve field-level source/scope evidence. Only acquisition provenance
	// changes here, not the source document's identity, values or missing fields.
	var document map[string]json.RawMessage
	if json.Unmarshal(r.Inspection, &document) != nil {
		return nil, ErrDocumentProtocol
	}
	parsed.Coverage.Source = "camofox_product_document"
	document["coverage"], err = json.Marshal(parsed.Coverage)
	if err != nil {
		return nil, ErrDocumentProtocol
	}
	return json.Marshal(document)
}
