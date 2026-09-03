// order-refund-metadata scans authenticated order documents for refund,
// cancellation, return, exchange, and settlement key shapes. It emits only
// key paths, JSON types, aggregate presence counters, and sign/emptiness
// metadata. A terminal read failure preserves already collected redacted
// evidence with an explicit error code. It never emits source values,
// identifiers, cookies, or order rows.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

const (
	maxAllowedPages = 200
	maxWalkDepth    = 16
)

var candidateFragments = []string{
	"cancel",
	"exchange",
	"refund",
	"reimburse",
	"return",
	"settle",
}

var evidenceFragments = []string{
	"amount",
	"currency",
	"date",
	"fee",
	"paid",
	"price",
	"quantity",
	"status",
	"total",
	"type",
}

type pathEvidence struct {
	Path            string          `json:"path"`
	JSONTypes       []string        `json:"json_types"`
	Occurrences     int             `json:"occurrences"`
	SamplesPresent  int             `json:"samples_present"`
	Contexts        map[string]int  `json:"contexts"`
	NonNull         int             `json:"non_null,omitempty"`
	NumericNegative int             `json:"numeric_negative,omitempty"`
	NumericZero     int             `json:"numeric_zero,omitempty"`
	NumericPositive int             `json:"numeric_positive,omitempty"`
	NonEmptyStrings int             `json:"non_empty_strings,omitempty"`
	TrueBooleans    int             `json:"true_booleans,omitempty"`
	NonEmptyArrays  int             `json:"non_empty_arrays,omitempty"`
	NonEmptyObjects int             `json:"non_empty_objects,omitempty"`
	typeSet         map[string]bool `json:"-"`
}

type localEvidence struct {
	types           map[string]bool
	occurrences     int
	nonNull         int
	numericNegative int
	numericZero     int
	numericPositive int
	nonEmptyStrings int
	trueBooleans    int
	nonEmptyArrays  int
	nonEmptyObjects int
}

type report struct {
	SchemaVersion      int            `json:"schema_version"`
	Scope              string         `json:"scope"`
	Operation          string         `json:"operation"`
	BrowserMode        string         `json:"browser_mode"`
	PagesScanned       int            `json:"pages_scanned"`
	OrdersScanned      int            `json:"orders_scanned"`
	OrderStates        map[string]int `json:"order_states"`
	CandidatePaths     []pathEvidence `json:"candidate_paths"`
	Completed          bool           `json:"completed"`
	StoppedAtPageLimit bool           `json:"stopped_at_page_limit"`
	TerminalError      string         `json:"terminal_error,omitempty"`
	Limitations        []string       `json:"limitations"`
}

func main() {
	maxPages := flag.Int("max-pages", 100, "maximum authenticated order pages to inspect")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall read-only probe timeout")
	pageDelay := flag.Duration("page-delay", 750*time.Millisecond, "pause between order-page reads")
	headed := flag.Bool("headed", false, "force visible installed Chrome instead of headless-first mode")
	flag.Parse()
	if *maxPages < 1 || *maxPages > maxAllowedPages || *timeout <= 0 || *pageDelay < 0 || *pageDelay > 10*time.Second {
		fail("invalid probe bounds")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	paths, err := platform.DefaultPaths()
	if err != nil {
		fail("default paths unavailable")
	}
	var source *browser.Native
	if *headed {
		source = browser.NewNativeHeadedSync(paths.ProfileDir)
	} else {
		source = browser.NewNative(paths.ProfileDir)
	}
	defer source.Close()

	result := report{
		SchemaVersion: 1,
		Scope:         "authenticated_order_model",
		Operation:     "read_only_get",
		BrowserMode:   "headless_first",
		OrderStates:   map[string]int{},
		Limitations: []string{
			"Key names and co-occurrence are discovery evidence, not proof of monetary semantics.",
			"No response value, identifier, timestamp, product text, cursor, cookie, or raw payload is emitted.",
		},
	}
	if *headed {
		result.BrowserMode = "headed"
	}
	aggregated := map[string]*pathEvidence{}
	seenCursors := map[string]bool{}
	var cursor *core.OrderCursor

scanLoop:
	for result.PagesScanned < *maxPages {
		cursorKey := safeCursorKey(cursor)
		if seenCursors[cursorKey] {
			result.TerminalError = "pagination_cycle_detected"
			break
		}
		seenCursors[cursorKey] = true

		document, err := source.Fetch(ctx, cursor)
		if err != nil {
			result.TerminalError = classifyReadError(err)
			break
		}
		page, err := coupangorders.ParseOrderDocument(document)
		if err != nil {
			result.TerminalError = "normalized_order_contract_unavailable"
			break
		}
		root, err := decodeRoot(document)
		if err != nil {
			result.TerminalError = "structured_order_document_unavailable"
			break
		}
		domain, ok := orderDomain(root)
		if !ok {
			result.TerminalError = "order_domain_unavailable"
			break
		}
		orders, ok := domain["orderList"].([]any)
		if !ok {
			result.TerminalError = "order_list_shape_unavailable"
			break
		}

		result.PagesScanned++
		documentMetadata := make(map[string]any, len(domain)-1)
		for key, value := range domain {
			if key != "orderList" {
				documentMetadata[key] = value
			}
		}
		mergeSample(aggregated, collectCandidatePaths(documentMetadata, "domain", false, 0), "document_metadata")

		for _, raw := range orders {
			order, ok := raw.(map[string]any)
			if !ok {
				result.TerminalError = "order_entry_shape_unavailable"
				break scanLoop
			}
			state := classifyOrderState(order)
			result.OrderStates[state]++
			result.OrdersScanned++
			mergeSample(aggregated, collectCandidatePaths(order, "order", false, 0), state)
		}

		cursor = page.Next
		if cursor == nil {
			result.Completed = true
			break
		}
		if *pageDelay > 0 {
			timer := time.NewTimer(*pageDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				result.TerminalError = "timeout"
				break scanLoop
			case <-timer.C:
			}
		}
	}
	result = finalizeReport(result, aggregated, *maxPages)

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail("metadata encoding failed")
	}
	fmt.Println(string(encoded))
}

