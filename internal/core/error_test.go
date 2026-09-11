package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPublicErrorContractClassifiesWithoutLeakingCauses(t *testing.T) {
	for _, tc := range []struct{ code, reason string }{
		{"invalid_request", "invalid_request"},
		{"camofox_operation_unsupported", "unsupported_operation"},
		{"browser_setup_required", "setup_required"},
		{"permission_required", "permission_required"},
		{"camofox_authentication_required", "authentication_required"},
		{"camofox_access_denied", "access_denied"},
		{"profile_in_use", "busy"},
		{"operation_cancelled", "cancelled"},
		{"operation_timed_out", "deadline_exceeded"},
		{"camofox_document_changed", "target_changed"},
		{"document_protocol_error", "protocol_error"},
		{"product_source_unavailable", "source_unavailable"},
		{"internal_error", "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			err := WithErrorCode(tc.code, errors.New("synthetic-private-cause"))
			response := PublicError("products_search", fmt.Errorf("synthetic-private-wrapper: %w", err))
			if response.Error.Code != tc.code || response.Error.Reason != tc.reason || response.Error.Operation != "products_search" || response.Error.Message == "" || response.Error.NextAction == "" {
				t.Fatalf("wrong classified error: %+v", response.Error)
			}
			var fields map[string]map[string]any
			if json.Unmarshal([]byte(response.ErrorJSON()), &fields) != nil || len(fields["error"]) != 6 || strings.Contains(response.ErrorJSON(), "synthetic-private") {
				t.Fatal("missing explicit wire field or private error leaked")
			}
			if PublicError("cart_add", err).Error.Retryable || PublicError("auth_login_if_needed", err).Error.Retryable || PublicError("orders_sync", err).Error.Retryable {
				t.Fatal("read retry hint authorized retrying a mutation")
			}
		})
	}
}

func TestPublicErrorPreservesSpecificJoinedFailures(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{ErrAuthenticationRequired, "camofox_authentication_required"},
		{ErrBrowserAccessDenied, "camofox_access_denied"},
		{context.Canceled, "operation_cancelled"},
		{context.DeadlineExceeded, "operation_timed_out"},
		{NewError("document_protocol_error"), "document_protocol_error"},
		{NewError("search_facet_unavailable"), "search_facet_unavailable"},
	} {
		err := errors.Join(NewError("product_source_unavailable"), fmt.Errorf("synthetic-private: %w", tc.cause))
		if !errors.Is(err, tc.cause) || PublicError("products_search", err).Error.Code != tc.code || PublicError("products_search", err).Error.Retryable {
			t.Fatalf("specific failure was lost: %s", tc.code)
		}
	}
	if PublicError("orders_sync", errors.Join(ErrSyncPageDeadline, context.DeadlineExceeded)).Error.Code != "page_deadline_exceeded" {
		t.Fatal("page progress recovery was replaced by generic timeout")
	}
	preview := PublicError("orders_preview", ErrPartialOrderData).Error
	if strings.Contains(preview.Message, "sync-status") || strings.Contains(preview.NextAction, "sync") || !strings.Contains(preview.Message, "no ledger or checkpoint") {
		t.Fatal("preview error implies use of persisted order history")
	}
}

func TestPublicErrorNeverClassifiesUntrustedText(t *testing.T) {
	for _, err := range []error{nil, errors.New("usage: synthetic-private"), errors.New("access_denied: synthetic-private"), NewError("synthetic-private-code"), WithErrorCode("synthetic-private-code", errors.New("synthetic-private-cause"))} {
		response := PublicError("synthetic-private-operation", err)
		if response.Error.Code != "internal_error" || response.Error.Operation != "unknown" || strings.Contains(response.ErrorJSON(), "synthetic-private") {
			t.Fatal("untrusted text became public error metadata")
		}
	}
}
