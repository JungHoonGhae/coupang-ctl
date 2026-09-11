package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestAuthenticationStatusErrorIsTypedAndRedacted(t *testing.T) {
	var output bytes.Buffer
	err := &camofoxReadError{cause: errors.Join(core.ErrAuthenticationStatusUnavailable, errors.New("synthetic-private-source-value"))}
	WriteCommandError(&output, []string{"auth", "verify"}, err)
	var wire core.ErrorResponse
	if json.Unmarshal(output.Bytes(), &wire) != nil || wire.Error.Code != "authentication_status_unavailable" || bytes.Contains(output.Bytes(), []byte("synthetic-private")) {
		t.Fatal("authentication error lost classification or leaked source details")
	}
}
