package core

import "time"

// ProductSearchRefinement retains the sequence of source catalogs. Catalogs
// from different selection states are not a union of simultaneously usable choices.
type ProductSearchRefinement struct {
	// AppliedSelections is the current verified set. Prior category choices
	// remain in Steps; they are not simultaneous constraints on the final list.
	AppliedSelections []ProductFacetSelection     `json:"applied_selections"`
	CategoryTrail     []string                    `json:"category_trail,omitempty" jsonschema:"Verified prior category choices replayed before the active category on query searches; these are navigation history, not simultaneous filters"`
	Steps             []ProductFacetObservation   `json:"steps"`
	Issue             *ProductFacetSelectionIssue `json:"issue,omitempty"`
}

type ProductFacetObservation struct {
	CategoryID string                 `json:"category_id,omitempty" jsonschema:"Source category for this particular catalog observation; later catalogs may follow an explicit verified category choice"`
	Selection  *ProductFacetSelection `json:"selection,omitempty"`
	Facets     []ProductFacet         `json:"facets"`
	FetchedAt  time.Time              `json:"fetched_at"`
}

type ProductFacetSelectionIssue struct {
	Selection ProductFacetSelection `json:"selection"`
	Reason    string                `json:"reason" jsonschema:"choice_unavailable, selection_unverified, source_category_changed, source_access_denied, source_authentication_required, source_read_incomplete, time_budget_reached, or document_budget_reached; no silent query fallback"`
}
