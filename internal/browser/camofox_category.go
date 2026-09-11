package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/coupang/categories"
)

func (c *Camofox) FetchProductCategory(ctx context.Context, reference core.ProductReference) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reference.ProductID == "" || !validDocumentNumericID(reference.ProductID) || !validDocumentNumericID(reference.VendorItemID) {
		return nil, ErrDocumentProtocol
	}
	target := "https://www.coupang.com/vp/products/" + reference.ProductID
	if reference.VendorItemID != "" {
		target += "?vendorItemId=" + url.QueryEscape(reference.VendorItemID)
	}
	u, _ := json.Marshal(target)
	expression := "(" + strings.Replace(categoryPageReader, "export function", "function", 1) + ")(" + string(u) + ")"
	result, err := c.reader("category").read(ctx, target, expression)
	if err != nil {
		return nil, err
	}
	if len(result.Category) == 0 || len(result.Category) > 32<<10 || result.Search != nil || result.Page != nil || len(result.Inspection) != 0 {
		return nil, ErrDocumentProtocol
	}
	var document struct {
		Documents []json.RawMessage `json:"json_ld"`
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Category))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || decoder.Decode(new(any)) != io.EOF || document.Documents == nil {
		return nil, ErrDocumentProtocol
	}
	if len(document.Documents) == 0 {
		return []byte(`{"json_ld":[]}`), nil
	}
	parsed, err := categories.ParseProductCategory(result.Category)
	if err != nil {
		return nil, ErrStructuredCategoryDataMissing
	}
	// Re-encode only validated category nodes, not arbitrary JSON-LD fields.
	rows := make([]map[string]any, 0, len(parsed.Path))
	for _, node := range parsed.Path {
		rows = append(rows, map[string]any{"@type": "ListItem", "position": node.Position, "name": node.Name, "item": "https://www.coupang.com/np/categories/" + node.ID})
	}
	return json.Marshal(map[string]any{"json_ld": []any{map[string]any{"@type": "BreadcrumbList", "itemListElement": rows}}})
}
