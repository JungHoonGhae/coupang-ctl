package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func (s *SQLite) RecordPriceObservations(ctx context.Context, observations []core.ProductPriceObservation) error {
	if len(observations) == 0 {
		return nil
	}
	if len(observations) > 100 {
		return errors.New("price observation batch exceeds 100 items")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin price observation transaction: %w", err)
	}
	defer tx.Rollback()
	for _, observation := range observations {
		key, err := validatePriceObservation(observation)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO product_price_observations(
			product_key, product_id, item_id, vendor_item_id, name, canonical_url,
			current_amount, original_amount, discount_rate, currency, source, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			key, observation.Reference.ProductID, observation.Reference.ItemID,
			observation.Reference.VendorItemID, strings.TrimSpace(observation.Name),
			observation.CanonicalURL, observation.CurrentAmount, observation.OriginalAmount,
			observation.DiscountRate, observation.Currency, observation.Source,
			observation.ObservedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record product price observation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit price observations: %w", err)
	}
	return nil
}

func (s *SQLite) ListPriceObservations(ctx context.Context, request core.ProductPriceHistoryRequest) ([]core.ProductPriceObservation, bool, error) {
	if err := request.Validate(); err != nil {
		return nil, false, err
	}
	if request.Limit == 0 {
		return nil, false, errors.New("price history limit must be resolved before storage")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT product_id, item_id, vendor_item_id, name,
		canonical_url, current_amount, original_amount, discount_rate, currency, source, observed_at
		FROM product_price_observations
		WHERE ((? != '' AND vendor_item_id = ?) OR (? = '' AND product_id = ?))
		ORDER BY observed_at DESC, id DESC LIMIT ?`,
		request.VendorItemID, request.VendorItemID, request.VendorItemID, request.ProductID, request.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list product price observations: %w", err)
	}
	defer rows.Close()
	observations := make([]core.ProductPriceObservation, 0, request.Limit+1)
	for rows.Next() {
		var observation core.ProductPriceObservation
		var observedAt string
		if err := rows.Scan(&observation.Reference.ProductID, &observation.Reference.ItemID,
			&observation.Reference.VendorItemID, &observation.Name, &observation.CanonicalURL,
			&observation.CurrentAmount, &observation.OriginalAmount, &observation.DiscountRate,
			&observation.Currency, &observation.Source, &observedAt); err != nil {
			return nil, false, fmt.Errorf("scan product price observation: %w", err)
		}
		observation.ObservedAt, err = time.Parse(time.RFC3339Nano, observedAt)
		if err != nil {
			return nil, false, fmt.Errorf("parse product price observation time: %w", err)
		}
		observation.Provenance = "observed"
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate product price observations: %w", err)
	}
	truncated := len(observations) > request.Limit
	if truncated {
		observations = observations[:request.Limit]
	}
	for left, right := 0, len(observations)-1; left < right; left, right = left+1, right-1 {
		observations[left], observations[right] = observations[right], observations[left]
	}
	return observations, truncated, nil
}

func (s *SQLite) PurgePriceObservations(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM product_price_observations")
	if err != nil {
		return 0, fmt.Errorf("purge product price observations: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged product price observations: %w", err)
	}
	return int(deleted), nil
}

func validatePriceObservation(observation core.ProductPriceObservation) (string, error) {
	if !core.NumericProductIdentifier(observation.Reference.ProductID) ||
		(observation.Reference.ItemID != "" && !core.NumericProductIdentifier(observation.Reference.ItemID)) ||
		(observation.Reference.VendorItemID != "" && !core.NumericProductIdentifier(observation.Reference.VendorItemID)) {
		return "", errors.New("price observation product identifiers must be numeric")
	}
	name := strings.TrimSpace(observation.Name)
	if name == "" || !utf8.ValidString(name) || len([]rune(name)) > 500 {
		return "", errors.New("price observation name is invalid")
	}
	if observation.CurrentAmount <= 0 || observation.OriginalAmount < 0 || observation.DiscountRate < 0 || observation.DiscountRate > 100 || observation.Currency != "KRW" {
		return "", errors.New("price observation amounts are invalid")
	}
	if observation.ObservedAt.IsZero() || (observation.Source != "coupang_product_search" && observation.Source != "coupang_product_inspection") {
		return "", errors.New("price observation source metadata is invalid")
	}
	if observation.CanonicalURL != "" {
		parsed, err := url.Parse(observation.CanonicalURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" || parsed.Path != "/vp/products/"+observation.Reference.ProductID {
			return "", errors.New("price observation canonical URL is invalid")
		}
	}
	if observation.Reference.VendorItemID != "" {
		return "vendor:" + observation.Reference.VendorItemID, nil
	}
	return "product:" + observation.Reference.ProductID, nil
}
