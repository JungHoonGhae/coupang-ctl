package core_test

import (
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReceiptCapabilitiesExposeImplementedStateAndNextEvidence(t *testing.T) {
	report := core.CurrentCapabilities()
	byID := make(map[string]core.Capability, len(report.Capabilities))
	for _, capability := range report.Capabilities {
		if _, exists := byID[capability.ID]; exists {
			t.Fatalf("duplicate capability id %q", capability.ID)
		}
		byID[capability.ID] = capability
	}

	for _, id := range []string{"batch_receipts", "payment_method_installment_insights"} {
		capability, ok := byID[id]
		if !ok {
			t.Fatalf("missing capability %q", id)
		}
		if capability.Status != core.CapabilityExperimental || capability.LastVerified == "" || capability.NextWork == "" {
			t.Fatalf("capability %q does not expose experimental evidence state: %#v", id, capability)
		}
	}
}
