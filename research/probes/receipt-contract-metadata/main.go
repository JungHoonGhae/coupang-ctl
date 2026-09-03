// receipt-contract-metadata prints browser-sanitized receipt page key paths,
// control kinds, same-origin static endpoint paths, and bounded representative
// vendor-receipt shapes only. It performs no click, POST, request creation, or
// receipt download. Raw order identifiers remain in browser memory and are
// never emitted.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

const (
	maxOrderSamplePages = 100
	maxOrderSamples     = 20
)

type orderDocumentSource interface {
	Fetch(context.Context, *core.OrderCursor) ([]byte, error)
}

type orderSampleOptions struct {
	MaxPages   int
	MaxSamples int
	PageDelay  time.Duration
}

type orderSampleMetadata struct {
	Status              string         `json:"status"`
	PagesScanned        int            `json:"pages_scanned"`
	OrdersScanned       int            `json:"orders_scanned"`
	Completed           bool           `json:"completed"`
	StoppedAtPageLimit  bool           `json:"stopped_at_page_limit"`
	TerminalError       string         `json:"terminal_error,omitempty"`
	SelectedCount       int            `json:"selected_count"`
	SelectedStateCounts map[string]int `json:"selected_state_counts"`
	MaximumPages        int            `json:"maximum_pages"`
	MaximumSamples      int            `json:"maximum_samples"`
}

func main() {
	headed := flag.Bool("headed", false, "force visible installed Chrome instead of headless-first mode")
	skipOrderSamples := flag.Bool("skip-order-samples", false, "skip order lookup and per-order vendor reads")
	maxSamplePages := flag.Int("max-order-pages", 20, "maximum authenticated order pages scanned for representative receipt samples")
	maxSamples := flag.Int("max-order-samples", 12, "maximum representative order receipt reads")
	orderPageDelay := flag.Duration("order-page-delay", 300*time.Millisecond, "pause between order pages while selecting receipt samples")
	timeout := flag.Duration("timeout", 90*time.Second, "overall read-only probe timeout")
	flag.Parse()
	if *timeout <= 0 || *timeout > 5*time.Minute || *maxSamplePages < 1 || *maxSamplePages > maxOrderSamplePages ||
		*maxSamples < 1 || *maxSamples > maxOrderSamples || *orderPageDelay < 0 || *orderPageDelay > 5*time.Second {
		fail("invalid_probe_bounds")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	paths, err := platform.DefaultPaths()
	if err != nil {
		fail("default_paths_unavailable")
	}
	var source *browser.Native
	if *headed {
		source = browser.NewNativeHeadedSync(paths.ProfileDir)
	} else {
		source = browser.NewNative(paths.ProfileDir)
	}
	defer source.Close()
	var orderSamples []browser.ReceiptResearchOrderSample
	orderSampleMetadata := orderSampleMetadata{
		Status: "skipped", SelectedStateCounts: map[string]int{},
		MaximumPages: *maxSamplePages, MaximumSamples: *maxSamples,
	}
	if !*skipOrderSamples {
		orderSamples, orderSampleMetadata = sampleOrders(ctx, source, orderSampleOptions{
			MaxPages: *maxSamplePages, MaxSamples: *maxSamples, PageDelay: *orderPageDelay,
		})
	}
	document, err := source.FetchReceiptResearchMetadata(ctx, orderSamples)
	if err != nil {
		fail(safeErrorCode(err))
	}
	if !json.Valid(document) {
		fail("invalid_sanitized_metadata")
	}
	var result map[string]any
	if json.Unmarshal(document, &result) != nil {
		fail("invalid_sanitized_metadata")
	}
	result["browser_mode"] = map[bool]string{true: "headed", false: "headless_first"}[*headed]
	result["order_sample_status"] = orderSampleMetadata.Status
	result["order_sample_metadata"] = orderSampleMetadata
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail("metadata_encoding_failed")
	}
	fmt.Println(string(encoded))
}

