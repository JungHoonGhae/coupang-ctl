package core

import "testing"

func TestProductDocumentReadLimits(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 2, 3, 4} {
		search := ProductSearchRequest{Query: "synthetic", DocumentReadLimit: limit}
		if (search.Validate() == nil) != (limit >= 0 && limit <= 2) {
			t.Errorf("search allowance %d", limit)
		}
		detail := ProductInspectRequest{ProductID: "101", DocumentReadLimit: limit}
		if (detail.Validate() == nil) != (limit >= 0 && limit <= 3) {
			t.Errorf("detail allowance %d", limit)
		}
	}
}
