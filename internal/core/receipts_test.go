package core

import "testing"

func TestOrderSourceReferenceIsStableAndDoesNotExposeSourceID(t *testing.T) {
	first := OrderSourceReference("synthetic-order-id")
	second := OrderSourceReference("synthetic-order-id")
	if first != second || len(first) != 64 || first == "synthetic-order-id" {
		t.Fatalf("unexpected source reference %q", first)
	}
}

func TestVendorReceiptRequestRequiresHashedReferenceAndBoundedSearch(t *testing.T) {
	valid := VendorReceiptRequest{SourceRef: OrderSourceReference("synthetic-order"), MaxPages: 1000}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	for _, request := range []VendorReceiptRequest{
		{SourceRef: "raw-order-id", MaxPages: 1},
		{SourceRef: valid.SourceRef, MaxPages: 1001},
	} {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid request accepted: %#v", request)
		}
	}
}