func finalizeReport(result report, aggregated map[string]*pathEvidence, maxPages int) report {
	result.StoppedAtPageLimit = !result.Completed && result.TerminalError == "" && result.PagesScanned >= maxPages
	result.CandidatePaths = sortedEvidence(aggregated)
	return result
}

func decodeRoot(document []byte) (map[string]any, error) {
	payload := bytes.TrimSpace(document)
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	if payload[0] != '{' {
		var err error
		payload, err = nextDataPayload(payload)
		if err != nil {
			return nil, err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	return root, nil
}

func nextDataPayload(document []byte) ([]byte, error) {
	remaining := document
	for {
		start := bytes.Index(remaining, []byte("<script"))
		if start < 0 {
			return nil, fmt.Errorf("next data missing")
		}
		remaining = remaining[start:]
		tagEnd := bytes.IndexByte(remaining, '>')
		if tagEnd < 0 {
			return nil, fmt.Errorf("script tag malformed")
		}
		tag := remaining[:tagEnd+1]
		body := remaining[tagEnd+1:]
		end := bytes.Index(body, []byte("</script>"))
		if end < 0 {
			return nil, fmt.Errorf("script body malformed")
		}
		if bytes.Contains(tag, []byte(`id="__NEXT_DATA__"`)) || bytes.Contains(tag, []byte(`id='__NEXT_DATA__'`)) {
			return bytes.TrimSpace(body[:end]), nil
		}
		remaining = body[end+len("</script>"):]
	}
}

func orderDomain(root map[string]any) (map[string]any, bool) {
	current := root
	for _, key := range []string{"props", "pageProps", "domains", "desktopOrder"} {
		next, ok := current[key].(map[string]any)
		if !ok {
			if _, modelOK := root["orderList"]; modelOK {
				return root, true
			}
			return nil, false
		}
		current = next
	}
	return current, true
}

func collectCandidatePaths(value any, path string, withinCandidate bool, depth int) map[string]*localEvidence {
	result := map[string]*localEvidence{}
	walkCandidatePaths(value, path, withinCandidate, false, depth, result)
	return result
}

func walkCandidatePaths(value any, path string, withinCandidate bool, recordCurrent bool, depth int, result map[string]*localEvidence) {
	if depth > maxWalkDepth {
		return
	}
	if recordCurrent {
		recordLocal(result, path, value)
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			keyCandidate := candidateKey(key)
			candidateContext := withinCandidate || keyCandidate
			recordChild := keyCandidate || (withinCandidate && evidenceKey(key))
			walkCandidatePaths(typed[key], path+"."+key, candidateContext, recordChild, depth+1, result)
		}
	case []any:
		arrayPath := path + "[]"
		for _, item := range typed {
			walkCandidatePaths(item, arrayPath, withinCandidate, false, depth+1, result)
		}
	}
}

func recordLocal(result map[string]*localEvidence, path string, value any) {
	evidence := result[path]
	if evidence == nil {
		evidence = &localEvidence{types: map[string]bool{}}
		result[path] = evidence
	}
	evidence.occurrences++
	valueType := jsonType(value)
	evidence.types[valueType] = true
	if value != nil {
		evidence.nonNull++
	}
	switch typed := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		if err == nil {
			switch {
			case number < 0:
				evidence.numericNegative++
			case number == 0:
				evidence.numericZero++
			default:
				evidence.numericPositive++
			}
		}
	case float64:
		switch {
		case typed < 0:
			evidence.numericNegative++
		case typed == 0:
			evidence.numericZero++
		default:
			evidence.numericPositive++
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			evidence.nonEmptyStrings++
		}
	case bool:
		if typed {
			evidence.trueBooleans++
		}
	case []any:
		if len(typed) > 0 {
			evidence.nonEmptyArrays++
		}
	case map[string]any:
		if len(typed) > 0 {
			evidence.nonEmptyObjects++
		}
	}
}

