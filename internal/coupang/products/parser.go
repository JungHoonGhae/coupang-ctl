package products

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const maxProductDocumentBytes = 3 << 20

var ErrProductDataMissing = core.WithErrorCode("structured_product_data_missing", errors.New("structured product data missing"))

var (
	reviewEmailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	reviewPhonePattern = regexp.MustCompile(`\b0\d{1,2}[- .]?\d{3,4}[- .]?\d{4}\b`)
)

type searchEnvelope struct {
	Items     []productCardPayload `json:"items"`
	NoResults bool                 `json:"no_results"`
	Coverage  core.ProductCoverage `json:"coverage"`
}

type productCardPayload struct {
	scalarPresence map[string]bool
	ProductID      string                      `json:"product_id"`
	ItemID         string                      `json:"item_id"`
	VendorItemID   string                      `json:"vendor_item_id"`
	Name           string                      `json:"name"`
	URL            string                      `json:"url"`
	ImageURL       string                      `json:"image_url"`
	CurrentAmount  int64                       `json:"current_amount"`
	Currency       string                      `json:"currency"`
	OriginalAmount int64                       `json:"original_amount"`
	DiscountRate   int                         `json:"discount_rate"`
	Rating         float64                     `json:"rating"`
	ReviewCount    int                         `json:"review_count"`
	Rocket         bool                        `json:"rocket"`
	FreeShipping   bool                        `json:"free_shipping"`
	Coupon         bool                        `json:"coupon"`
	Sponsored      bool                        `json:"sponsored"`
	SearchPosition int                         `json:"search_position"`
	RankSource     string                      `json:"rank_source"`
	ReviewScope    string                      `json:"review_scope"`
	ObservedFields []string                    `json:"observed_fields"`
	FieldEvidence  []core.ProductFieldEvidence `json:"field_evidence"`
}

type inspectionEnvelope struct {
	Product            productCardPayload              `json:"product"`
	SelectedOptions    []string                        `json:"selected_options"`
	SelectedAttributes []core.ProductSelectedAttribute `json:"selected_attributes"`
	FieldEvidence      []core.ProductFieldEvidence     `json:"field_evidence"`
	Description        string                          `json:"description"`
	Specifications     []string                        `json:"specifications"`
	GalleryImages      []string                        `json:"gallery_images"`
	DetailImages       []string                        `json:"detail_images"`
	Delivery           core.ProductDelivery            `json:"delivery"`
	Benefits           []core.ProductBenefit           `json:"benefits"`
	Rating             core.ProductRatingSummary       `json:"rating"`
	Reviews            []core.ProductReview            `json:"reviews"`
	Coverage           core.ProductCoverage            `json:"coverage"`
	Warnings           []string                        `json:"warnings"`
}

type cartAddEnvelope struct {
	Attempted bool   `json:"attempted"`
	Added     bool   `json:"added"`
	Quantity  int    `json:"quantity"`
	CartURL   string `json:"cart_url"`
	Source    string `json:"source"`
}

func ParseSearchDocument(document []byte) ([]core.ProductCard, core.ProductCoverage, error) {
	var payload searchEnvelope
	if err := decodeDocument(document, &payload); err != nil ||
		(len(payload.Items) == 0 && !payload.NoResults) ||
		(len(payload.Items) > 0 && payload.NoResults) {
		return nil, core.ProductCoverage{}, ErrProductDataMissing
	}
	items := make([]core.ProductCard, 0, len(payload.Items))
	// These are calculated here, never trusted from the document's coverage.
	payload.Coverage.SourceNoResults = payload.NoResults
	payload.Coverage.SourceItems = len(payload.Items)
	payload.Coverage.RejectedItems = 0
	payload.Coverage.DuplicateItems = 0
	// Search extraction has no auxiliary quantity/review reads.
	payload.Coverage.BudgetOmittedFields = nil
	seen := map[string]bool{}
	for _, raw := range payload.Items {
		item, err := normalizeCard(raw)
		if err != nil {
			payload.Coverage.RejectedItems++
			continue
		}
		key := item.Reference.ProductID + "/" + item.Reference.ItemID + "/" + item.Reference.VendorItemID
		if seen[key] {
			payload.Coverage.DuplicateItems++
			continue
		}
		seen[key] = true
		items = append(items, item)
	}
	if len(items) == 0 && !payload.NoResults {
		return nil, core.ProductCoverage{}, ErrProductDataMissing
	}
	if payload.Coverage.Source == "" {
		payload.Coverage.Source = "coupang_search_document"
	}
	payload.Coverage.ObservedFields = normalizedStrings(payload.Coverage.ObservedFields, 50, 100)
	payload.Coverage.UnavailableFields = normalizedStrings(payload.Coverage.UnavailableFields, 50, 100)
	return items, payload.Coverage, nil
}

