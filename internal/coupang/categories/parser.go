package categories

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const (
	maxCategoryDocumentBytes = 2 << 20
	maxCategoryLevels        = 12
)

var ErrCategoryDataMissing = errors.New("product category data missing")

type envelope struct {
	Documents []json.RawMessage `json:"json_ld"`
}

type breadcrumb struct {
	Type     string          `json:"@type"`
	Elements []breadcrumbRow `json:"itemListElement"`
}

type breadcrumbRow struct {
	Type     string `json:"@type"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Item     string `json:"item"`
}

func ParseProductCategory(document []byte) (core.ProductCategory, error) {
	if len(document) == 0 || len(document) > maxCategoryDocumentBytes {
		return core.ProductCategory{}, ErrCategoryDataMissing
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	var payload envelope
	if err := decoder.Decode(&payload); err != nil || len(payload.Documents) == 0 {
		return core.ProductCategory{}, ErrCategoryDataMissing
	}
	for _, raw := range payload.Documents {
		var candidate breadcrumb
		if err := json.Unmarshal(raw, &candidate); err != nil || candidate.Type != "BreadcrumbList" {
			continue
		}
		path, err := categoryPath(candidate.Elements)
		if err != nil {
			return core.ProductCategory{}, err
		}
		if len(path) > 0 {
			return core.ProductCategory{Source: core.CategorySourceProductJSONLDBreadcrumb, Path: path}, nil
		}
	}
	return core.ProductCategory{}, ErrCategoryDataMissing
}

func categoryPath(rows []breadcrumbRow) ([]core.ProductCategoryNode, error) {
	if len(rows) == 0 || len(rows) > maxCategoryLevels+2 {
		return nil, ErrCategoryDataMissing
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Position < rows[j].Position })
	path := make([]core.ProductCategoryNode, 0, len(rows))
	lastPosition := 0
	for _, row := range rows {
		if row.Type != "ListItem" || row.Position <= lastPosition {
			return nil, fmt.Errorf("%w: invalid breadcrumb order", core.ErrInvalidOrderData)
		}
		lastPosition = row.Position
		name := strings.TrimSpace(row.Name)
		if name == "" || len([]rune(name)) > 100 {
			return nil, fmt.Errorf("%w: invalid breadcrumb label", core.ErrInvalidOrderData)
		}
		parsed, err := url.Parse(row.Item)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" {
			return nil, fmt.Errorf("%w: invalid breadcrumb target", core.ErrInvalidOrderData)
		}
		if categoryID, ok := categoryTarget(parsed.Path); ok {
			path = append(path, core.ProductCategoryNode{ID: categoryID, Name: name, Position: row.Position})
		}
	}
	if len(path) == 0 || len(path) > maxCategoryLevels {
		return nil, ErrCategoryDataMissing
	}
	return path, nil
}

func categoryTarget(path string) (string, bool) {
	const prefix = "/np/categories/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(path, prefix)
	if strings.Contains(id, "/") || id == "" {
		return "", false
	}
	_, err := strconv.ParseUint(id, 10, 64)
	return id, err == nil
}
