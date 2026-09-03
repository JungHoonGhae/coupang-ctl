package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOrdinaryBrowserNativeHostPingUsesSyntheticEmptyPageAndCleansRendezvous(t *testing.T) {
	stateDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := PingOrdinaryBrowserNativeHost(ctx, stateDir); err != nil {
		t.Fatalf("PingOrdinaryBrowserNativeHost() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, ordinaryRendezvousFilename)); !os.IsNotExist(err) {
		t.Fatalf("ping left rendezvous metadata behind: %v", err)
	}
}

func TestOrdinaryBrowserNativeHostPingCleansRendezvousOnCancellation(t *testing.T) {
	stateDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := PingOrdinaryBrowserNativeHost(ctx, stateDir); err == nil {
		t.Fatal("expected canceled native-host ping to fail")
	}
	if _, err := os.Stat(filepath.Join(stateDir, ordinaryRendezvousFilename)); !os.IsNotExist(err) {
		t.Fatalf("canceled ping left rendezvous metadata behind: %v", err)
	}
}