func sampleOrders(ctx context.Context, source orderDocumentSource, options orderSampleOptions) ([]browser.ReceiptResearchOrderSample, orderSampleMetadata) {
	metadata := orderSampleMetadata{
		Status: "no_order_samples", SelectedStateCounts: map[string]int{},
		MaximumPages: options.MaxPages, MaximumSamples: options.MaxSamples,
	}
	seen := map[string]bool{}
	buckets := map[string][]browser.ReceiptResearchOrderSample{}
	seenCursors := map[string]bool{}
	var cursor *core.OrderCursor
	for metadata.PagesScanned < options.MaxPages {
		cursorKey := "initial"
		if cursor != nil {
			cursorKey = fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
		}
		if seenCursors[cursorKey] {
			metadata.TerminalError = "pagination_cycle_detected"
			break
		}
		seenCursors[cursorKey] = true
		document, err := source.Fetch(ctx, cursor)
		if err != nil {
			metadata.TerminalError = safeErrorCode(err)
			break
		}
		page, err := coupangorders.ParseOrderDocument(document)
		if err != nil {
			metadata.TerminalError = "order_shape_unavailable"
			break
		}
		root, err := decodeRoot(document)
		if err != nil {
			metadata.TerminalError = "order_shape_unavailable"
			break
		}
		domain := root
		for _, key := range []string{"props", "pageProps", "domains", "desktopOrder"} {
			next, ok := domain[key].(map[string]any)
			if !ok {
				if _, modelOK := root["orderList"]; modelOK {
					domain = root
					break
				}
				metadata.TerminalError = "order_shape_unavailable"
				break
			}
			domain = next
		}
		if metadata.TerminalError != "" {
			break
		}
		orders, ok := domain["orderList"].([]any)
		if !ok {
			metadata.TerminalError = "order_shape_unavailable"
			break
		}
		metadata.PagesScanned++
		metadata.OrdersScanned += len(orders)
		for _, raw := range orders {
			order, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			orderID := receiptResearchOrderID(order["orderId"])
			if orderID == "" || seen[orderID] {
				continue
			}
			seen[orderID] = true
			state := classifyOrderState(order)
			if len(buckets[state]) < options.MaxSamples {
				buckets[state] = append(buckets[state], browser.ReceiptResearchOrderSample{OrderID: orderID, State: state})
			}
		}
		cursor = page.Next
		if cursor == nil {
			metadata.Completed = true
			break
		}
		if options.PageDelay > 0 {
			timer := time.NewTimer(options.PageDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				metadata.TerminalError = "timeout"
			case <-timer.C:
			}
			if metadata.TerminalError != "" {
				break
			}
		}
	}
	metadata.StoppedAtPageLimit = !metadata.Completed && metadata.TerminalError == "" && metadata.PagesScanned >= options.MaxPages
	result := selectOrderSamples(buckets, options.MaxSamples)
	metadata.SelectedCount = len(result)
	for _, sample := range result {
		metadata.SelectedStateCounts[sample.State]++
	}
	if len(result) > 0 {
		metadata.Status = "sampled_in_memory"
		if metadata.TerminalError != "" {
			metadata.Status = "partial_in_memory"
		}
	} else if metadata.TerminalError != "" {
		metadata.Status = metadata.TerminalError
	}
	return result, metadata
}

func receiptResearchOrderID(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}

func selectOrderSamples(buckets map[string][]browser.ReceiptResearchOrderSample, limit int) []browser.ReceiptResearchOrderSample {
	states := []string{"fully_canceled", "canceled_and_returned_units", "returned_units", "canceled_units", "ordinary"}
	result := make([]browser.ReceiptResearchOrderSample, 0, limit)
	for round := 0; len(result) < limit; round++ {
		added := false
		for _, state := range states {
			if round >= len(buckets[state]) {
				continue
			}
			result = append(result, buckets[state][round])
			added = true
			if len(result) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return result
}

func classifyOrderState(order map[string]any) string {
	if value, ok := order["allCanceled"].(bool); ok && value {
		return "fully_canceled"
	}
	hasCanceled := hasPositiveNamedNumber(order, map[string]bool{
		"cancelquantity": true, "cancelledquantity": true, "canceledquantity": true,
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

func decodeRoot(document []byte) (map[string]any, error) {
	payload := bytes.TrimSpace(document)
	if len(payload) == 0 {
		return nil, errors.New("empty document")
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
			return nil, errors.New("next data missing")
		}
		remaining = remaining[start:]
		tagEnd := bytes.IndexByte(remaining, '>')
		if tagEnd < 0 {
			return nil, errors.New("script tag malformed")
		}
		tag := remaining[:tagEnd+1]
		body := remaining[tagEnd+1:]
		end := bytes.Index(body, []byte("</script>"))
		if end < 0 {
			return nil, errors.New("script body malformed")
		}
		if bytes.Contains(tag, []byte(`id="__NEXT_DATA__"`)) || bytes.Contains(tag, []byte(`id='__NEXT_DATA__'`)) {
			return bytes.TrimSpace(body[:end]), nil
		}
		remaining = body[end+len("</script>"):]
	}
}

func safeErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, browser.ErrBrowserAccessDenied):
		return "browser_access_denied"
	case errors.Is(err, browser.ErrAuthenticationRequired):
		return "authentication_required"
	default:
		return "receipt_metadata_unavailable"
	}
}

func fail(code string) {
	fmt.Fprintln(os.Stderr, "receipt contract metadata probe failed:", code)
	os.Exit(1)
}
