// receipt-contract-metadata prints browser-sanitized receipt page key paths,
// control kinds, and same-origin static endpoint paths only. It performs no
// click, POST, request creation, or receipt download.
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
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

func main() {
	headed := flag.Bool("headed", false, "force visible installed Chrome instead of headless-first mode")
	skipOrderSamples := flag.Bool("skip-order-samples", false, "skip order lookup and per-order vendor reads")
	timeout := flag.Duration("timeout", 90*time.Second, "overall read-only probe timeout")
	flag.Parse()
	if *timeout <= 0 || *timeout > 5*time.Minute {
		fail("invalid timeout")
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
	orderSampleStatus := "skipped"
	if !*skipOrderSamples {
		orderSamples, orderSampleStatus = sampleOrders(ctx, source)
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
	result["order_sample_status"] = orderSampleStatus
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail("metadata_encoding_failed")
	}
	fmt.Println(string(encoded))
}

func sampleOrders(ctx context.Context, source *browser.Native) ([]browser.ReceiptResearchOrderSample, string) {
	document, err := source.Fetch(ctx, nil)
	if err != nil {
		return nil, safeErrorCode(err)
	}
	root, err := decodeRoot(document)
	if err != nil {
		return nil, "order_shape_unavailable"
	}
	domain := root
	for _, key := range []string{"props", "pageProps", "domains", "desktopOrder"} {
		next, ok := domain[key].(map[string]any)
		if !ok {
			if _, modelOK := root["orderList"]; modelOK {
				domain = root
				break
			}
			return nil, "order_shape_unavailable"
		}
		domain = next
	}
	orders, ok := domain["orderList"].([]any)
	if !ok {
		return nil, "order_shape_unavailable"
	}
	seen := map[string]bool{}
	result := make([]browser.ReceiptResearchOrderSample, 0, 5)
	for _, raw := range orders {
		order, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		var orderID string
		switch value := order["orderId"].(type) {
		case string:
			orderID = value
		case json.Number:
			orderID = value.String()
		case float64:
			if value == float64(int64(value)) {
				orderID = fmt.Sprintf("%.0f", value)
			}
		}
		if orderID == "" || seen[orderID] {
			continue
		}
		seen[orderID] = true
		result = append(result, browser.ReceiptResearchOrderSample{OrderID: orderID, State: classifyOrderState(order)})
		if len(result) == 5 {
			break
		}
	}
	if len(result) == 0 {
		return nil, "no_order_samples"
	}
	return result, "sampled_in_memory"
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
