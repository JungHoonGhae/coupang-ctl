package core

import (
	"context"
	"encoding/json"
	"errors"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Reason     string `json:"reason"`
	Operation  string `json:"operation"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"next_action"`
}

// ErrorJSON exposes the same JSON to a text-only error transport (such as MCP).
// Construct public responses with PublicError, never from an upstream message.
func (r ErrorResponse) ErrorJSON() string {
	data, _ := json.Marshal(r)
	return string(data)
}

type classifiedError struct {
	code  string
	cause error
}

func (e *classifiedError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return errorPolicyFor(e.code).message
}
func (e *classifiedError) Unwrap() error { return e.cause }
func (e *classifiedError) Is(target error) bool {
	t, ok := target.(*classifiedError)
	return ok && e.code == t.code
}

// NewError identifies a known failure without coupling callers to a browser,
// database or output transport. Unknown codes never become public output.
func NewError(code string) error { return &classifiedError{code: code} }

// WithErrorCode preserves errors.Is/As and internal diagnostics. Only the
// allowlisted policy, never cause.Error(), is serialized by PublicError.
func WithErrorCode(code string, cause error) error {
	if cause == nil {
		return nil
	}
	return &classifiedError{code: code, cause: cause}
}

// ValidateRequest marks validation at the request seam, not by matching text
// that an untrusted source could also return.
func ValidateRequest(request interface{ Validate() error }) error {
	return WithErrorCode("invalid_request", request.Validate())
}

type errorPolicy struct {
	code, reason, message, nextAction string
	retryable                         bool
	match                             error
}

// Specific failures precede broad source wrappers, including errors.Join.
// Domain codes remain explicit; reason is the common CLI/MCP taxonomy.
var errorPolicies = []errorPolicy{
	{"cursor_loop", "protocol_error", "order sync encountered a page already committed in this scan; inspect orders sync-status and the source pagination contract before further acquisition; automatic retry is stopped", "orders_sync_status", false, ErrSyncCursorLoop},
	{"sync_in_progress", "busy", "another order sync is running for this ledger; inspect orders sync-status and wait for it to finish before retrying", "orders_sync_status", false, ErrSyncInProgress},
	{"time_budget_exhausted", "deadline_exceeded", "order sync reached its time budget; inspect orders sync-status for committed progress before resuming; complete coverage is not established", "orders_sync_status", false, ErrSyncTimeBudget},
	{"page_deadline_exceeded", "deadline_exceeded", "an order sync page exceeded its deadline; inspect orders sync-status for committed progress before resuming; complete coverage is not established", "orders_sync_status", false, ErrSyncPageDeadline},
	{"partial_order_data", "protocol_error", "Coupang marked an order page as partial; that page was not imported or skipped; inspect orders sync-status and resume later", "orders_sync_status", false, ErrPartialOrderData},
	{"operation_cancelled", "cancelled", "the operation was cancelled; this is not a success result", "none", false, context.Canceled},
	{"operation_timed_out", "deadline_exceeded", "the operation exceeded its time limit; no alternate browser was selected", "review_deadline", false, context.DeadlineExceeded},
	{"camofox_access_denied", "access_denied", "the dedicated Camofox read was denied; retry later in the same profile; this does not prove login expired and no alternate browser was selected", "wait_for_source_access", false, ErrBrowserAccessDenied},
	{"camofox_authentication_required", "authentication_required", "run coupangctl login --manual or coupangctl login --qr --link for the dedicated Camofox profile", "choose_authentication_method", false, ErrAuthenticationRequired},
	{"interactive_confirmation_required", "permission_required", "tell the user that a visible QR login browser may open, then retry with confirmed=true", "request_confirmation", false, ErrInteractiveConfirmationRequired},
	{"invalid_request", "invalid_request", "the request does not satisfy the documented input contract; check command help or the tool input schema", "correct_request", false, nil},
	{"invalid_command", "invalid_request", "see the command help for supported arguments", "correct_request", false, nil},
	{"browser_mode_retired", "unsupported_operation", "legacy browser mode retired; use the dedicated Camofox runtime (coupangctl camofox setup --help)", "configure_runtime", false, nil},
	{"camofox_operation_unsupported", "unsupported_operation", "operation not supported by the Camofox read-only adapter", "check_capabilities", false, nil},
	{"browser_setup_required", "setup_required", "configure the dedicated runtime with coupangctl camofox setup --help, then retry; local tools remain available; no browser was opened", "configure_runtime", false, nil},
	{"permission_required", "permission_required", "the operation requires explicit user permission; no permission or alternate profile was selected automatically", "request_permission", false, nil},
	{"profile_in_use", "busy", "another coupangctl browser operation is using the dedicated profile; wait for it to finish and retry", "wait_for_operation", true, nil},
	{"camofox_document_changed", "target_changed", "the dedicated Camofox operation could not finish; no alternate browser was selected", "repeat_verified_read", false, nil},
	{"camofox_timeout", "deadline_exceeded", "the dedicated Camofox operation could not finish; no alternate browser was selected", "review_deadline", false, nil},
	{"document_protocol_error", "protocol_error", "the browser response did not satisfy the documented response contract; no source result was accepted", "inspect_source_contract", false, nil},
	{"search_facet_unavailable", "target_changed", "the selected sidebar filter could not be verified; rediscover available filters with a plain search", "rediscover_filters", false, nil},
	{"product_identity_mismatch", "target_changed", "the product inspection did not match the requested option; no mismatched result was accepted", "repeat_verified_read", false, nil},
	{"authentication_status_unavailable", "source_unavailable", "the source did not return verifiable authentication status; this does not prove the session expired; retry the quiet check later", "repeat_quiet_auth_check", false, ErrAuthenticationStatusUnavailable},
	{"invalid_order_data", "protocol_error", "the upstream order document has an unsupported shape", "inspect_source_contract", false, ErrInvalidOrderData},
	{"structured_order_data_missing", "protocol_error", "the authenticated order document was not available", "inspect_source_contract", false, nil},
	{"structured_category_data_missing", "protocol_error", "the product document did not expose a verifiable category path; this is not evidence that the product has no category", "inspect_source_contract", false, nil},
	{"structured_product_data_missing", "protocol_error", "the product search or detail document did not expose the expected structured fields", "inspect_source_contract", false, nil},
	{"structured_account_benefits_data_missing", "protocol_error", "the authenticated membership or reward document was not available", "inspect_source_contract", false, nil},
	{"structured_receipt_data_missing", "protocol_error", "the authenticated receipt read endpoint did not expose the expected structure", "inspect_source_contract", false, nil},
	{"local_data_unavailable", "source_unavailable", "check the configured local state directory and ledger access; offline report rendering remains available", "check_local_data", false, nil},
	{"product_price_history_unavailable", "source_unavailable", "the local product price history was unavailable", "check_local_data", false, nil},
	{"product_price_watch_unavailable", "source_unavailable", "the local product price watchlist was unavailable", "check_local_data", false, nil},
	{"product_price_watch_requires_observation", "invalid_request", "observe this exact product option with search or inspect before adding it to the watchlist", "observe_exact_option", false, nil},
	{"vendor_receipt_not_found", "source_unavailable", "the hashed order reference was not found within the requested page bound", "check_source_coverage", false, ErrVendorReceiptNotFound},
	{"qr_expired", "authentication_required", "the QR login expired; run auth login again", "choose_authentication_method", false, nil},
	{"qr_login_timeout", "deadline_exceeded", "the QR login was not approved before timeout", "choose_authentication_method", false, nil},
	{"qr_return_context_missing", "target_changed", "QR approval did not return to the protected order page", "repeat_quiet_auth_check", false, nil},
	{"qr_link_unavailable", "source_unavailable", "the ephemeral QR app link could not be decoded; retry or explicitly choose coupangctl login --manual in the same Camofox profile", "choose_authentication_method", false, nil},
	{"recap_image_render_failed", "source_unavailable", "the installed browser could not render the local recap image", "check_runtime", false, nil},
	{"camofox_request_failed", "source_unavailable", "the dedicated Camofox operation could not finish; no alternate browser was selected", "check_runtime", false, nil},
	{"camofox_unavailable", "source_unavailable", "Camofox runtime unavailable; check the dedicated runtime configuration (no browser fallback)", "check_runtime", false, nil},
	{"product_source_unavailable", "source_unavailable", "the public product source was temporarily unavailable; the request may be retried", "repeat_verified_read", true, nil},
	{"document_source_unavailable", "source_unavailable", "the protected order document could not be loaded", "repeat_verified_read", true, nil},
	{"receipt_source_unavailable", "source_unavailable", "the authenticated receipt source was temporarily unavailable", "repeat_verified_read", true, nil},
	{"provider_unavailable", "internal_error", "the requested tool dependency could not be initialized", "report_error", false, nil},
	{"internal_error", "internal_error", "coupangctl could not complete the command", "report_error", false, nil},
}

