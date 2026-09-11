package orders_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
)

func TestOrderPartialResponseCannotBecomeAUsablePage(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		for _, partial := range []string{`true`, `null`, `"false"`, `0`, `[]`, `{}`} {
			for _, pagination := range []string{`"hasNext":false`, `"hasNext":true,"nextYear":2026,"nextPageIndex":2`} {
				t.Run(fmt.Sprintf("bootstrap=%t/partial=%s/%s", bootstrap, partial, pagination), func(t *testing.T) {
					document := `{"orderList":[],"partial":` + partial + `,` + pagination + `}`
					if bootstrap {
						document = `{"props":{"pageProps":{"domains":{"desktopOrder":` + document + `}}}}`
					}
					page, err := coupangorders.ParseOrderDocument([]byte(document))
					if err == nil || page.Orders != nil || page.Next != nil {
						t.Fatal("source partial/ambiguous page accepted as complete page evidence")
					}
					want := core.ErrInvalidOrderData
					if partial == "true" {
						want = core.ErrPartialOrderData
					}
					if !errors.Is(err, want) {
						t.Fatal("source partial and malformed responses were not distinguished", err)
					}
				})
			}
		}
	}
}

func TestOrderExplicitNonPartialResponsePreservesPagination(t *testing.T) {
	for _, extra := range []string{``, `,"partial":false`} {
		page, err := coupangorders.ParseOrderDocument([]byte(`{"orderList":[],"hasNext":true,"nextYear":2026,"nextPageIndex":2` + extra + `}`))
		if err != nil || page.Orders == nil || page.Next == nil || page.Next.Year != 2026 || page.Next.Page != 2 {
			t.Fatal("ordinary source response lost pagination", err)
		}
	}
}
