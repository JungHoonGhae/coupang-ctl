package products

import (
	"bytes"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var htmlSearchDenial = regexp.MustCompile(`(?i)access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근`)
var htmlSearchChallenge = regexp.MustCompile(`(?i)captcha|보안문자|자동입력방지`)
var htmlSearchPrice = regexp.MustCompile(`^(?:[0-9]+|[0-9]{1,3}(?:,[0-9]{3})+)$`)

// NormalizeSearchHTML is a static, structured-data adapter, not a script
// executor or a general page scraper. Only a list bound to the requested
// search identity can produce the canonical search envelope. No DOM-card
// fallback is attempted here; browser-rendered extraction remains separate.
func NormalizeSearchHTML(document []byte, targetURL string, capturedAt time.Time) ([]byte, error) {
	target, ok := htmlSearchIdentity(targetURL)
	if !ok || capturedAt.IsZero() || len(document) == 0 || len(document) > maxProductDocumentBytes || !utf8.Valid(document) {
		return nil, ErrProductDataMissing
	}
	root, err := html.Parse(bytes.NewReader(document))
	if err != nil {
		return nil, ErrProductDataMissing
	}
	var scripts []string
	var text strings.Builder
	password, productAnchor := false, false
	nodes := 0
	for node := range root.Descendants() {
		nodes++
		if nodes > 100000 {
			return nil, ErrProductDataMissing
		}
		if inertSearchHTMLNode(node) {
			continue
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script":
				if strings.EqualFold(strings.TrimSpace(htmlAttribute(node, "type")), "application/ld+json") {
					var content strings.Builder
					for child := node.FirstChild; child != nil; child = child.NextSibling {
						if child.Type == html.TextNode {
							content.WriteString(child.Data)
						}
					}
					if content.Len() <= 1000000 {
						scripts = append(scripts, content.String())
					}
					if len(scripts) > 100 {
						return nil, ErrProductDataMissing
					}
				}
			case "input":
				password = password || strings.EqualFold(htmlAttribute(node, "type"), "password")
			case "a":
				productAnchor = productAnchor || strings.Contains(htmlAttribute(node, "href"), "/vp/products/")
			}
		} else if node.Type == html.TextNode && (node.Parent == nil || (node.Parent.Data != "script" && node.Parent.Data != "style")) {
			text.WriteString(node.Data)
			text.WriteByte(' ')
		}
	}
	if htmlSearchChallenge.MatchString(text.String()) {
		return nil, core.ErrBrowserAccessDenied
	}
	if password {
		return nil, core.ErrAuthenticationRequired
	}
	if htmlSearchDenial.MatchString(text.String()) {
		return nil, core.ErrBrowserAccessDenied
	}

	lists := make(map[string]map[string]any)
	var walk func(any, int, bool)
	walk = func(raw any, depth int, ownerBound bool) {
		if depth > 8 {
			return
		}
		if values, ok := raw.([]any); ok {
			for _, value := range values[:min(len(values), 100)] {
				walk(value, depth+1, ownerBound)
			}
			return
		}
		node, ok := raw.(map[string]any)
		if !ok {
			return
		}
		if jsonLDType(node, "ItemList") {
			_, hasURL := node["url"]
			if searchListMatches(node["url"], target) || (ownerBound && !hasURL) {
				encoded, _ := json.Marshal(node)
				lists[string(encoded)] = node
			}
			return
		}
		if (jsonLDType(node, "SearchResultsPage") || jsonLDType(node, "CollectionPage")) && searchListMatches(node["url"], target) {
			walk(node["mainEntity"], depth+1, true)
		}
		walk(node["@graph"], depth+1, false)
	}
	for _, script := range scripts {
		var data any
		decoder := json.NewDecoder(strings.NewReader(script))
		decoder.UseNumber()
		if json.Valid([]byte(script)) && decoder.Decode(&data) == nil {
			walk(data, 0, false)
		}
	}
	if len(lists) != 1 {
		return nil, ErrProductDataMissing
	}
	var list map[string]any
	for _, value := range lists {
		list = value
	}
	entries, ok := list["itemListElement"].([]any)
	if !ok {
		return nil, ErrProductDataMissing
	}
	count, countIsNumber := list["numberOfItems"].(json.Number)
	_, explicitZero := searchJSONInteger(count, 0)
	if countIsNumber && explicitZero {
		if len(entries) != 0 || productAnchor {
			return nil, ErrProductDataMissing
		}
		return []byte(`{"items":[],"no_results":true,"coverage":{"source":"coupang_search_html_jsonld"}}`), nil
	}
	if len(entries) == 0 {
		return nil, ErrProductDataMissing
	}
	items := make([]map[string]any, 0, min(len(entries), 60))
	for index, entry := range entries[:min(len(entries), 60)] {
		mapped, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		product := mapped
		if !jsonLDType(product, "Product") {
			product, _ = mapped["item"].(map[string]any)
		}
		if !jsonLDType(product, "Product") {
			continue
		}
		position := int64(index + 1)
		if value, exists := mapped["position"]; exists {
			number, ok := value.(json.Number)
			if !ok {
				continue
			}
			var valid bool
			position, valid = searchJSONInteger(number, 10000)
			if !valid || position < 1 {
				continue
			}
		}
		if item := normalizeSearchJSONLDProduct(product, position, capturedAt); item != nil {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, ErrProductDataMissing
	}
	encoded, err := json.Marshal(map[string]any{"items": items, "coverage": map[string]any{"source": "coupang_search_html_jsonld"}})
	if err != nil {
		return nil, ErrProductDataMissing
	}
	if _, _, err := ParseSearchDocument(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func htmlAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func inertSearchHTMLNode(node *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		switch parent.Data {
		case "template", "noscript", "svg", "math":
			return true
		}
	}
	return false
}

func jsonLDType(node map[string]any, wanted string) bool {
	if node["@type"] == wanted {
		return true
	}
	if types, ok := node["@type"].([]any); ok {
		for _, value := range types {
			if value == wanted {
				return true
			}
		}
	}
	return false
}

func htmlSearchIdentity(raw string) ([3]string, bool) {
	var identity [3]string
	base, _ := url.Parse("https://www.coupang.com")
	u, err := base.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != base.Host || u.Path != "/np/search" || u.User != nil || u.Fragment != "" {
		return identity, false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return identity, false
	}
	for _, key := range []string{"q", "sorter", "page"} {
		if len(query[key]) > 1 {
			return identity, false
		}
	}
	identity = [3]string{query.Get("q"), query.Get("sorter"), query.Get("page")}
	if strings.TrimSpace(identity[0]) == "" || utf8.RuneCountInString(identity[0]) > 200 {
		return identity, false
	}
	if !query.Has("sorter") {
		identity[1] = "scoreDesc"
	}
	switch identity[1] {
	case "scoreDesc", "saleCountDesc", "latestAsc", "salePriceAsc", "salePriceDesc":
	default:
		return identity, false
	}
	if !query.Has("page") {
		identity[2] = "1"
	}
	page, err := strconv.Atoi(identity[2])
	return identity, err == nil && page >= 1 && page <= 100 && strconv.Itoa(page) == identity[2]
}

func searchListMatches(raw any, expected [3]string) bool {
	value, ok := raw.(string)
	if !ok {
		return false
	}
	identity, ok := htmlSearchIdentity(value)
	return ok && identity == expected
}

func canonicalSearchProduct(raw string) (core.ProductReference, string, bool) {
	base, _ := url.Parse("https://www.coupang.com")
	u, err := base.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != base.Host || u.User != nil {
		return core.ProductReference{}, "", false
	}
	id := strings.TrimPrefix(u.Path, "/vp/products/")
	if id == u.Path || !core.NumericProductIdentifier(id) {
		return core.ProductReference{}, "", false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return core.ProductReference{}, "", false
	}
	for _, key := range []string{"itemId", "vendorItemId"} {
		if len(query[key]) > 1 || (query.Has(key) && !core.NumericProductIdentifier(query.Get(key))) {
			return core.ProductReference{}, "", false
		}
	}
	ref := core.ProductReference{ProductID: id, ItemID: query.Get("itemId"), VendorItemID: query.Get("vendorItemId")}
	u.RawQuery, u.Fragment = "", ""
	clean := make(url.Values)
	if ref.ItemID != "" {
		clean.Set("itemId", ref.ItemID)
	}
	if ref.VendorItemID != "" {
		clean.Set("vendorItemId", ref.VendorItemID)
	}
	u.RawQuery = clean.Encode()
	return ref, u.String(), true
}

func normalizeSearchJSONLDProduct(product map[string]any, position int64, capturedAt time.Time) map[string]any {
	name, _ := product["name"].(string)
	name = normalizedText(name, 300)
	if name == "" {
		return nil
	}
	offer, _ := product["offers"].(map[string]any)
	if offers, ok := product["offers"].([]any); ok && len(offers) == 1 {
		offer, _ = offers[0].(map[string]any)
	}
	rawURL, _ := product["url"].(string)
	if rawURL == "" {
		rawURL, _ = offer["url"].(string)
	}
	reference, canonicalURL, ok := canonicalSearchProduct(rawURL)
	if !ok {
		return nil
	}
	productRef := core.ProductReference{ProductID: reference.ProductID}
	evidence := []core.ProductFieldEvidence{{Field: "name", Source: "json_ld", Locator: "jsonld.Product.name", Method: "native_field", Provenance: "observed", Scope: "product", Reference: productRef, CapturedAt: capturedAt}}
	fields := []string{"name", "search_position"}
	item := map[string]any{"product_id": reference.ProductID, "item_id": reference.ItemID, "vendor_item_id": reference.VendorItemID, "name": name, "url": canonicalURL, "search_position": position, "rank_source": "coupang_search_order"}
	amount, method, present := searchHTMLPrice(offer["price"])
	if present {
		scope, priceRef := "product", productRef
		if offerURL, ok := offer["url"].(string); ok && (reference.ItemID != "" || reference.VendorItemID != "") {
			if offerRef, _, valid := canonicalSearchProduct(offerURL); valid && offerRef == reference {
				scope, priceRef = "selected_option", reference
			}
		}
		provenance := "observed"
		if method != "native_field" {
			provenance = "derived"
		}
		currency, _ := offer["priceCurrency"].(string)
		if normalizedCurrency(currency) != currency {
			currency = ""
		}
		item["current_amount"], item["currency"] = amount, normalizedCurrency(currency)
		fields = append(fields, "price.current_amount")
		evidence = append(evidence, core.ProductFieldEvidence{Field: "price.current_amount", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: method, Provenance: provenance, Scope: scope, Reference: priceRef, CapturedAt: capturedAt})
	}
	item["observed_fields"], item["field_evidence"] = fields, evidence
	return item
}

func searchHTMLPrice(raw any) (int64, string, bool) {
	var amount int64
	var err error
	method := "native_field"
	switch value := raw.(type) {
	case json.Number:
		var valid bool
		amount, valid = searchJSONInteger(value, 1000000000000)
		if !valid {
			return 0, method, false
		}
	case string:
		method = "numeric_parse"
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "원"))
		if !htmlSearchPrice.MatchString(value) {
			return 0, method, false
		}
		amount, err = strconv.ParseInt(strings.ReplaceAll(value, ",", ""), 10, 64)
	default:
		return 0, method, false
	}
	return amount, method, err == nil && amount >= 0 && amount <= 1000000000000
}

func searchJSONInteger(value json.Number, maximum int64) (int64, bool) {
	number, err := value.Float64()
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > float64(maximum) || math.Trunc(number) != number {
		return 0, false
	}
	return int64(number), true
}
