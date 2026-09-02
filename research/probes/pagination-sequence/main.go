// pagination-sequence prints pagination cursors only. It never emits order
// values, identifiers, cookies, or customer data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/JungHoonGhae/oss-coupangctl/internal/browser"
	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	"github.com/JungHoonGhae/oss-coupangctl/internal/platform"
)

type step struct {
	Request *core.OrderCursor `json:"request,omitempty"`
	HasNext bool              `json:"has_next"`
	Next    *core.OrderCursor `json:"next,omitempty"`
}

func main() {
	ctx := context.Background()
	paths, err := platform.DefaultPaths()
	if err != nil {
		fail()
	}
	source := browser.NewNative(paths.ProfileDir)
	defer source.Close()
	var cursor *core.OrderCursor
	seen := map[string]bool{}
	steps := make([]step, 0, 20)
	for len(steps) < 20 {
		key := cursorKey(cursor)
		if seen[key] {
			break
		}
		seen[key] = true
		document, err := source.Fetch(ctx, cursor)
		if err != nil {
			fail()
		}
		var root map[string]any
		if json.Unmarshal(document, &root) != nil {
			fail()
		}
		domain, ok := nestedMap(root, "props", "pageProps", "domains", "desktopOrder")
		if !ok {
			fail()
		}
		pagination, ok := domain["orderPagination"].(map[string]any)
		if !ok {
			fail()
		}
		hasNext, _ := pagination["hasNext"].(bool)
		entry := step{Request: cursor, HasNext: hasNext}
		if hasNext {
			year, yearOK := integer(pagination["nextYear"])
			page, pageOK := integer(pagination["nextPageIndex"])
			if !yearOK || !pageOK {
				fail()
			}
			entry.Next = &core.OrderCursor{Year: year, Page: page}
		}
		steps = append(steps, entry)
		cursor = entry.Next
		if cursor == nil {
			break
		}
	}
	encoded, _ := json.MarshalIndent(map[string]any{"steps": steps}, "", "  ")
	fmt.Println(string(encoded))
}

func nestedMap(root map[string]any, path ...string) (map[string]any, bool) {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func integer(value any) (int, bool) {
	number, ok := value.(float64)
	return int(number), ok && number == float64(int(number))
}

func cursorKey(cursor *core.OrderCursor) string {
	if cursor == nil {
		return "initial"
	}
	return fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
}

func fail() {
	fmt.Fprintln(os.Stderr, "pagination sequence probe failed")
	os.Exit(1)
}
