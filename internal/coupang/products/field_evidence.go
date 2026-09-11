package products

import (
	"slices"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func normalizedCurrency(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 3 {
		return ""
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return ""
		}
	}
	return value
}

func validFieldEvidence(values []core.ProductFieldEvidence, reference core.ProductReference, available []string) bool {
	if len(values) > 64 {
		return false
	}
	seen := make(map[string]bool)
	for _, value := range values {
		if seen[value.Field] || !slices.Contains(available, value.Field) || !value.ValidFor(reference) {
			return false
		}
		seen[value.Field] = true
	}
	return true
}
