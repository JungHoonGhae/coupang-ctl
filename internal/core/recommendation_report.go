package core

import (
	"errors"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"
)

const ProductRecommendationReportSchemaVersion = 2

// Operational resource limits, not a sufficient/optimal recommendation count.
const ProductReportMaxInputBytes = 4 << 20
const ProductReportMaxOutputBytes = 8 << 20

type ProductRecommendationReportRenderRequest struct {
	Report ProductRecommendationReport `json:"report" jsonschema:"Existing recommendation v5 evidence including inspected exclusions, required conditions, next actions and optional private-local purchase aggregates. Sync v2/v3 records remain separate from verified account-history coverage; does not initiate shopping research"`
}

type ProductRecommendationReportRenderResult struct {
	SchemaVersion        int                         `json:"schema_version"`
	Visibility           string                      `json:"visibility"`
	RecommendationStatus ProductRecommendationStatus `json:"recommendation_status"`
	MediaType            string                      `json:"media_type"`
	HTML                 string                      `json:"html"`
	BytesRendered        int                         `json:"bytes_rendered"`
}

type ProductImageKind string

const (
	ProductImageKindGallery  ProductImageKind = "gallery"
	ProductImageKindDetail   ProductImageKind = "detail"
	ProductImageKindExternal ProductImageKind = "external"
)

type ProductImageRetrieval string

const (
	ProductImageRetrievalCDPCache ProductImageRetrieval = "cdp_resource_cache"
	// ProductImageRetrievalBrowserCache means Chrome was allowed to reuse its
	// HTTP cache, but CDP did not prove whether this individual load was a hit.
	// Keep that distinction explicit instead of presenting an eligible request
	// as an observed cache hit.
	ProductImageRetrievalBrowserCache ProductImageRetrieval = "browser_cache_preferred"
	ProductImageRetrievalNetwork      ProductImageRetrieval = "network_fallback"
)

type ProductImageVisualEvidence struct {
	ProductID string                `json:"product_id"`
	ImageURL  string                `json:"image_url"`
	Kind      ProductImageKind      `json:"kind"`
	Source    string                `json:"source"`
	Retrieval ProductImageRetrieval `json:"retrieval"`
	Findings  []string              `json:"findings"`
}

type ProductImageReadRequest struct {
	ProductID string `json:"product_id" jsonschema:"Public numeric product identifier returned by product_inspect"`
	ImageURL  string `json:"image_url" jsonschema:"Coupang CDN image URL returned by product_inspect"`
}

type ProductImageReadResult struct {
	SchemaVersion int                   `json:"schema_version"`
	ProductID     string                `json:"product_id"`
	ImageURL      string                `json:"image_url"`
	MIMEType      string                `json:"mime_type"`
	ByteCount     int                   `json:"byte_count"`
	Retrieval     ProductImageRetrieval `json:"retrieval"`
	Data          []byte                `json:"-"`
}

func (r ProductImageReadRequest) Validate() error {
	if !NumericProductIdentifier(r.ProductID) {
		return errors.New("product image product_id must be numeric")
	}
	return ValidateProductImageURL(r.ImageURL)
}

// ValidateProductImageURL restricts acquisition, not product ownership. A valid
// CDN URL still needs an association with the inspected product's evidence.
func ValidateProductImageURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") ||
		!(parsed.Hostname() == "coupangcdn.com" || strings.HasSuffix(parsed.Hostname(), ".coupangcdn.com")) {
		return errors.New("product image URL must use the Coupang CDN")
	}
	return nil
}

type ProductComparisonAxis struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Weight       float64 `json:"weight"`
	EvidenceRule string  `json:"evidence_rule"`
}

type ProductComparisonValue struct {
	AxisID      string   `json:"axis_id"`
	Score       *float64 `json:"score"`
	Provenance  string   `json:"provenance"`
	Explanation string   `json:"explanation"`
}

type ProductCandidateScore struct {
	ProductID        string                   `json:"product_id"`
	Values           []ProductComparisonValue `json:"values"`
	WeightedFit      *float64                 `json:"weighted_fit"`
	EvidenceCoverage float64                  `json:"evidence_coverage" jsonschema:"Sum of declared axis weights having a non-null score; availability only, not confidence or scientific validation"`
}

type ProductEvidenceSource struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	FaviconURL string   `json:"favicon_url,omitempty"`
	Kind       string   `json:"kind"`
	Facts      []string `json:"facts"`
}

