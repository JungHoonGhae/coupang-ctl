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

func TestProductSearchRejectsQueryCombinedWithCategory(t *testing.T) {
	if err := (ProductSearchRequest{Query: "synthetic", CategoryID: "12345"}).Validate(); err == nil {
		t.Fatal("query would be silently ignored by category navigation")
	}
}
