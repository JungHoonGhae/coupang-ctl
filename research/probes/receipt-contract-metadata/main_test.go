package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type syntheticOrderSource struct {
	documents map[string][]byte
}

func (s syntheticOrderSource) Fetch(_ context.Context, cursor *core.OrderCursor) ([]byte, error) {
	key := "initial"
	if cursor != nil {
		key = fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
	}
	document, ok := s.documents[key]
	if !ok {
		return nil, errors.New("synthetic document missing")
	}
	return document, nil
}

func TestSampleOrdersWalksBoundedPagesAndPreservesStateDiversity(t *testing.T) {
	source := syntheticOrderSource{documents: map[string][]byte{
		"initial": receiptSamplePage(`
			{"orderId":101,"orderedAt":1787929200000,"totalProductPrice":1000,"deliveryGroupList":[]},
			{"orderId":102,"orderedAt":1787929200000,"totalProductPrice":1000,"deliveryGroupList":[]},
			{"orderId":103,"orderedAt":1787929200000,"totalProductPrice":1000,"deliveryGroupList":[]}
		`, true),
		"2026/2": receiptSamplePage(`
			{"orderId":104,"orderedAt":1787929200000,"totalProductPrice":1000,"deliveryGroupList":[{"productList":[{"productId":1,"vendorItemId":2,"productName":"Synthetic returned","quantity":1,"returnReceiptQuantity":1,"unitPrice":1000,"discountedUnitPrice":1000}]}]},
			{"orderId":105,"orderedAt":1787929200000,"totalProductPrice":1000,"allCanceled":true,"deliveryGroupList":[]}
		`, false),
	}}

	samples, metadata := sampleOrders(context.Background(), source, orderSampleOptions{MaxPages: 2, MaxSamples: 3})
	if metadata.Status != "sampled_in_memory" || metadata.PagesScanned != 2 || metadata.OrdersScanned != 5 || !metadata.Completed || metadata.StoppedAtPageLimit || metadata.TerminalError != "" {
		t.Fatalf("unexpected sampling metadata: %#v", metadata)
	}
	if metadata.SelectedCount != 3 || metadata.SelectedStateCounts["ordinary"] != 1 || metadata.SelectedStateCounts["returned_units"] != 1 || metadata.SelectedStateCounts["fully_canceled"] != 1 {
		t.Fatalf("state diversity was not preserved: %#v", metadata)
	}
	if len(samples) != 3 {
		t.Fatalf("samples = %d, want 3", len(samples))
	}
}

func receiptSamplePage(orders string, hasNext bool) []byte {
	pagination := `"hasNext":false`
	if hasNext {
		pagination = `"hasNext":true,"nextYear":2026,"nextPageIndex":2`
	}
	return []byte(`{"orderList":[` + orders + `],` + pagination + `}`)
}
