package core

import "testing"

func TestCategoryCatalogRequestBoundsUserControlledSearch(t *testing.T) {
	if err := (CategoryCatalogRequest{Query: "생활용품", Limit: 50}).Validate(); err != nil {
		t.Fatalf("valid category catalog request rejected: %v", err)
	}
	if err := (CategoryCatalogRequest{Limit: 201}).Validate(); err == nil {
		t.Fatal("oversized category catalog limit was accepted")
	}
	oversized := make([]rune, 101)
	for index := range oversized {
		oversized[index] = '가'
	}
	if err := (CategoryCatalogRequest{Query: string(oversized), Limit: 10}).Validate(); err == nil {
		t.Fatal("oversized category query was accepted")
	}
}
