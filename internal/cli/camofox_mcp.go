package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/partners"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP discovery is independent of both browser configuration and the ledger.
// This owner shares one lazily opened ledger until the stdio session drains.
// Browser configuration is reread per source tool so setup can be repaired
// without reconnecting the AI client. Local tools never read that configuration.
type camofoxMCPRuntime struct {
	mu     sync.Mutex
	ledger *store.SQLite
	closed bool
}

func runCamofoxMCP(ctx context.Context, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime := &camofoxMCPRuntime{}
	defer runtime.close()
	return mcpserver.NewWithFactories(runtime.factories(), version).Run(ctx, &mcp.StdioTransport{})
}

func (r *camofoxMCPRuntime) factories() mcpserver.ProviderFactories {
	return mcpserver.ProviderFactories{
		Auth: func(ctx context.Context, _ string) (mcpserver.StatusProvider, error) {
			source, err := r.source(ctx)
			if err != nil {
				return nil, err
			}
			return auth.NewService(source), nil
		},
		AuthRecovery: func(ctx context.Context, _ string) (mcpserver.AuthRecoveryProvider, error) {
			source, err := r.source(ctx)
			if err != nil {
				return nil, err
			}
			return auth.NewService(source), nil
		},
		OrderPreview: func(ctx context.Context, _ string) (mcpserver.OrderPreviewProvider, error) {
			source, err := r.source(ctx)
			if err != nil {
				return nil, err
			}
			return orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox), nil
		},
		Orders: func(ctx context.Context, tool string) (mcpserver.OrderProvider, error) {
			var source orders.PageSource
			if tool == "orders_sync" || tool == "orders_enrich_categories" {
				var err error
				source, err = r.source(ctx)
				if err != nil {
					return nil, err
				}
			}
			ledger, err := r.localLedger(ctx)
			if err != nil {
				return nil, err
			}
			return orders.NewWithPageSourceAndSyncSource(ledger, source, core.SyncSourceCamofox), nil
		},
		Products: func(ctx context.Context, tool string) (mcpserver.ProductProvider, error) {
			return r.productService(ctx, tool)
		},
		Recommendations: func(ctx context.Context, tool string) (mcpserver.ProductRecommendationProvider, error) {
			return r.productService(ctx, tool)
		},
		Account: func(ctx context.Context, _ string) (mcpserver.AccountProvider, error) {
			source, err := r.source(ctx)
			if err != nil {
				return nil, err
			}
			ledger, err := r.localLedger(ctx)
			if err != nil {
				return nil, err
			}
			return account.NewWithCosts(source, ledger), nil
		},
	}
}

func (r *camofoxMCPRuntime) source(ctx context.Context) (*browser.Camofox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		return nil, mcpserver.ErrBrowserSetupRequired
	}
	cfg, err := browser.ReadCamofoxConfig(paths.StateDir)
	if err != nil {
		return nil, mcpserver.ErrBrowserSetupRequired
	}
	return browser.NewCamofox(paths.StateDir, cfg), nil
}

func (r *camofoxMCPRuntime) localLedger(ctx context.Context) (*store.SQLite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.closed {
		return nil, mcpserver.ErrLocalDataUnavailable
	}
	if r.ledger != nil {
		return r.ledger, nil
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		return nil, mcpserver.ErrLocalDataUnavailable
	}
	ledger, err := store.Open(ctx, filepath.Join(paths.StateDir, "camofox-coupangctl.sqlite3"))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, mcpserver.ErrLocalDataUnavailable
	}
	r.ledger = ledger
	return ledger, nil
}

func (r *camofoxMCPRuntime) productService(ctx context.Context, tool string) (*products.Service, error) {
	var source products.Source
	var affiliate products.AffiliateLinker
	switch tool {
	case "product_price_history", "product_watchlist", "product_watch_add", "product_watch_remove":
		// These tools operate on already observed, private-local evidence only.
	default:
		browserSource, err := r.source(ctx)
		if err != nil {
			return nil, err
		}
		source = coupangproducts.New(browserSource)
		affiliate = partners.NewFromEnvironment(os.Getenv)
	}
	switch tool {
	case "products_search", "product_inspect", "products_recommend":
		// Local evidence is optional to source reads. Open it only when the
		// service records a verified price or fulfills an explicit purchase-
		// context request. Its existing warnings preserve source results when
		// storage fails; local-history/watch tools still require their ledger.
		return products.NewWithAffiliateAndPrices(source, affiliate, r).WithPurchaseHistory(r), nil
	}
	ledger, err := r.localLedger(ctx)
	if err != nil {
		return nil, err
	}
	return products.NewWithAffiliateAndPrices(source, affiliate, ledger).WithPurchaseHistory(ledger), nil
}

func (r *camofoxMCPRuntime) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.ledger != nil {
		_ = r.ledger.Close()
	}
}
