package core

import (
	"errors"
	"time"
)

const PriceHistorySchemaVersion = 1

type ProductPriceHistoryRequest struct {
	ProductID    string `json:"product_id" jsonschema:"Public numeric product identifier returned by products_search"`
	VendorItemID string `json:"vendor_item_id,omitempty" jsonschema:"Optional exact numeric vendor item identifier; omit to return separate series for every observed option"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum stored observations returned across all series,minimum=1,maximum=500"`
}

func (r ProductPriceHistoryRequest) Validate() error {
	if !NumericProductIdentifier(r.ProductID) || (r.VendorItemID != "" && !NumericProductIdentifier(r.VendorItemID)) {
		return errors.New("product_id and vendor_item_id must be numeric")
	}
	if r.Limit < 0 || r.Limit > 500 {
		return errors.New("limit must be between 1 and 500")
	}
	return nil
}

// ProductPriceObservation is local evidence captured from a successful public
// search or inspection. It never contains an affiliate URL or private account data.
type ProductPriceObservation struct {
	Reference      ProductReference `json:"reference"`
	Name           string           `json:"name"`
	CanonicalURL   string           `json:"canonical_url,omitempty"`
	CurrentAmount  int64            `json:"current_amount"`
	OriginalAmount int64            `json:"original_amount,omitempty"`
	DiscountRate   int              `json:"discount_rate,omitempty"`
	Currency       string           `json:"currency"`
	ObservedAt     time.Time        `json:"observed_at"`
	Source         string           `json:"source"`
	Provenance     string           `json:"provenance"`
}

type ProductPriceHistory struct {
	SchemaVersion    int                            `json:"schema_version"`
	Visibility       string                         `json:"visibility"`
	ProductID        string                         `json:"product_id"`
	VendorItemID     string                         `json:"vendor_item_id,omitempty"`
	ObservationCount int                            `json:"observation_count"`
	SeriesCount      int                            `json:"series_count"`
	FirstReturnedAt  *time.Time                     `json:"first_returned_at,omitempty"`
	LastReturnedAt   *time.Time                     `json:"last_returned_at,omitempty"`
	Series           []ProductPriceSeries           `json:"series"`
	Coverage         ProductPriceHistoryCoverage    `json:"coverage"`
	Definitions      ProductPriceHistoryDefinitions `json:"definitions"`
	Warnings         []string                       `json:"warnings"`
}

type ProductPriceSeries struct {
	Identity     string                    `json:"identity"`
	Reference    ProductReference          `json:"reference"`
	LatestName   string                    `json:"latest_name"`
	CanonicalURL string                    `json:"canonical_url,omitempty"`
	Observations []ProductPriceObservation `json:"observations"`
	Trend        ProductPriceTrend         `json:"trend"`
}

type ProductPriceTrend struct {
	ObservationCount               int     `json:"observation_count"`
	FirstReturnedAmountKRW         int64   `json:"first_returned_amount_krw"`
	LatestAmountKRW                int64   `json:"latest_amount_krw"`
	MinimumAmountKRW               int64   `json:"minimum_amount_krw"`
	MaximumAmountKRW               int64   `json:"maximum_amount_krw"`
	ChangeFromFirstReturnedKRW     int64   `json:"change_from_first_returned_krw"`
	ChangeFromFirstReturnedPercent float64 `json:"change_from_first_returned_percent"`
	Direction                      string  `json:"direction"`
	Provenance                     string  `json:"provenance"`
}

type ProductPriceHistoryCoverage struct {
	ReturnedObservations int  `json:"returned_observations"`
	Limit                int  `json:"limit"`
	Truncated            bool `json:"truncated"`
}

type ProductPriceHistoryDefinitions struct {
	PriceSource    string `json:"price_source"`
	SeriesIdentity string `json:"series_identity"`
	Trend          string `json:"trend"`
	HistoryStart   string `json:"history_start"`
}

type ProductPriceHistoryPurgeResult struct {
	ObservationsDeleted int `json:"observations_deleted"`
}
