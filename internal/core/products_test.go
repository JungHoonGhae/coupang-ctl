package core

import "testing"

func TestProductSearchAcceptsNativeCategoryRankingWithoutFreeText(t *testing.T) {
	request := ProductSearchRequest{CategoryID: "12345", Sort: ProductSortSales, MinMemoryGB: 16, MinStorageGB: 512}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid category ranking request rejected: %v", err)
	}
	if err := (ProductSearchRequest{Sort: ProductSortSales}).Validate(); err == nil {
		t.Fatal("request without a query or category was accepted")
	}
}
