package core_test

import (
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCapabilitiesExposeImplementedStateAndNextEvidence(t *testing.T) {
	report := core.CurrentCapabilities()
	if report.SchemaVersion != 3 {
		t.Fatalf("capability schema version = %d, want 3", report.SchemaVersion)
	}
	if report.Summary.Total != 16 || report.Summary.StatusCounts.Available != 6 || report.Summary.StatusCounts.Experimental != 7 || report.Summary.StatusCounts.Planned != 3 {
		t.Fatalf("unexpected capability status summary: %#v", report.Summary)
	}
	if report.Summary.NextStepCounts.Maintenance != 4 || report.Summary.NextStepCounts.LiveValidation != 2 ||
		report.Summary.NextStepCounts.EvidenceRequired != 1 || report.Summary.NextStepCounts.ExternalDependency != 1 ||
		report.Summary.NextStepCounts.UserAuthorization != 0 || report.Summary.NextStepCounts.LongitudinalValidation != 2 {
		t.Fatalf("unexpected capability next-step summary: %#v", report.Summary)
	}
	if report.Summary.ImplementationNextSteps != 6 || report.Summary.ValidationOrCoordinationNextSteps != 6 {
		t.Fatalf("summary does not separate code work from external evidence: %#v", report.Summary)
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

	for _, id := range []string{"batch_receipts", "payment_method_installment_insights", "explicit_cart_add"} {
		capability, ok := byID[id]
		if !ok {
			t.Fatalf("missing capability %q", id)
		}
		if capability.Status != core.CapabilityPlanned || len(capability.Interface) != 0 || capability.NextStepKind != core.CapabilityNextImplementation || capability.NextWork == "" {
			t.Fatalf("capability %q does not expose experimental evidence state: %#v", id, capability)
		}
	}
	account := byID["account_membership_benefits"]
	if account.Status != core.CapabilityExperimental || len(account.Interface) != 2 || account.NextStepKind != core.CapabilityNextImplementation || account.LastVerified != "2026-09-11" || len(account.BlockedBy) != 0 {
		t.Fatal("account read capability lost its bounded implementation or remaining evidence work")
	}
	price := byID["price_and_repurchase"]
	if history := byID["full_order_history"]; history.Status != core.CapabilityExperimental || history.NextStepKind != core.CapabilityNextImplementation {
		t.Fatal("unverified account/scan implementation presented as complete history")
	}
	reporting := byID["recommendation_report"]
	if reporting.Status != core.CapabilityExperimental || reporting.NextStepKind != core.CapabilityNextLiveValidation || len(reporting.Interface) != 2 {
		t.Fatal("report capability must retain experimental/live-validation status")
	}
	if price.Status != core.CapabilityExperimental || price.LastVerified == "" || price.NextWork == "" {
		t.Fatalf("price capability does not expose its experimental evidence state: %#v", price)
	}
	for _, id := range []string{"ordinary_browser_bridge", "current_browser_connection"} {
		if _, exists := byID[id]; exists {
			t.Fatal("retired transport is still advertised")
		}
	}
	for id, kind := range map[string]core.CapabilityNextStepKind{
		"transparent_affiliate_deeplinks": core.CapabilityNextExternalDependency,
		"explicit_cart_add":               core.CapabilityNextImplementation,
		"product_categories":              core.CapabilityNextLongitudinalValidation,
		"price_and_repurchase":            core.CapabilityNextLongitudinalValidation,
	} {
		capability := byID[id]
		if capability.NextStepKind != kind || len(capability.BlockedBy) == 0 {
			t.Fatalf("capability %q does not expose its blocker class: %#v", id, capability)
		}
	}
	for _, id := range []string{"natural_language_product_discovery"} {
		capability := byID[id]
		if capability.Status != core.CapabilityAvailable || capability.NextStepKind != core.CapabilityNextMaintenance || len(capability.BlockedBy) != 0 || capability.LastVerified == "" {
			t.Fatalf("validated product capability %q is not available: %#v", id, capability)
		}
	}
}