type ProductRecommendationReport struct {
	SchemaVersion  int                          `json:"schema_version,omitempty"`
	Title          string                       `json:"title"`
	Subtitle       string                       `json:"subtitle,omitempty"`
	Recommendation ProductRecommendationResult  `json:"recommendation"`
	Axes           []ProductComparisonAxis      `json:"axes,omitempty"`
	Scores         []ProductCandidateScore      `json:"scores,omitempty"`
	VisualEvidence []ProductImageVisualEvidence `json:"visual_evidence"`
	Sources        []ProductEvidenceSource      `json:"sources,omitempty"`
	Summary        string                       `json:"summary,omitempty" jsonschema:"Editorial interpretation of the supplied evidence, always displayed as inference; not a source-native fact or suitability guarantee"`
	CandidateNotes []ProductReportCandidateNote `json:"candidate_notes,omitempty"`
	DecisionPaths  []ProductReportDecisionPath  `json:"decision_paths,omitempty" jsonschema:"At most three conditional editorial paths tied to exact displayed candidates; displayed as inference, never as user preferences or execution history"`
}

// Decision paths explain the editorial choice, not an observed preference or
// an automatic ranking. The renderer independently reassesses hard conditions.
type ProductReportDecisionPath struct {
	Label      string             `json:"label"`
	When       string             `json:"when"`
	Reason     string             `json:"reason"`
	References []ProductReference `json:"references"`
}

// Editorial notes are separate from observed product names, source evidence,
// and reassessed conditions. Exact references prevent notes crossing options.
type ProductReportCandidateNote struct {
	Reference ProductReference `json:"reference"`
	Title     string           `json:"title" jsonschema:"Short editorial label; original product name remains in evidence"`
	Rationale string           `json:"rationale" jsonschema:"Editorial interpretation, displayed as inference"`
	Tradeoffs []string         `json:"tradeoffs,omitempty"`
}

type ProductRecommendationReportWriteRequest struct {
	Report     ProductRecommendationReport `json:"report"`
	OutputPath string                      `json:"output_path"`
}

type ProductRecommendationReportWriteResult struct {
	SchemaVersion       int    `json:"schema_version"`
	OutputPath          string `json:"output_path"`
	BytesWritten        int    `json:"bytes_written"`
	DiscoveredCount     int    `json:"discovered_count"`
	CandidateCount      int    `json:"candidate_count"`
	VisualEvidenceCount int    `json:"visual_evidence_count"`
}

