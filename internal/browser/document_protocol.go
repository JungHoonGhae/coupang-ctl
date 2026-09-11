package browser

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// documentPageResult is a bounded data envelope, independent of browser transport.
// It never carries cookies, page HTML, or arbitrary network payloads.
type documentPageResult struct {
	Status     string          `json:"status"`
	Search     *searchDocument `json:"search,omitempty"`
	Inspection json.RawMessage `json:"inspection,omitempty"`
	Category   json.RawMessage `json:"category,omitempty"`
	Page       *core.OrderPage `json:"page,omitempty"`
}

var ErrDocumentProtocol = core.WithErrorCode("document_protocol_error", errors.New("invalid browser document response"))

func validateOrderDocument(page core.OrderPage) error {
	if core.ValidateOrderDocument(page) != nil {
		return ErrDocumentProtocol
	}
	return nil
}

func validDocumentNumericID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 24 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validDocumentText(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
