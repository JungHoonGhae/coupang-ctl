package browser

import (
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"net/url"
	"slices"
	"strings"
)

// A narrow allowlist: no page body, cookies, arbitrary script, or arbitrary URL.
type searchDocumentItem struct {
	ProductID      string                      `json:"product_id"`
	ItemID         string                      `json:"item_id,omitempty"`
	VendorItemID   string                      `json:"vendor_item_id,omitempty"`
	Name           string                      `json:"name"`
	URL            string                      `json:"url"`
	ImageURL       string                      `json:"image_url,omitempty"`
	CurrentAmount  *int64                      `json:"current_amount,omitempty"`
	Currency       string                      `json:"currency,omitempty"`
	FieldEvidence  []core.ProductFieldEvidence `json:"field_evidence,omitempty"`
	ObservedFields []string                    `json:"observed_fields"`
	SearchPosition int                         `json:"search_position"`
	RankSource     string                      `json:"rank_source"`
}

type searchDocument struct {
	Items     []searchDocumentItem `json:"items"`
	NoResults bool                 `json:"no_results,omitempty"`
}

func (d searchDocument) Validate() error {
	if (len(d.Items) == 0 && !d.NoResults) || (len(d.Items) > 0 && d.NoResults) || len(d.Items) > 60 {
		return ErrDocumentProtocol
	}
	for _, item := range d.Items {
		if !core.NumericProductIdentifier(item.ProductID) || !validDocumentNumericID(item.ItemID) || !validDocumentNumericID(item.VendorItemID) || strings.TrimSpace(item.Name) == "" || !validDocumentText(item.Name, 2000) || item.SearchPosition < 1 || item.SearchPosition > 10000 || (item.RankSource != "coupang_search_order" && item.RankSource != "dom_search_list_order") {
			return ErrDocumentProtocol
		}
		priceAvailable := slices.Contains(item.ObservedFields, "price.current_amount")
		imageAvailable := slices.Contains(item.ObservedFields, "image_url")
		if imageAvailable != (item.ImageURL != "") || (item.ImageURL != "" && core.ValidateProductImageURL(item.ImageURL) != nil) {
			return ErrDocumentProtocol
		}
		if priceAvailable != (item.CurrentAmount != nil) || (item.CurrentAmount != nil && (*item.CurrentAmount < 0 || *item.CurrentAmount > 1_000_000_000_000)) {
			return ErrDocumentProtocol
		}
		if item.Currency != "" && (!priceAvailable || len(item.Currency) != 3 || strings.IndexFunc(item.Currency, func(r rune) bool { return r < 'A' || r > 'Z' }) != -1) {
			return ErrDocumentProtocol
		}
		u, err := url.Parse(item.URL)
		if err != nil || u.Scheme != "https" || u.Host != "www.coupang.com" || u.User != nil || u.Fragment != "" || u.Path != "/vp/products/"+item.ProductID {
			return ErrDocumentProtocol
		}
		for key, values := range u.Query() {
			if len(values) != 1 || (key != "itemId" && key != "vendorItemId") {
				return ErrDocumentProtocol
			}
		}
		if u.Query().Get("itemId") != item.ItemID || u.Query().Get("vendorItemId") != item.VendorItemID {
			return ErrDocumentProtocol
		}
		if len(item.ObservedFields) > 4 {
			return ErrDocumentProtocol
		}
		for _, field := range item.ObservedFields {
			if field != "name" && field != "search_position" && field != "price.current_amount" && field != "image_url" {
				return ErrDocumentProtocol
			}
		}
		if len(item.FieldEvidence) > 3 {
			return ErrDocumentProtocol
		}
		reference := core.ProductReference{ProductID: item.ProductID, ItemID: item.ItemID, VendorItemID: item.VendorItemID}
		seen := make(map[string]bool)
		for _, evidence := range item.FieldEvidence {
			if (evidence.Field != "name" && evidence.Field != "price.current_amount" && evidence.Field != "image_url") || seen[evidence.Field] || !slices.Contains(item.ObservedFields, evidence.Field) || !evidence.ValidFor(reference) {
				return ErrDocumentProtocol
			}
			seen[evidence.Field] = true
		}
		if imageAvailable && !seen["image_url"] {
			return ErrDocumentProtocol
		}
	}
	return nil
}
