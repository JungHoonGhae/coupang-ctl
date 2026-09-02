package account

import (
	"context"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

type DocumentSource interface {
	FetchAccountBenefits(context.Context, core.AccountBenefitsRequest) ([]byte, error)
}

type Client struct {
	source DocumentSource
}

func New(source DocumentSource) *Client {
	return &Client{source: source}
}

func (c *Client) Snapshot(ctx context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	document, err := c.source.FetchAccountBenefits(ctx, request)
	if err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	return ParseSnapshotDocument(document)
}
