package browser_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	coupangreceipts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/receipts"
)

func TestLiveHeadlessDedicatedProfile(t *testing.T) {
	profileDir := os.Getenv("COUPANGCTL_LIVE_PROFILE_DIR")
	if profileDir == "" {
		t.Skip("COUPANGCTL_LIVE_PROFILE_DIR is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	native := browser.NewNative(profileDir)
	if os.Getenv("COUPANGCTL_LIVE_HEADED") == "1" {
		native = browser.NewNativeHeadedSync(profileDir)
	}
	defer native.Close()
	document, err := native.Fetch(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if page.Orders == nil {
		t.Fatal("headless order document did not contain an order list")
	}
}

func TestLiveHeadlessReceiptReads(t *testing.T) {
	profileDir := os.Getenv("COUPANGCTL_LIVE_PROFILE_DIR")
	if profileDir == "" {
		t.Skip("COUPANGCTL_LIVE_PROFILE_DIR is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	native := browser.NewNative(profileDir)
	if os.Getenv("COUPANGCTL_LIVE_HEADED") == "1" {
		native = browser.NewNativeHeadedSync(profileDir)
	}
	defer native.Close()

	statusDocument, err := native.FetchReceiptStatus(ctx)
	if err != nil {
		t.Fatalf("receipt status read: %v", err)
	}
	if _, err := coupangreceipts.ParseStatusDocument(statusDocument); err != nil {
		t.Fatalf("receipt status parse: %v", err)
	}
	historyRequest := core.ReceiptHistoryRequest{Kind: core.ReceiptKindCard, PageSize: 5}
	historyDocument, err := native.FetchReceiptHistory(ctx, historyRequest)
	if err != nil {
		t.Fatalf("receipt history read: %v", err)
	}
	if _, err := coupangreceipts.ParseHistoryDocument(historyDocument, historyRequest); err != nil {
		t.Fatalf("receipt history parse: %v", err)
	}
	summaryRequest := core.ReceiptSummaryRequest{Kind: core.ReceiptKindCard, From: "2026-01-01", To: "2026-09-03", MaxCards: 20}
	summaryDocument, err := native.FetchReceiptSummary(ctx, summaryRequest)
	if err != nil {
		t.Fatalf("receipt summary read: %v", err)
	}
	if _, err := coupangreceipts.ParseSummaryDocument(summaryDocument, summaryRequest); err != nil {
		t.Fatalf("receipt summary parse: %v", err)
	}
}
