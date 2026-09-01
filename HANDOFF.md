# coupangctl research handoff

Last validated: 2026-09-01 (Asia/Seoul)

## Product thesis

Build a local commerce data layer for consumers rather than another DOM-driven shopping MCP. The high-value path is full order-history export, normalized local storage, repeat-purchase and spending analysis, and product comparison using current price, delivery, promotion, rating, and review data.

## Confirmed

### Authentication

- A dedicated Chrome session can bootstrap authentication.
- Email/password login triggered CAPTCHA and was not bypassed.
- Phone-number login with a human-provided OTP succeeded.
- OTPs were used ephemerally and were not persisted.
- Doppler project/config: `cli-mcp-lab/dev_coupang`.
- Secret names: `COUPANG_EMAIL`, `COUPANG_PASSWORD`, `COUPANG_PHONE`.

### Public and product surfaces

- Search results are server-rendered and can be extracted without scrolling.
- `https://www.coupang.com/next-api/review` returns structured review data when replayed with a valid anonymous web session.
- Product detail pages expose JSON endpoints for review summaries, paged reviews, quantity/price/delivery, promotions, inquiries, related products, recommendations, banners, and brand data.
- `https://reco.coupang.com/recommend/widget` exposes rich product metadata including identifiers, images, prices, discounts, delivery badges, rating counts, and rating averages.

### Orders

- Order list URL: `https://mc.coupang.com/ssr/desktop/order/list`.
- The response contains an approximately 147 KB `__NEXT_DATA__` JSON document.
- Validated location: `props.pageProps.domains.desktopOrder`.
- First response contained five order groups and structured pagination.
- Pagination fields: `hasPrev`, `prevYear`, `prevPageIndex`, `hasNext`, `nextYear`, `nextPageIndex`.
- Order data includes order date/ID, totals, product and vendor-item IDs, names, quantities, list/discounted prices, images, shipment status, carrier and invoice information, estimated/promised/delivered dates, seller metadata, cancellation/return/exchange state, review eligibility, reorder eligibility, fees, and brand information.
- Routine extraction does not require DOM selectors; parse `__NEXT_DATA__` directly.

## Important limitation

Pure Node HTTP replay of the order-list document returned `403` with minimal headers and `406` even after replaying captured browser headers. Review JSON replay succeeded, but strict browser-process-free order retrieval has not yet been proven.

The pragmatic first architecture is:

1. Use a minimal Chromium session only for authentication and protected order-document retrieval.
2. Parse embedded JSON directly; do not click, scroll, or scrape rendered order cards.
3. Normalize results into local SQLite.
4. Run all analysis, search, comparison, export, CLI, and MCP operations locally without browser interaction.
5. Investigate the server-side order API used to populate Next.js SSR as the next protocol-research target.

## Proposed first milestones

1. `coupangctl auth` — isolated profile, phone OTP, session status, logout.
2. `coupangctl orders sync --all` — year/page traversal with checkpointing and redacted logs.
3. SQLite schema for orders, shipments, products, prices, sellers, and sync state.
4. `coupangctl orders list`, `spend`, `reorder`, and JSON/CSV export.
5. Product/review client with ad labelling, price normalization, and comparison.
6. MCP server layered over the same typed core rather than a separate browser implementation.

## Security and compliance

- Never commit credentials or session cookies.
- Prefer OS keychain or an encrypted local session store for runtime cookies; Doppler is for development bootstrap only.
- Redact PII and stable identifiers from logs and fixtures.
- Use synthetic fixtures in tests.
- No checkout, purchase, payment, cancellation, return, or account-setting mutation without an explicit separately designed confirmation boundary.
- Review applicable terms and legal constraints before public release.

## Prior-art observation

The inspected `coupang-browser-mcp` project used Playwright CDP and DOM/embedded-data extraction. At the time of research it had no GitHub stars or forks and its order tooling parsed rendered markup. `coupangctl` should differentiate through durable local data, protocol-first extraction, broad history, and useful consumer analytics.

