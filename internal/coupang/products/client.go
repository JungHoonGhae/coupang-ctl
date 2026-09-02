package products

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type DocumentSource interface {
	FetchProductSearch(context.Context, core.ProductSearchRequest) ([]byte, error)
	FetchProductInspection(context.Context, core.ProductInspectRequest) ([]byte, error)
	AddProductToCart(context.Context, core.CartAddRequest) ([]byte, error)
}

func (c *Client) AddToCart(ctx context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	document, err := c.source.AddProductToCart(ctx, request)
	if err != nil {
		return core.CartAddResult{}, err
	}
	return ParseCartAddDocument(document, request)
}

type Client struct {
	source DocumentSource
}

func New(source DocumentSource) *Client {
	return &Client{source: source}
}

func (c *Client) Search(ctx context.Context, request core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	document, err := c.source.FetchProductSearch(ctx, request)
	if err != nil {
		return nil, core.ProductCoverage{}, err
	}
	return ParseSearchDocument(document)
}

func (c *Client) Inspect(ctx context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	document, err := c.source.FetchProductInspection(ctx, request)
	if err != nil {
		return core.ProductInspection{}, err
	}
	return ParseInspectionDocument(document, request)
}
