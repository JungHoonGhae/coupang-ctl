package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
)

func TestCamofoxCategoryCLIRequiresRuntimeButEmptyLedgerDoesNotStartIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", dir)
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"orders", "categories", "--max-products", "1"}, &out, &stderr, "test")
	if !errors.Is(err, browser.ErrCamofoxUnavailable) {
		t.Fatal("category source read skipped configuration")
	}
	cfg := `{"schema_version":1,"node":"/synthetic-node","server":"/synthetic-server","engine_dir":"/synthetic-engine","user_id":"synthetic"}`
	if err := os.WriteFile(filepath.Join(dir, "camofox.json"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"orders", "categories", "--max-products", "1"}, &out, &stderr, "test"); err != nil {
		t.Fatal("page source did not connect categories", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"products_processed": 0`)) {
		t.Fatal("unexpected empty category result")
	}
}

func TestCategoryDataFailureIsNotReportedAsAnOrderDocumentFailure(t *testing.T) {
	var out bytes.Buffer
	WriteCommandError(&out, []string{"orders", "categories"}, &camofoxReadError{cause: errors.Join(orders.ErrDocumentSource, browser.ErrStructuredCategoryDataMissing)})
	if !bytes.Contains(out.Bytes(), []byte(`"code": "structured_category_data_missing"`)) {
		t.Fatal("category failure lost its specific recovery context")
	}
}
