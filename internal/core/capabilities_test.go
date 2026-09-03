package core_test

import (
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReceiptCapabilitiesExposeImplementedStateAndNextEvidence(t *testing.T) {
	report := core.CurrentCapabilities()
	if report.SchemaVersion != 2 {
		t.Fatalf("capability schema version = %d, want 2", report.SchemaVersion)
	}
	byID := make(map[string]core.Capability, len(report.Capabilities))
	for _, capability := range report.Capabilities {
		if _, exists := byID[capability.ID]; exists {
			t.Fatalf("duplicate capability id %q", capability.ID)
		}
		if len(capability.Implemented) == 0 || capability.NextStepKind == "" {
			t.Fatalf("capability %q omits implementation or next-step state: %#v", capability.ID, capability)
		}
		if capability.Status == core.CapabilityExperimental && capability.NextWork == "" {
			t.Fatalf("experimental capability %q omits next work: %#v", capability.ID, capability)
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
	price := byID["price_and_repurchase"]
	if price.Status != core.CapabilityExperimental || price.LastVerified == "" || price.NextWork == "" {
		t.Fatalf("price capability does not expose its experimental evidence state: %#v", price)
	}
	for id, kind := range map[string]core.CapabilityNextStepKind{
		"transparent_affiliate_deeplinks": core.CapabilityNextExternalDependency,
		"explicit_cart_add":               core.CapabilityNextUserAuthorization,
		"product_categories":              core.CapabilityNextLongitudinalValidation,
		"price_and_repurchase":            core.CapabilityNextLongitudinalValidation,
	} {
		capability := byID[id]
		if capability.NextStepKind != kind || len(capability.BlockedBy) == 0 {
			t.Fatalf("capability %q does not expose its blocker class: %#v", id, capability)
		}
	}
	for _, id := range []string{"natural_language_product_discovery", "source_native_product_rankings"} {
		capability := byID[id]
		if capability.Status != core.CapabilityAvailable || capability.NextStepKind != core.CapabilityNextMaintenance || len(capability.BlockedBy) != 0 || capability.LastVerified == "" {
			t.Fatalf("validated product capability %q is not available: %#v", id, capability)
		}
	}
}
