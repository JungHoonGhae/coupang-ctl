package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
)

func TestCollectCandidatePathsEmitsMetadataWithoutValues(t *testing.T) {
	order := map[string]any{
		"orderId":     "private-order-id",
		"productName": "private-product-name",
		"refundInfo": map[string]any{
			"status": "private-refund-status",
			"amount": json.Number("12345"),
			"events": []any{map[string]any{"kind": "private-event-value"}},
		},
	}

	local := collectCandidatePaths(order, "order", false, 0)
	if _, ok := local["order.orderId"]; ok {
		t.Fatal("unrelated identifier path must not be collected")
	}
	amount := local["order.refundInfo.amount"]
	if amount == nil || amount.numericPositive != 1 {
		t.Fatalf("expected positive-number metadata, got %#v", amount)
	}
	if _, ok := local["order.refundInfo.events[].kind"]; ok {
		t.Fatal("unrelated descendant metadata must not be collected")
	}
	encoded, err := json.Marshal(local)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"private-order-id", "private-product-name", "private-refund-status", "12345", "private-event-value"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("metadata output leaked source value %q", privateValue)
		}
	}
}

func TestClassifyReadErrorDoesNotExposeWrappedDetails(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: fmt.Errorf("private detail: %w", browser.ErrBrowserAccessDenied), want: "browser_access_denied"},
		{err: fmt.Errorf("private detail: %w", browser.ErrAuthenticationRequired), want: "authentication_required"},
		{err: fmt.Errorf("private detail: %w", context.DeadlineExceeded), want: "timeout"},
		{err: errors.New("private transport detail"), want: "authenticated_order_read_failed"},
	}
	for _, test := range tests {
		if got := classifyReadError(test.err); got != test.want {
			t.Fatalf("classifyReadError() = %q, want %q", got, test.want)
		}
	}
}

func TestFinalizeReportPreservesRedactedPartialEvidenceOnTerminalReadError(t *testing.T) {
	aggregated := map[string]*pathEvidence{}
	mergeSample(aggregated, collectCandidatePaths(map[string]any{
		"refundInfo": map[string]any{"status": "private-refund-value"},
	}, "order", false, 0), "returned_units")
	result := finalizeReport(report{
		SchemaVersion: 1,
		PagesScanned:  3,
		TerminalError: "browser_access_denied",
	}, aggregated, 100)
	if result.Completed || result.StoppedAtPageLimit || result.TerminalError != "browser_access_denied" || result.PagesScanned != 3 || len(result.CandidatePaths) != 2 {
		t.Fatalf("unexpected partial report: %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-refund-value") {
		t.Fatalf("partial report leaked a source value: %s", encoded)
	}
}

func TestClassifyOrderStateUsesExplicitQuantities(t *testing.T) {
	tests := []struct {
		name  string
		order map[string]any
		want  string
	}{
		{name: "ordinary", order: map[string]any{}, want: "ordinary"},
		{name: "fully canceled", order: map[string]any{"allCanceled": true}, want: "fully_canceled"},
		{name: "canceled units", order: map[string]any{"items": []any{map[string]any{"cancelQuantity": json.Number("1")}}}, want: "canceled_units"},
		{name: "returned units", order: map[string]any{"items": []any{map[string]any{"returnReceiptQuantity": json.Number("2")}}}, want: "returned_units"},
		{name: "both", order: map[string]any{"cancelledQuantity": json.Number("1"), "returnedQuantity": json.Number("1")}, want: "canceled_and_returned_units"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyOrderState(test.order); got != test.want {
				t.Fatalf("classifyOrderState() = %q, want %q", got, test.want)
			}
		})
	}
}