func (r ProductRecommendationReport) Validate() error {
	if len(r.DecisionPaths) > 3 {
		return errors.New("at most three editorial decision paths are supported")
	}
	for _, path := range r.DecisionPaths {
		if strings.TrimSpace(path.Label) == "" || utf8.RuneCountInString(path.Label) > 20 || strings.TrimSpace(path.When) == "" || utf8.RuneCountInString(path.When) > 240 || strings.TrimSpace(path.Reason) == "" || utf8.RuneCountInString(path.Reason) > 1200 || len(path.References) == 0 || len(path.References) > 200 {
			return errors.New("decision paths require bounded labels, assumptions, reasons and references")
		}
		seen := map[ProductReference]bool{}
		for _, ref := range path.References {
			matches := 0
			for _, candidate := range r.Recommendation.Candidates {
				if candidate.Product.Reference == ref {
					matches++
				}
			}
			if matches != 1 || seen[ref] {
				return errors.New("decision paths must reference unique exact displayed candidates")
			}
			seen[ref] = true
		}
	}
	if utf8.RuneCountInString(r.Summary) > 1600 || len(r.CandidateNotes) > len(r.Recommendation.Candidates) {
		return errors.New("report editorial notes exceed limits")
	}
	noted := make(map[ProductReference]bool)
	for _, note := range r.CandidateNotes {
		matches := 0
		for _, candidate := range r.Recommendation.Candidates {
			if candidate.Product.Reference == note.Reference {
				matches++
			}
		}
		if matches != 1 || noted[note.Reference] || strings.TrimSpace(note.Title) == "" || utf8.RuneCountInString(note.Title) > 80 || utf8.RuneCountInString(note.Rationale) > 1200 || len(note.Tradeoffs) > 10 {
			return errors.New("report editorial note requires a unique exact candidate and bounded text")
		}
		for _, text := range note.Tradeoffs {
			if utf8.RuneCountInString(text) > 400 {
				return errors.New("report tradeoff exceeds text limit")
			}
		}
		noted[note.Reference] = true
	}
	if len(r.Axes) > 32 || len(r.Recommendation.Candidates) > 200 || len(r.Scores) > 200 || len(r.Recommendation.Discovered) > 1000 {
		return errors.New("report exceeds rendering resource limits")
	}
	if err := validateReportDecisions(r.Recommendation); err != nil {
		return err
	}
	if err := validateReportPurchase(r.Recommendation); err != nil {
		return err
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("report title is required")
	}
	if r.SchemaVersion != 0 && r.SchemaVersion != ProductRecommendationReportSchemaVersion {
		return errors.New("unsupported recommendation report schema version")
	}
	if r.Recommendation.SchemaVersion != ProductRecommendationSchemaVersion {
		return errors.New("report requires the current typed recommendation schema")
	}
	if r.Recommendation.CategoryID != "" {
		if err := (ProductSearchRequest{Query: r.Recommendation.Query, CategoryID: r.Recommendation.CategoryID}).Validate(); err != nil {
			return err
		}
	}
	if r.Recommendation.AppliedCategoryID != "" {
		if r.Recommendation.Refinement == nil {
			return errors.New("applied category requires verified sidebar refinement")
		}
		scope := ProductCoverage{AppliedCategoryID: r.Recommendation.AppliedCategoryID, Facets: r.Recommendation.Facets}
		request := ProductSearchRequest{Query: r.Recommendation.Query, CategoryID: r.Recommendation.CategoryID, FacetSelections: r.Recommendation.Refinement.AppliedSelections}
		if err := scope.ValidateCategoryScope(request); err != nil {
			return err
		}
	}
	switch r.Recommendation.Status {
	case ProductRecommendationComplete, ProductRecommendationIncomplete, ProductRecommendationNeedsInput:
	case ProductRecommendationNoMatches:
		if len(r.Recommendation.Candidates) > 0 {
			return errors.New("no_matches report cannot contain recommendation candidates")
		}
	default:
		return errors.New("unsupported recommendation status")
	}
	candidates := make(map[string]int, len(r.Recommendation.Candidates))
	for _, candidate := range r.Recommendation.Candidates {
		id := candidate.Product.Reference.ProductID
		if !NumericProductIdentifier(id) {
			return errors.New("report candidate product_id must be numeric")
		}
		candidates[id]++
	}
	for _, evidence := range r.VisualEvidence {
		if candidates[evidence.ProductID] != 1 {
			return errors.New("visual evidence must reference an unambiguous candidate")
		}
		if !validHTTPURL(evidence.ImageURL) || strings.TrimSpace(evidence.Source) == "" || len(evidence.Findings) == 0 {
			return errors.New("visual evidence requires an image URL, source, and findings")
		}
		switch evidence.Kind {
		case ProductImageKindGallery, ProductImageKindDetail, ProductImageKindExternal:
		default:
			return errors.New("unsupported visual evidence kind")
		}
		switch evidence.Retrieval {
		case ProductImageRetrievalCDPCache, ProductImageRetrievalBrowserCache, ProductImageRetrievalNetwork:
		default:
			return errors.New("unsupported visual evidence retrieval")
		}
	}
	axis := make(map[string]ProductComparisonAxis, len(r.Axes))
	weightTotal := 0.0
	for _, item := range r.Axes {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.EvidenceRule) == "" || !finiteComparisonNumber(item.Weight) || item.Weight <= 0 || item.Weight > 1 {
			return errors.New("comparison axes require an id, label, evidence rule, and positive weight")
		}
		if _, exists := axis[item.ID]; exists {
			return errors.New("comparison axis ids must be unique")
		}
		axis[item.ID] = item
		weightTotal += item.Weight
	}
	if len(r.Axes) > 0 && math.Abs(weightTotal-1) > 0.000001 {
		return errors.New("comparison axis weights must sum to 1")
	}
	scored := make(map[string]bool)
	for _, score := range r.Scores {
		if candidates[score.ProductID] != 1 || scored[score.ProductID] || !finiteComparisonNumber(score.EvidenceCoverage) || score.EvidenceCoverage < 0 || score.EvidenceCoverage > 1 {
			return errors.New("candidate score has an invalid candidate or coverage")
		}
		scored[score.ProductID] = true
		seen := make(map[string]bool)
		coverage, weighted := 0.0, 0.0
		known := 0
		for _, value := range score.Values {
			item, ok := axis[value.AxisID]
			if !ok || seen[value.AxisID] || value.Score != nil && (!finiteComparisonNumber(*value.Score) || *value.Score < 0 || *value.Score > 100) {
				return errors.New("candidate score has an invalid axis or value")
			}
			seen[value.AxisID] = true
			if value.Score == nil {
				continue
			}
			if strings.TrimSpace(value.Explanation) == "" {
				return errors.New("known comparison score requires an explanation")
			}
			switch value.Provenance {
			case "observed", "derived", "inferred":
			default:
				return errors.New("known comparison score requires an explicit provenance class")
			}
			known++
			coverage += item.Weight
			weighted += item.Weight * *value.Score
		}
		if math.Abs(coverage-score.EvidenceCoverage) > 0.000001 {
			return errors.New("comparison coverage must equal the available axis weight")
		}
		if score.WeightedFit != nil && (len(axis) == 0 || known != len(axis) || !finiteComparisonNumber(*score.WeightedFit) || *score.WeightedFit < 0 || *score.WeightedFit > 100 || math.Abs(*score.WeightedFit-weighted) > 0.000001) {
			return errors.New("weighted fit requires every axis and must match the declared calculation")
		}
	}
	for _, source := range r.Sources {
		if strings.TrimSpace(source.Name) == "" || !validHTTPURL(source.URL) || source.FaviconURL != "" && !validHTTPURL(source.FaviconURL) {
			return errors.New("report sources require valid names and URLs")
		}
	}
	return nil
}

func finiteComparisonNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil
}
