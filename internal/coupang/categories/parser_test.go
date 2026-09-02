package categories_test

import (
	"errors"
	"testing"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	"github.com/JungHoonGhae/oss-coupangctl/internal/coupang/categories"
)

func TestParseProductCategoryKeepsOnlySourceNativeCategoryBreadcrumbs(t *testing.T) {
	document := []byte(`{"json_ld":[{"@type":"Product","name":"Synthetic private product"},{"@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Home","item":"https://www.coupang.com/"},{"@type":"ListItem","position":2,"name":"Synthetic broad","item":"https://www.coupang.com/np/categories/100"},{"@type":"ListItem","position":3,"name":"Synthetic leaf","item":"https://www.coupang.com/np/categories/200"}]}]}`)
	result, err := categories.ParseProductCategory(document)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != core.CategorySourceProductJSONLDBreadcrumb || len(result.Path) != 2 || result.Path[0].ID != "100" || result.Path[0].Name != "Synthetic broad" || result.Path[0].Position != 2 || result.Path[1].ID != "200" || result.Path[1].Name != "Synthetic leaf" {
		t.Fatalf("unexpected category: %#v", result)
	}
}

func TestParseProductCategoryRejectsExternalOrMalformedBreadcrumbs(t *testing.T) {
	for _, document := range [][]byte{
		[]byte(`{"json_ld":[]}`),
		[]byte(`{"json_ld":[{"@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Synthetic","item":"https://example.com/np/categories/100"}]}]}`),
		[]byte(`{"json_ld":[{"@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Synthetic","item":"https://www.coupang.com/np/categories/100"},{"@type":"ListItem","position":1,"name":"Synthetic","item":"https://www.coupang.com/np/categories/200"}]}]}`),
	} {
		if _, err := categories.ParseProductCategory(document); err == nil || (!errors.Is(err, categories.ErrCategoryDataMissing) && !errors.Is(err, core.ErrInvalidOrderData)) {
			t.Fatalf("malformed category document was accepted: %v", err)
		}
	}
}