func mergeSample(aggregated map[string]*pathEvidence, local map[string]*localEvidence, contextName string) {
	for path, observed := range local {
		evidence := aggregated[path]
		if evidence == nil {
			evidence = &pathEvidence{
				Path:     path,
				Contexts: map[string]int{},
				typeSet:  map[string]bool{},
			}
			aggregated[path] = evidence
		}
		evidence.Occurrences += observed.occurrences
		evidence.SamplesPresent++
		evidence.Contexts[contextName]++
		evidence.NonNull += observed.nonNull
		evidence.NumericNegative += observed.numericNegative
		evidence.NumericZero += observed.numericZero
		evidence.NumericPositive += observed.numericPositive
		evidence.NonEmptyStrings += observed.nonEmptyStrings
		evidence.TrueBooleans += observed.trueBooleans
		evidence.NonEmptyArrays += observed.nonEmptyArrays
		evidence.NonEmptyObjects += observed.nonEmptyObjects
		for valueType := range observed.types {
			evidence.typeSet[valueType] = true
		}
	}
}

func sortedEvidence(aggregated map[string]*pathEvidence) []pathEvidence {
	paths := make([]string, 0, len(aggregated))
	for path := range aggregated {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]pathEvidence, 0, len(paths))
	for _, path := range paths {
		evidence := aggregated[path]
		for valueType := range evidence.typeSet {
			evidence.JSONTypes = append(evidence.JSONTypes, valueType)
		}
		sort.Strings(evidence.JSONTypes)
		result = append(result, *evidence)
	}
	return result
}

func classifyOrderState(order map[string]any) string {
	if value, ok := order["allCanceled"].(bool); ok && value {
		return "fully_canceled"
	}
	hasCanceled := hasPositiveNamedNumber(order, map[string]bool{
		"cancelquantity": true, "cancelledquantity": true,
	})
	hasReturned := hasPositiveNamedNumber(order, map[string]bool{
		"returnreceiptquantity": true, "returnedquantity": true,
	})
	switch {
	case hasCanceled && hasReturned:
		return "canceled_and_returned_units"
	case hasCanceled:
		return "canceled_units"
	case hasReturned:
		return "returned_units"
	default:
		return "ordinary"
	}
}

func hasPositiveNamedNumber(value any, names map[string]bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if names[strings.ToLower(key)] && positiveNumber(child) {
				return true
			}
			if hasPositiveNamedNumber(child, names) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasPositiveNamedNumber(child, names) {
				return true
			}
		}
	}
	return false
}

func positiveNumber(value any) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return err == nil && parsed > 0
	case float64:
		return typed > 0
	default:
		return false
	}
}

func candidateKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range candidateFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func evidenceKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range evidenceFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case json.Number, float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func safeCursorKey(cursor *core.OrderCursor) string {
	if cursor == nil {
		return "initial"
	}
	return fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
}

func classifyReadError(err error) string {
	switch {
	case errors.Is(err, browser.ErrBrowserAccessDenied):
		return "browser_access_denied"
	case errors.Is(err, browser.ErrAuthenticationRequired):
		return "authentication_required"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "authenticated_order_read_failed"
	}
}

func fail(reason string) {
	fmt.Fprintln(os.Stderr, "order refund metadata probe failed:", reason)
	os.Exit(1)
}
