package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func syntheticCamofoxOrderPage() core.OrderPage {
	return core.OrderPage{
		Orders: []core.Order{{
			SourceRef: core.OrderSourceReference("synthetic-order"), PurchasedAt: "2026-09-03",
			TotalAmount: 25900, Currency: "KRW",
			Items: []core.OrderItem{{ProductID: "101", VendorItemID: "202", Name: "Synthetic item", Quantity: 1,
				UnitPrice: 25900, PaidPrice: 25900, CommerceKind: core.CommerceKindProductPurchase}},
		}},
		Next: &core.OrderCursor{Year: 2026, Page: 1},
	}
}

func TestCamofoxOrderDocumentValidation(t *testing.T) {
	for name, mutate := range map[string]func(*core.OrderPage){
		"valid":         func(*core.OrderPage) {},
		"raw reference": func(p *core.OrderPage) { p.Orders[0].SourceRef = "synthetic-order" },
		"invalid date":  func(p *core.OrderPage) { p.Orders[0].PurchasedAt = "2026-02-31" },
		"inconsistent timestamp": func(p *core.OrderPage) {
			v := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			p.Orders[0].PurchasedAtTime = &v
		},
		"negative amount":  func(p *core.OrderPage) { p.Orders[0].TotalAmount = -1 },
		"unknown currency": func(p *core.OrderPage) { p.Orders[0].Currency = "" },
		"cursor year":      func(p *core.OrderPage) { p.Next.Year = 2101 },
		"cursor page":      func(p *core.OrderPage) { p.Next.Page = -1 },
		"too many orders": func(p *core.OrderPage) {
			for len(p.Orders) <= 5 {
				p.Orders = append(p.Orders, p.Orders[0])
			}
		},
		"too many items": func(p *core.OrderPage) {
			for len(p.Orders[0].Items) <= 100 {
				p.Orders[0].Items = append(p.Orders[0].Items, p.Orders[0].Items[0])
			}
		},
		"invalid product":     func(p *core.OrderPage) { p.Orders[0].Items[0].ProductID = "invalid" },
		"zero quantity":       func(p *core.OrderPage) { p.Orders[0].Items[0].Quantity = 0 },
		"excess cancellation": func(p *core.OrderPage) { p.Orders[0].Items[0].CancelledQuantity = 2 },
		"excess return":       func(p *core.OrderPage) { p.Orders[0].Items[0].ReturnedQuantity = 2 },
		"unbounded text":      func(p *core.OrderPage) { p.Orders[0].Items[0].Name = strings.Repeat("a", 2001) },
		"nul text":            func(p *core.OrderPage) { p.Orders[0].Items[0].Name = "synthetic\x00" },
		"unknown delivery":    func(p *core.OrderPage) { p.Orders[0].Items[0].DeliveryStatus = "invented" },
		"unknown commerce":    func(p *core.OrderPage) { p.Orders[0].Items[0].CommerceKind = "invented" },
	} {
		t.Run(name, func(t *testing.T) {
			page := syntheticCamofoxOrderPage()
			mutate(&page)
			wire, err := json.Marshal(map[string]any{"status": "ok", "page": page})
			if err != nil {
				t.Fatal("synthetic encoding failed")
			}
			c := &Camofox{run: func(_ context.Context, _ string, mode string, _ core.QRLinkPresenter) ([]byte, error) {
				if mode != "orders" {
					t.Fatal("wrong document operation")
				}
				return wire, nil
			}}
			got, err := c.FetchPage(context.Background(), nil)
			if name != "valid" {
				if !errors.Is(err, ErrDocumentProtocol) || len(got.Orders) != 0 {
					t.Fatal("invalid order document accepted")
				}
				return
			}
			if err != nil || len(got.Orders) != 1 || got.Next == nil || got.Next.Page != 1 {
				t.Fatal("valid order document rejected")
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "synthetic-order") {
				t.Fatal("raw order identifier escaped")
			}
		})
	}
}

func TestCamofoxRejectsInvalidOrderCursorBeforeAcquisition(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("invalid cursor acquired browser")
		return nil, nil
	}}
	for _, cursor := range []core.OrderCursor{{Year: 1999}, {Year: 2101}, {Year: 2026, Page: -1}, {Year: 2026, Page: 1001}} {
		if _, err := c.FetchPage(context.Background(), &cursor); !errors.Is(err, ErrDocumentProtocol) {
			t.Fatal("invalid cursor accepted")
		}
	}
}

func TestCamofoxRejectsMixedDocumentEnvelopes(t *testing.T) {
	for _, payload := range []string{
		`{"status":"ok","page":{"orders":[]},"search":{"items":[],"no_results":true}}`,
		`{"status":"ok","page":{"orders":[]},"inspection":{}}`,
	} {
		c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(payload), nil
		}}
		if _, err := c.FetchPage(context.Background(), nil); !errors.Is(err, ErrDocumentProtocol) {
			t.Fatal("mixed order envelope accepted")
		}
	}
	for _, extra := range []string{`,"page":{"orders":[]}`, `,"inspection":{}`} {
		payload := `{"result":{"status":"ok","search":{"items":[],"no_results":true}` + extra + `}}`
		c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(payload), nil
		}}
		if _, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic"}); !errors.Is(err, ErrDocumentProtocol) {
			t.Fatal("mixed search envelope accepted")
		}
	}
}

func TestCamofoxQRLinksKeepTrustedDestinationAndApprovalCode(t *testing.T) {
	const direct = "https://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic"
	const app = "https://applink.coupang.com/open?url=https%3A%2F%2Flogin.coupang.com%2Flogin%2Fm%2Fqrcode%2Fbind.pang%3FqrCode%3Dsynthetic"
	for _, tc := range []struct {
		url, code string
		valid     bool
	}{
		{direct, "42", true}, {app, "09", true},
		{"https://evil.example/open?url=" + direct, "42", false},
		{"http://login.coupang.com/login/m/qrcode/bind.pang?qrCode=synthetic", "42", false},
		{"https://login.coupang.com/login/m/qrcode/bind.pang", "42", false},
		{direct + "#fragment", "42", false}, {direct, "4", false}, {direct, "xx", false},
	} {
		// The runtime uses camelCase for this private channel; it is not public JSON.
		wire, _ := json.Marshal(map[string]string{"url": tc.url, "approvalCode": tc.code})
		stream := "COUPANGCTL_QR " + string(wire) + "\nCOUPANGCTL_RESULT {\"status\":\"ok\"}\n"
		calls := 0
		_, err := consumeCamofoxOutput(context.Background(), strings.NewReader(stream), func(_ context.Context, link core.QRLoginLink) error {
			calls++
			if link.URL != tc.url || link.ApprovalCode != tc.code {
				t.Fatal("synthetic QR material changed")
			}
			return nil
		})
		if tc.valid {
			if err != nil || calls != 1 {
				t.Fatal("trusted synthetic QR link rejected")
			}
		} else if err == nil || calls != 0 {
			t.Fatal("untrusted QR material presented")
		}
	}
}