func ParseInspectionDocument(document []byte, request core.ProductInspectRequest) (core.ProductInspection, error) {
	var payload inspectionEnvelope
	if err := decodeDocument(document, &payload); err != nil {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	if !declaredInspectionFieldsPresent(document, payload.Coverage.ObservedFields) {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	if payload.Rating.Average < 0 || payload.Rating.Average > 5 || payload.Rating.Count < 0 {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	for _, count := range payload.Rating.Distribution {
		if count < 0 {
			return core.ProductInspection{}, ErrProductDataMissing
		}
	}
	product, err := normalizeCard(payload.Product)
	if err != nil || product.Reference.ProductID != request.ProductID ||
		(request.ItemID != "" && product.Reference.ItemID != request.ItemID) ||
		(request.VendorItemID != "" && product.Reference.VendorItemID != request.VendorItemID) {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	payload.Description = normalizedText(payload.Description, 5000)
	payload.SelectedOptions = normalizedStrings(payload.SelectedOptions, 20, 300)
	payload.Specifications = normalizedStrings(payload.Specifications, 50, 300)
	payload.GalleryImages = normalizedImageURLs(payload.GalleryImages, 50)
	payload.DetailImages = normalizedImageURLs(payload.DetailImages, 50)
	payload.Delivery.Summary = normalizedText(payload.Delivery.Summary, 500)
	payload.Benefits = normalizeBenefits(payload.Benefits)
	payload.Reviews = normalizeReviews(payload.Reviews)
	payload.Coverage.ObservedFields = normalizedStrings(payload.Coverage.ObservedFields, 80, 100)
	payload.Coverage.UnavailableFields = normalizedStrings(payload.Coverage.UnavailableFields, 80, 100)
	if len(payload.Coverage.BudgetOmittedFields) > 2 {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	for _, field := range payload.Coverage.BudgetOmittedFields {
		if field != "quantity_info" && field != "reviews" {
			return core.ProductInspection{}, ErrProductDataMissing
		}
		// Preserve an explicit budget omission as missing evidence even if a
		// malformed summary omitted it from unavailable_fields.
	}
	payload.Coverage.BudgetOmittedFields = normalizedStrings(payload.Coverage.BudgetOmittedFields, 2, 40)
	payload.Coverage.UnavailableFields = normalizedStrings(append(append([]string{}, payload.Coverage.BudgetOmittedFields...), payload.Coverage.UnavailableFields...), 80, 100)
	hasCardBenefit := false
	for _, benefit := range payload.Benefits {
		if benefit.Kind == "card" {
			hasCardBenefit = true
			break
		}
	}
	reconcileInspectionCoverage(&payload.Coverage, len(payload.SelectedOptions) > 0, hasCardBenefit)
	if !validFieldEvidence(payload.FieldEvidence, product.Reference, payload.Coverage.ObservedFields) {
		return core.ProductInspection{}, ErrProductDataMissing
	}
	if payload.Coverage.Source == "" {
		payload.Coverage.Source = "coupang_product_document"
	}
	result := core.ProductInspection{
		Product: product, SelectedOptions: payload.SelectedOptions, Description: payload.Description, Specifications: payload.Specifications,
		SelectedAttributes: payload.SelectedAttributes,
		GalleryImages:      payload.GalleryImages, DetailImages: payload.DetailImages,
		Delivery: payload.Delivery, Benefits: payload.Benefits, Rating: payload.Rating,
		Reviews: payload.Reviews, Coverage: payload.Coverage, FieldEvidence: payload.FieldEvidence,
		Warnings: normalizedStrings(payload.Warnings, 20, 300),
	}
	hasSelectedAttributes := len(result.SelectedAttributes) > 0
	if hasSelectedAttributes {
		if _, ok := result.SelectedAttributesEvidence(); !ok {
			return core.ProductInspection{}, ErrProductDataMissing
		}
	} else {
		// A declared fact without its value is not merely an empty option tuple.
		for _, e := range result.FieldEvidence {
			if e.Field == "selected_attributes" {
				return core.ProductInspection{}, ErrProductDataMissing
			}
		}
		result.SelectedAttributes = []core.ProductSelectedAttribute{}
	}
	result.Coverage.ObservedFields = removeString(result.Coverage.ObservedFields, "selected_attributes")
	result.Coverage.UnavailableFields = removeString(result.Coverage.UnavailableFields, "selected_attributes")
	if hasSelectedAttributes {
		result.Coverage.ObservedFields = append(result.Coverage.ObservedFields, "selected_attributes")
	} else {
		result.Coverage.UnavailableFields = append(result.Coverage.UnavailableFields, "selected_attributes")
	}
	return result, nil
}

func reconcileInspectionCoverage(coverage *core.ProductCoverage, selectedOptionsObserved, cardBenefitObserved bool) {
	fields := []struct {
		name     string
		observed bool
	}{
		{name: "selected_options", observed: selectedOptionsObserved},
		{name: "card_benefit", observed: cardBenefitObserved},
	}
	for _, field := range fields {
		coverage.ObservedFields = removeString(coverage.ObservedFields, field.name)
		coverage.UnavailableFields = removeString(coverage.UnavailableFields, field.name)
	}
	observed := make([]string, 0, len(coverage.ObservedFields)+len(fields))
	unavailable := make([]string, 0, len(coverage.UnavailableFields)+len(fields))
	for _, field := range fields {
		if field.observed {
			observed = append(observed, field.name)
		} else {
			unavailable = append(unavailable, field.name)
		}
	}
	coverage.ObservedFields = normalizedStrings(append(observed, coverage.ObservedFields...), 80, 100)
	coverage.UnavailableFields = normalizedStrings(append(unavailable, coverage.UnavailableFields...), 80, 100)
}

func removeString(values []string, unwanted string) []string {
	result := values[:0]
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}

func ParseCartAddDocument(document []byte, request core.CartAddRequest) (core.CartAddResult, error) {
	var payload cartAddEnvelope
	if err := decodeDocument(document, &payload); err != nil || !payload.Attempted || payload.Quantity != request.Quantity || payload.CartURL != "https://cart.coupang.com/cartView.pang" || payload.Source != "coupang_product_page" {
		return core.CartAddResult{}, ErrProductDataMissing
	}
	warnings := []string{}
	if !payload.Added {
		warnings = append(warnings, "the add-to-cart control was pressed but the resulting cart state could not be verified; do not retry automatically")
	}
	return core.CartAddResult{
		Attempted: true, Added: payload.Added, Verified: payload.Added,
		Product:  core.ProductReference{ProductID: request.ProductID, ItemID: request.ItemID, VendorItemID: request.VendorItemID},
		Quantity: payload.Quantity, CartURL: payload.CartURL, Source: payload.Source, Warnings: warnings,
	}, nil
}

func decodeDocument(document []byte, target any) error {
	if len(document) == 0 || len(document) > maxProductDocumentBytes || !json.Valid(document) {
		return ErrProductDataMissing
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	return decoder.Decode(target)
}

func normalizeCard(raw productCardPayload) (core.ProductCard, error) {
	observedFields := normalizedStrings(raw.ObservedFields, 30, 100)
	for _, field := range observedFields {
		if _, scalar := cardScalarPaths[field]; scalar && !raw.scalarPresence[field] {
			return core.ProductCard{}, ErrProductDataMissing
		}
	}
	name := normalizedText(raw.Name, 300)
	if !core.NumericProductIdentifier(raw.ProductID) || name == "" || !validProductURL(raw.URL, raw.ProductID) ||
		!matchingOptionURL(raw.URL, raw.ItemID, raw.VendorItemID) {
		return core.ProductCard{}, ErrProductDataMissing
	}
	if raw.ItemID != "" && !core.NumericProductIdentifier(raw.ItemID) || raw.VendorItemID != "" && !core.NumericProductIdentifier(raw.VendorItemID) {
		return core.ProductCard{}, ErrProductDataMissing
	}
	if raw.CurrentAmount < 0 || raw.OriginalAmount < 0 || raw.DiscountRate < 0 || raw.DiscountRate > 100 || raw.Rating < 0 || raw.Rating > 5 || raw.ReviewCount < 0 || raw.SearchPosition < 0 {
		return core.ProductCard{}, ErrProductDataMissing
	}
	if !validFieldEvidence(raw.FieldEvidence, core.ProductReference{ProductID: raw.ProductID, ItemID: raw.ItemID, VendorItemID: raw.VendorItemID}, observedFields) {
		return core.ProductCard{}, ErrProductDataMissing
	}
	imageURL := ""
	if validImageURL(raw.ImageURL) {
		imageURL = raw.ImageURL
	}
	return core.ProductCard{
		Reference: core.ProductReference{ProductID: raw.ProductID, ItemID: raw.ItemID, VendorItemID: raw.VendorItemID},
		Name:      name, URL: raw.URL, ImageURL: imageURL,
		Price:  core.ProductPrice{CurrentAmount: raw.CurrentAmount, OriginalAmount: raw.OriginalAmount, DiscountRate: raw.DiscountRate, Currency: normalizedCurrency(raw.Currency)},
		Rating: raw.Rating, ReviewCount: raw.ReviewCount, Rocket: raw.Rocket, FreeShipping: raw.FreeShipping,
		Coupon: raw.Coupon, Sponsored: raw.Sponsored,
		SearchPosition: raw.SearchPosition, RankSource: normalizedText(raw.RankSource, 60),
		ReviewScope: normalizedText(raw.ReviewScope, 60), ObservedFields: observedFields, FieldEvidence: raw.FieldEvidence,
	}, nil
}

func validProductURL(raw, productID string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.User == nil && parsed.Scheme == "https" && parsed.Host == "www.coupang.com" && parsed.Path == "/vp/products/"+productID
}

// A canonical product URL may omit option parameters. If present, they must
// identify the same observed option, not a different or ambiguous one.
func matchingOptionURL(raw, itemID, vendorItemID string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for key, expected := range map[string]string{"itemId": itemID, "vendorItemId": vendorItemID} {
		if values, present := query[key]; present {
			if len(values) != 1 || expected == "" || values[0] != expected {
				return false
			}
		}
	}
	return true
}

func validImageURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "coupangcdn.com" || strings.HasSuffix(host, ".coupangcdn.com")
}

func normalizedImageURLs(values []string, limit int) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if len(result) >= limit || seen[value] || !validImageURL(value) {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizedText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || !utf8.ValidString(value) {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}

func normalizedStrings(values []string, limit, maxRunes int) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = normalizedText(value, maxRunes)
		if value == "" || seen[value] || len(result) >= limit {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeBenefits(values []core.ProductBenefit) []core.ProductBenefit {
	result := make([]core.ProductBenefit, 0, len(values))
	for _, value := range values {
		value.Kind = normalizedText(value.Kind, 30)
		value.Title = normalizedText(value.Title, 200)
		value.Description = normalizedText(value.Description, 500)
		value.Condition = normalizedText(value.Condition, 300)
		value.Source = normalizedText(value.Source, 60)
		if len(result) >= 20 || value.Title == "" || value.Source == "" {
			continue
		}
		switch value.Kind {
		case "coupon", "card", "cashback", "promotion":
			result = append(result, value)
		}
	}
	return result
}

func normalizeReviews(values []core.ProductReview) []core.ProductReview {
	result := make([]core.ProductReview, 0, len(values))
	for _, value := range values {
		if len(result) >= 20 || value.Rating < 0 || value.Rating > 5 || value.HelpfulCount < 0 {
			continue
		}
		value.Content = normalizedText(value.Content, 1500)
		value.ObservedFields = normalizedStrings(value.ObservedFields, 10, 100)
		value.Content = redactReviewPII(value.Content)
		value.CreatedDate = normalizedText(value.CreatedDate, 40)
		value.ImageURLs = normalizedImageURLs(value.ImageURLs, 10)
		if value.Content == "" && value.Rating == 0 && len(value.ImageURLs) == 0 {
			continue
		}
		result = append(result, value)
	}
	return result
}

func redactReviewPII(value string) string {
	value = reviewEmailPattern.ReplaceAllString(value, "[redacted-email]")
	return reviewPhonePattern.ReplaceAllString(value, "[redacted-phone]")
}
