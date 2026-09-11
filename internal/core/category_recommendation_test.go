package core

import (
	"strings"
	"testing"
)

func TestRecommendationSearchScopeValidation(t *testing.T) {
	for _, test := range []struct {
		query, category string
		valid           bool
	}{
		{"synthetic", "", true}, {"", "123456", true}, {"  ", "123456", true},
		{"", "", false}, {"synthetic", "123456", false},
		{"", "../search", false}, {"", "123?x=1", false}, {"", "123/", false},
		{"", " 123", false}, {"", "１２３", false}, {"", strings.Repeat("1", 25), false},
		{strings.Repeat("a", 201), "", false}, {string([]byte{0xff}), "", false},
	} {
		r := ProductRecommendationRequest{Query: test.query, CategoryID: test.category}
		if (r.Validate() == nil) != test.valid {
			t.Fatalf("unexpected scope validation: %+v", test)
		}
		searchErr := (ProductSearchRequest{Query: test.query, CategoryID: test.category}).Validate()
		if (searchErr == nil) != test.valid {
			t.Fatal("recommendation and search identity validation disagree")
		}
	}
}
