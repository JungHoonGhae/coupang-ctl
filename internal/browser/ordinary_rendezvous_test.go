package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestOrdinaryBridgeRendezvousRelaysOneAuthenticatedBrowserConnection(t *testing.T) {
	stateDir := t.TempDir()
	// The captured value keeps validation deterministic, while remaining a real
	// future socket deadline for the duration of this test.
	now := time.Now().UTC()
	bridge, err := startOrdinaryBridgeRendezvous(
		stateDir,
		func() time.Time { return now },
		bytes.NewReader(bytes.Repeat([]byte{0xab}, ordinaryRendezvousTokenBytes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })

	info, err := os.Stat(ordinaryRendezvousPath(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("rendezvous mode = %o, want 600", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	requestID := strings.Repeat("b", 32)
	request := OrdinaryBridgeRequest{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Operation:     OrdinaryBridgeReadOrders,
	}
	type result struct {
		response OrdinaryBridgeResponse
		err      error
	}
	resultChannel := make(chan result, 1)
	go func() {
		response, roundTripErr := bridge.RoundTrip(ctx, request)
		resultChannel <- result{response: response, err: roundTripErr}
	}()

	jobs, err := connectOrdinaryBridgeRendezvous(ctx, stateDir, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	relayed, err := jobs.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if relayed != request {
		t.Fatalf("relayed request = %#v, want %#v", relayed, request)
	}
	want := OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Status:        OrdinaryBridgeOK,
		Page: &core.OrderPage{Orders: []core.Order{{
			SourceRef:   strings.Repeat("a", sha256.Size*2),
			PurchasedAt: "2026-09-03",
			Currency:    "KRW",
		}}},
	}
	if err := jobs.Complete(ctx, want); err != nil {
		t.Fatal(err)
	}
	got := <-resultChannel
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.response.RequestID != want.RequestID || got.response.Status != want.Status || got.response.Page == nil || len(got.response.Page.Orders) != 1 {
		t.Fatalf("round-trip response = %#v, want %#v", got.response, want)
	}
	if _, err := os.Stat(ordinaryRendezvousPath(stateDir)); !os.IsNotExist(err) {
		t.Fatalf("rendezvous remained after authentication: %v", err)
	}
}

func TestOrdinaryBridgeRendezvousReplacesOnlyExpiredOwner(t *testing.T) {
	stateDir := t.TempDir()
	current := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return current }
	first, err := startOrdinaryBridgeRendezvous(
		stateDir,
		now,
		bytes.NewReader(bytes.Repeat([]byte{0xaa}, ordinaryRendezvousTokenBytes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	if _, err := startOrdinaryBridgeRendezvous(
		stateDir,
		now,
		bytes.NewReader(bytes.Repeat([]byte{0xbb}, ordinaryRendezvousTokenBytes)),
	); !errors.Is(err, ErrOrdinaryRendezvous) {
		t.Fatalf("active rendezvous replacement error = %v, want ErrOrdinaryRendezvous", err)
	}

	current = current.Add(ordinaryRendezvousLifetime + time.Second)
	second, err := startOrdinaryBridgeRendezvous(
		stateDir,
		now,
		bytes.NewReader(bytes.Repeat([]byte{0xcc}, ordinaryRendezvousTokenBytes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ordinaryRendezvousPath(stateDir)); err != nil {
		t.Fatalf("old owner removed replacement rendezvous: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ordinaryRendezvousPath(stateDir)); !os.IsNotExist(err) {
		t.Fatalf("replacement rendezvous remained after close: %v", err)
	}
}

func TestOrdinaryBrowserBridgePublicInterfaceDrivesNativeHost(t *testing.T) {
	stateDir := t.TempDir()
	bridge, err := StartOrdinaryBrowserBridge(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	extensionID := strings.Repeat("a", 32)
	requestID := strings.Repeat("d", 32)
	response := OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Status:        OrdinaryBridgeUnavailable,
	}
	responsePayload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var extensionInput bytes.Buffer
	if err := writeOrdinaryNativeFrame(&extensionInput, responsePayload); err != nil {
		t.Fatal(err)
	}
	var extensionOutput bytes.Buffer
	hostResult := make(chan error, 1)
	go func() {
		hostResult <- RunOrdinaryBrowserNativeHost(
			ctx,
			stateDir,
			"chrome-extension://"+extensionID+"/",
			extensionID,
			&extensionInput,
			&extensionOutput,
		)
	}()

	got, err := bridge.RoundTrip(ctx, OrdinaryBridgeRequest{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Operation:     OrdinaryBridgeReadOrders,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != OrdinaryBridgeUnavailable {
		t.Fatalf("bridge response status = %q, want %q", got.Status, OrdinaryBridgeUnavailable)
	}
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-hostResult; err != nil {
		t.Fatalf("native host shutdown error = %v, want nil", err)
	}
	relayedPayload, err := readOrdinaryNativeFrame(&extensionOutput)
	if err != nil {
		t.Fatal(err)
	}
	relayed, err := decodeOrdinaryBridgeRequestFrame(relayedPayload)
	if err != nil || relayed.RequestID != requestID || extensionOutput.Len() != 0 {
		t.Fatalf("extension request = %#v remaining=%d error=%v", relayed, extensionOutput.Len(), err)
	}
}

func TestOrdinaryBrowserBridgeFetchPageUsesTypedProtocol(t *testing.T) {
	stateDir := t.TempDir()
	bridge, err := StartOrdinaryBrowserBridge(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type pageResult struct {
		page core.OrderPage
		err  error
	}
	resultChannel := make(chan pageResult, 1)
	go func() {
		page, fetchErr := bridge.FetchPage(ctx, &core.OrderCursor{Year: 2024, Page: 7})
		resultChannel <- pageResult{page: page, err: fetchErr}
	}()

	jobs, err := connectOrdinaryBridgeRendezvous(ctx, stateDir, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	request, err := jobs.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if request.Operation != OrdinaryBridgeReadOrders || request.Cursor == nil || request.Cursor.Year != 2024 || request.Cursor.Page != 7 || !validOrdinaryBridgeRequestID(request.RequestID) {
		t.Fatalf("typed bridge request = %#v", request)
	}
	want := core.OrderPage{Orders: []core.Order{}}
	if err := jobs.Complete(ctx, OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     request.RequestID,
		Status:        OrdinaryBridgeOK,
		Page:          &want,
	}); err != nil {
		t.Fatal(err)
	}
	got := <-resultChannel
	if got.err != nil || len(got.page.Orders) != 0 {
		t.Fatalf("fetched page = %#v error=%v", got.page, got.err)
	}
}
