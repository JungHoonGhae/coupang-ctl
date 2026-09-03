package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type rowScanner interface {
	Scan(...any) error
}

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

func (s *SQLite) AddPriceWatch(ctx context.Context, request core.ProductWatchRequest, addedAt time.Time) (core.ProductWatchEntry, bool, error) {
	if err := request.Validate(); err != nil || addedAt.IsZero() {
		return core.ProductWatchEntry{}, false, errors.New("price watch request is invalid")
	}
	key := priceWatchKey(request)
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM product_price_watchlist WHERE product_key = ?)", key).Scan(&exists); err != nil {
		return core.ProductWatchEntry{}, false, fmt.Errorf("inspect product price watch: %w", err)
	}
	if !exists {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM product_price_watchlist").Scan(&count); err != nil {
			return core.ProductWatchEntry{}, false, fmt.Errorf("count product price watches: %w", err)
		}
		if count >= 500 {
			return core.ProductWatchEntry{}, false, errors.New("product price watchlist limit of 500 reached")
		}
	}
	var reference core.ProductReference
	var name, canonicalURL string
	err := s.db.QueryRowContext(ctx, `SELECT product_id, item_id, vendor_item_id, name, canonical_url
		FROM product_price_observations WHERE product_key = ?
		ORDER BY observed_at DESC, id DESC LIMIT 1`, key).Scan(
		&reference.ProductID, &reference.ItemID, &reference.VendorItemID, &name, &canonicalURL)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ProductWatchEntry{}, false, nil
	}
	if err != nil {
		return core.ProductWatchEntry{}, false, fmt.Errorf("read latest price observation for watch: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO product_price_watchlist(
		product_key, product_id, item_id, vendor_item_id, name, canonical_url, added_at, last_status
	) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
	ON CONFLICT(product_key) DO UPDATE SET
		product_id = excluded.product_id, item_id = excluded.item_id,
		vendor_item_id = excluded.vendor_item_id, name = excluded.name,
		canonical_url = excluded.canonical_url`,
		key, reference.ProductID, reference.ItemID, reference.VendorItemID,
		name, canonicalURL, addedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return core.ProductWatchEntry{}, false, fmt.Errorf("add product price watch: %w", err)
	}
	entry, err := s.priceWatchByKey(ctx, key)
	return entry, !exists, err
}

func (s *SQLite) RemovePriceWatch(ctx context.Context, request core.ProductWatchRequest) (core.ProductWatchEntry, bool, error) {
	if err := request.Validate(); err != nil {
		return core.ProductWatchEntry{}, false, err
	}
	key := priceWatchKey(request)
	entry, err := s.priceWatchByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ProductWatchEntry{Identity: key, Reference: core.ProductReference{ProductID: request.ProductID, VendorItemID: request.VendorItemID}}, false, nil
	}
	if err != nil {
		return core.ProductWatchEntry{}, false, err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM product_price_watchlist WHERE product_key = ?", key)
	if err != nil {
		return core.ProductWatchEntry{}, false, fmt.Errorf("remove product price watch: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return core.ProductWatchEntry{}, false, fmt.Errorf("count removed product price watch: %w", err)
	}
	return entry, deleted > 0, nil
}

func (s *SQLite) ListPriceWatches(ctx context.Context) ([]core.ProductWatchEntry, error) {
	return s.listPriceWatches(ctx, "", 500)
}

func (s *SQLite) ListDuePriceWatches(ctx context.Context, dueBefore time.Time, limit int) ([]core.ProductWatchEntry, error) {
	if dueBefore.IsZero() || limit < 1 || limit > 50 {
		return nil, errors.New("due price watch bounds are invalid")
	}
	return s.listPriceWatches(ctx, dueBefore.UTC().Format(time.RFC3339Nano), limit)
}

func (s *SQLite) CountDuePriceWatches(ctx context.Context, dueBefore time.Time) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_price_watchlist
		WHERE last_checked_at IS NULL OR last_checked_at <= ?`, dueBefore.UTC().Format(time.RFC3339Nano)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count due product price watches: %w", err)
	}
	return count, nil
}

func (s *SQLite) MarkPriceWatchChecked(ctx context.Context, reference core.ProductReference, checkedAt time.Time, status string) error {
	if checkedAt.IsZero() || (status != "observed" && status != "unavailable" && status != "failed") {
		return errors.New("price watch check metadata is invalid")
	}
	request := core.ProductWatchRequest{ProductID: reference.ProductID, VendorItemID: reference.VendorItemID}
	if err := request.Validate(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE product_price_watchlist
		SET last_checked_at = ?, last_status = ? WHERE product_key = ?`,
		checkedAt.UTC().Format(time.RFC3339Nano), status, priceWatchKey(request))
	if err != nil {
		return fmt.Errorf("mark product price watch checked: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count checked product price watch: %w", err)
	}
	if updated != 1 {
		return errors.New("product price watch was not found")
	}
	return nil
}

func (s *SQLite) PurgePriceWatches(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM product_price_watchlist")
	if err != nil {
		return 0, fmt.Errorf("purge product price watches: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged product price watches: %w", err)
	}
	return int(deleted), nil
}

func (s *SQLite) priceWatchByKey(ctx context.Context, key string) (core.ProductWatchEntry, error) {
	row := s.db.QueryRowContext(ctx, `SELECT product_key, product_id, item_id, vendor_item_id,
		name, canonical_url, added_at, last_checked_at, last_status
		FROM product_price_watchlist WHERE product_key = ?`, key)
	entry, err := scanPriceWatch(row)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return core.ProductWatchEntry{}, fmt.Errorf("read product price watch: %w", err)
	}
	return entry, err
}

func (s *SQLite) listPriceWatches(ctx context.Context, dueBefore string, limit int) ([]core.ProductWatchEntry, error) {
	query := `SELECT product_key, product_id, item_id, vendor_item_id,
		name, canonical_url, added_at, last_checked_at, last_status
		FROM product_price_watchlist`
	args := []any{}
	if dueBefore != "" {
		query += " WHERE last_checked_at IS NULL OR last_checked_at <= ?"
		args = append(args, dueBefore)
	}
	query += " ORDER BY COALESCE(last_checked_at, added_at), product_key LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list product price watches: %w", err)
	}
	defer rows.Close()
	entries := []core.ProductWatchEntry{}
	for rows.Next() {
		entry, err := scanPriceWatch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product price watch: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product price watches: %w", err)
	}
	return entries, nil
}

func scanPriceWatch(scanner rowScanner) (core.ProductWatchEntry, error) {
	var entry core.ProductWatchEntry
	var addedAt string
	var lastChecked sql.NullString
	if err := scanner.Scan(&entry.Identity, &entry.Reference.ProductID, &entry.Reference.ItemID,
		&entry.Reference.VendorItemID, &entry.Name, &entry.CanonicalURL,
		&addedAt, &lastChecked, &entry.LastStatus); err != nil {
		return core.ProductWatchEntry{}, err
	}
	var err error
	entry.AddedAt, err = time.Parse(time.RFC3339Nano, addedAt)
	if err != nil {
		return core.ProductWatchEntry{}, err
	}
	if lastChecked.Valid {
		checked, err := time.Parse(time.RFC3339Nano, lastChecked.String)
		if err != nil {
			return core.ProductWatchEntry{}, err
		}
		entry.LastCheckedAt = &checked
	}
	return entry, nil
}

func priceWatchKey(request core.ProductWatchRequest) string {
	if request.VendorItemID != "" {
		return "vendor:" + request.VendorItemID
	}
	return "product:" + request.ProductID
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
