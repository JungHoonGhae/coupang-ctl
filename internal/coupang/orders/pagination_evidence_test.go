package orders_test

import (
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
)

func TestOrderPaginationRequiresExplicitBooleanEndEvidence(t *testing.T) {
	for name, document := range map[string]string{
		"missing":                     `{"orderList":[]}`,
		"null":                        `{"orderList":[],"hasNext":null}`,
		"string_false":                `{"orderList":[],"hasNext":"false"}`,
		"string_true":                 `{"orderList":[],"hasNext":"true","nextYear":2026,"nextPageIndex":1}`,
		"number":                      `{"orderList":[],"hasNext":0}`,
		"empty_object":                `{"orderList":[],"orderPagination":{}}`,
		"null_container":              `{"orderList":[],"hasNext":false,"orderPagination":null}`,
		"array_container":             `{"orderList":[],"hasNext":false,"orderPagination":[]}`,
		"string_container":            `{"orderList":[],"hasNext":false,"orderPagination":"synthetic"}`,
		"continuation_missing_cursor": `{"orderList":[],"hasNext":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			page, err := coupangorders.ParseOrderDocument([]byte(document))
			if !errors.Is(err, core.ErrInvalidOrderData) {
				t.Fatalf("ambiguous pagination accepted as end: error=%v next=%v", err, page.Next)
			}
			if page.Orders != nil || page.Next != nil {
				t.Fatal("invalid pagination returned partially usable page")
			}
		})
	}
}

func TestOrderPaginationExplicitEmptyEndAndEmptyContinuationStayDistinct(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, next := range []bool{false, true} {
			pagination := `"hasNext":false`
			if next {
				pagination = `"hasNext":true,"nextYear":2026,"nextPageIndex":1`
			}
			if nested {
				pagination = `"orderPagination":{` + pagination + `}`
			}
			page, err := coupangorders.ParseOrderDocument([]byte(`{"orderList":[],` + pagination + `}`))
			if err != nil || page.Orders == nil || len(page.Orders) != 0 || (page.Next != nil) != next {
				t.Fatal("explicit empty pagination lost source semantics", err)
			}
		}
	}
}