func errorPolicyFor(code string) errorPolicy {
	for _, policy := range errorPolicies {
		if policy.code == code {
			return policy
		}
	}
	return errorPolicies[len(errorPolicies)-1]
}

// PublicError is the single CLI/MCP failure policy. It never reads err.Error(),
// request fields, profile paths or source payloads to classify a failure.
func PublicError(operation string, err error) ErrorResponse {
	policy := errorPolicyFor("internal_error")
	for _, candidate := range errorPolicies {
		if (candidate.match != nil && errors.Is(err, candidate.match)) || errors.Is(err, NewError(candidate.code)) {
			policy = candidate
			break
		}
	}
	operation = KnownOperation(operation)
	if operation == "orders_preview" && policy.code == "partial_order_data" {
		policy.message = "Coupang marked the current page as incomplete; no preview was returned and no ledger or checkpoint was read or written"
		policy.nextAction = "inspect_source_contract"
	}
	return ErrorResponse{Error: ErrorBody{Code: policy.code, Message: policy.message, Reason: policy.reason, Operation: operation, Retryable: policy.retryable && repeatableRead(operation), NextAction: policy.nextAction}}
}

// Operation names are fixed contract names, never argv, queries or URLs.
func KnownOperation(operation string) string {
	switch operation {
	case "auth_status", "auth_verify", "auth_login", "auth_login_if_needed", "doctor", "camofox_setup",
		"products_search", "products_recommend", "product_inspect", "products_report_render",
		"product_price_history", "product_watchlist", "product_watch_add", "product_watch_remove", "product_watch_refresh", "cart_add",
		"orders_preview", "orders_sync", "orders_sync_status", "orders_enrich_categories", "orders_category_catalog", "orders_category_stability",
		"orders_list", "orders_spend", "orders_stats", "orders_insights", "orders_product_insights", "orders_reorder_candidates", "orders_export",
		"account_benefits", "receipts_status", "receipts_list", "receipts_summary", "receipts_overview", "receipts_vendor":
		return operation
	default:
		return "unknown"
	}
}

func repeatableRead(operation string) bool {
	switch operation {
	case "products_search", "product_inspect", "products_recommend", "orders_preview", "account_benefits", "receipts_status", "receipts_list", "receipts_summary", "receipts_overview", "receipts_vendor", "auth_status", "auth_verify":
		return true
	default:
		return false
	}
}
