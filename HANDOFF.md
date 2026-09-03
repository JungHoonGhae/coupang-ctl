# coupangctl research handoff

Last validated: 2026-09-03 (Asia/Seoul)

## Product thesis

Build a local commerce data layer for consumers rather than another DOM-driven shopping MCP. The high-value path is full order-history export, normalized local storage, repeat-purchase and spending analysis, and product comparison using current price, delivery, promotion, rating, and review data.

## Confirmed

### Product implementation

- The distributed product is now a Go 1.26 module; TypeScript remains limited
  to development probes.
- `coupangctl version`, `doctor`, authentication, resumable order sync, local
  order queries, spending summaries, cancellation/return statistics,
  purchase/delivery trends, reorder candidates, normalized
  export/import, explicit purge, and the local stdio MCP server are implemented.
- `coupangctl capabilities` schema v2 lists each capability's concrete
  `implemented` surfaces separately from `next_step_kind`, `blocked_by`, and
  `last_verified`. External approval, explicit mutation authorization,
  missing source evidence, and elapsed-time validation are therefore visible
  without mislabeling them as unfinished code.
- CLI and MCP share the same typed authentication and order services. MCP
  exposes authentication status, sync, list, spend, reorder-candidate, and
  normalized-export tools. Destructive purge is intentionally CLI-only.
- `account benefits` and MCP `account_benefits` expose an experimental
  `private_local` account snapshot: current membership state/fee, source-
  reported benefit aggregates, registered payment-method brand/type/issuer,
  and expected plus monthly observed WOW Card reward aggregates. Account
  identifiers and raw cash transaction text are discarded.
- The same account response joins a separate local-ledger cost provider. It
  accepts only membership-only orders with explicit normalized metadata and
  never classifies by product name or amount. A complete 73-page live sync
  contained only the source enum pairs `SKU/GOODS` and `NORMAL/GOODS`, so zero
  matching rows is returned as `unavailable_no_explicit_membership_order_metadata`,
  not as zero fees paid.
- A headed, metadata-only live probe verified that the membership UI labels
  its displayed savings as `recent 3 months`. Schema v2 exposes that observed
  window. Its membership-only comparison uses current monthly fee times three
  only as an `inferred` estimate; pauses, refunds, free periods, and fee changes
  remain explicit limitations. Confirmed net value remains unset, and card
  rewards/card fees stay outside the comparison. Official Coupang guidance
  points membership-fee cash receipts to the PC receipt screen, making receipt
  evidence the next path for exact historical costs.
- `receipts status`, `receipts list`, and `receipts summary` now expose typed
  `private_local` cash/card receipt reads through both CLI and MCP. Live,
  metadata-only checks verified status, empty history pagination, aggregate
  count/amount shapes, and a card-method aggregate without printing card
  identifiers or raw values.
- `receipts download` can save an already-completed history artifact to a new
  private `0600` file. It re-reads the selected history row, keeps the source
  URL in browser memory, validates the final Coupang host and bounded content,
  and never overwrites a path. The parser and file behavior have synthetic
  contracts; the current live history had no completed artifact to download.
- Receipt request-creation POST routes are deliberately excluded. No supported
  command creates a receipt job, and no final purchase or payment automation is
  introduced.
- Gross spend remains backward compatible while a typed `commerce` breakdown
  separates product-purchase orders, explicit membership-fee orders, and
  unclassified legacy rows. Product behavior/statistics exclude explicit
  membership lines throughout time, delivery, brand, repeat, basket, and
  private product-spend queries.
- QR is now the default login mode. It enters through the protected order URL,
  preserving the return context needed by the order host; `--phone` remains a
  manual fallback.
- The manual `--phone` fallback launches an installed Chrome-family browser
  headed. Read-only verification and sync restore the private Coupang session
  into a short-lived dedicated Chrome profile with an ephemeral loopback CDP
  endpoint.
- `auth login --qr-output PATH` provides an optional server/Xvfb presentation
  adapter. It uses a headed browser, selects only the QR tab, creates a private
  `0600` screenshot, waits for approval, verifies return to the order page, and
  removes the screenshot on every terminal path. No QR value enters CLI JSON.
- `auth login --link` is an explicit ephemeral presentation adapter. The
  headed Chrome page decodes its visible QR with `BarcodeDetector`, validates
  the allowlisted Coupang app/login URL plus the two-digit approval number, and
  writes both once to stderr. Neither value enters JSON, files created by the
  product, persisted sessions, fixtures, structured logs, or error messages;
  callers must not redirect this explicit output into logs. A fresh-profile
  live check decoded and validated both fields before cancellation.
- Default QR login uses the same narrow headed adapter without writing a PNG;
  it automatically selects the QR tab in the visible browser and returns
  `verified` only after the exact protected order page finishes loading.
- Headless verification and sync have an explicit `--headed` read-only fallback
  when the protected document returns an access-denied response.
- Authentication state is captured after human-approved login into a private,
  atomic `0600` session file. A later process restores it into installed Chrome
  and rotates it only after a successful authenticated read; cookie values are
  never printed or passed through CLI/MCP outputs.
- The initial order document bootstraps the authenticated origin. Pagination
  then uses the UI's structured `GET /ssr/api/myorders/model` route with bounded
  `requestYear`, `pageIndex`, and `size` parameters. This fixed the year-boundary
  loop caused by treating UI `periodYear` as the model parameter.
- Order responses are normalized into SQLite and checkpointed per page. Sync is
  idempotent and resumable; only a complete from-start history walk reconciles
  stale normalized rows.
- Current normalization retains precise purchase/delivery timestamps,
  cancellation and return quantities, receipt eligibility, brand metadata, and
  gross order totals. Analytics expose purchase time/day/month distributions,
  return/cancellation rates, and shipment-duration average/median/p90 by year.
- `shopping_profile_v4` uses four denominator-visible observed-behavior axes:
  purchase-day concentration against a same-sample uniform null, literal
  day/night order majority, repeat-choice majority, and single-product basket
  majority. Each axis carries its sample, observation window, threshold basis,
  and provenance. Insufficient data produces `?` instead of a guessed type.
- The standalone recap now embeds a 16-character vector roster and shows its
  evidence: month-precision analysis window, axis receipts, yearly delivery
  bars, 24-hour order distribution, zero-filled monthly history, repeat and
  basket denominators, and source-native category coverage.
- `orders products` and MCP `orders_product_insights` expose a separate
  `private_local` product-level response: leaders by retained units, distinct
  orders, and eligible item paid amount; highest and lowest derived paid-unit
  amount; calendar-month average spend; and highest/lowest positive-spend day
  with up to five observed product lines. Product identity uses vendor-item ID
  then product ID and never name similarity. The response exposes ID and spend
  coverage and contains no stable product IDs.
- `orders recap --include-products` renders those real names, exact dates, and
  amounts only after an explicit privacy opt-in. The default recap stays
  `public_safe`; the opted-in result is `private_products`, keeps names blurred
  until clicked, and warns that the HTML itself must not be shared.
- Product category enrichment is implemented as a bounded, resumable command.
  It reads the product page's JSON-LD breadcrumb, stores every observed path
  node, groups aggregates by the leaf node, and leaves inaccessible products
  explicitly unavailable rather than inferring from names.
- The first full bounded category pass reached zero pending product references.
  Both source-native breadcrumbs and explicit unavailable outcomes occurred in
  live use. The adapter remains experimental until path stability is checked
  over time and on more than one account.
- `orders category-catalog` and MCP `orders_category_catalog` expose only the
  category IDs, labels, and path prefixes actually observed in those cached
  breadcrumbs. Query matching and distinct local-product counts are labeled
  derived, coverage is explicit, and returned IDs can be passed to
  `products_search` without inventing a category mapping or calling the
  browser during catalog lookup.
- Successful product search and inspection results now append only explicitly
  observed current prices to a local SQLite history. Vendor-item identities are
  separate series, affiliate URLs are never stored, and the CLI supports an
  explicitly confirmed price-history-only purge.
- `products price-history` and MCP `product_price_history` expose the bounded
  local history, per-series min/max/change, provenance, truncation, and the
  non-retroactive coverage boundary. A public live search and exact inspection
  recorded two observations for one option and read both back through the typed
  command.
- `orders reorder` now compares the latest exact-identity local observation
  with the latest fully retained paid-unit order evidence when both exist. It
  returns explicit unavailable states otherwise and does not fetch a fresh
  price or imply a guaranteed checkout price.
- An exact-identity watchlist is implemented in SQLite, CLI, and MCP. Adding a
  watch requires an existing observation; bounded refresh inspects only entries
  not checked within the requested interval and records observed/unavailable/
  failed status without affiliate conversion or commerce mutation. A live
  add-refresh-list-remove pass succeeded and left no test watch behind.
- `products watch-schedule` renders daily launchd, systemd, cron, or Windows
  Task Scheduler artifacts without opening a browser. Optional output writes
  private `0600` files without overwrite; scheduler activation stays an
  explicit human step. Every generated command calls only bounded headless
  `watch-refresh`, never affiliate conversion, cart, checkout, order, or
  payment.
- `orders recap-image` is a two-step public-share workflow. Without
  confirmation it returns the exact month-level values, axis evidence,
  sample sizes, provenance, and exclusion list that would appear. With a new
  `.png` path and `--confirm-public-safe-image`, it renders a 1080x1350 card
  from a temporary local HTML file, validates the PNG dimensions, writes it
  `0600` without overwrite, and removes the temporary source. Private product
  names, amounts, exact dates, category/brand labels, and payment data have no
  PNG inclusion flag. A live local render and pixel inspection succeeded.
- QR image output is cropped to the QR region and enlarged; full login-page
  screenshots are no longer written.
- macOS can request an already-visible OTP send/resend control via the native
  accessibility tree. This optional helper uses semantic controls rather than
  screen coordinates and never reads, accepts, or stores the OTP.
- The CGO-free SQLite and MCP dependency set cross-builds for macOS, Windows,
  and Linux on both arm64 and amd64.

### Authentication

- A dedicated Chrome session can bootstrap authentication.
- Coupang exposes a desktop QR login. Redacted headed-browser research observed
  `POST /login/qrcode/create.pang` and `/login/qrcode/query.pang`; the create
  response contains QR link/code, verification-code, status, and creation-time
  fields. Values were not logged or persisted.
- A generic QR login successfully authenticated the main Coupang site but did
  not authenticate a later direct order-host visit. Entering login through the
  protected order URL is therefore a required session/return-context invariant,
  not a cosmetic redirect.
- Fresh true-headless Chrome received HTTP 403 at the protected order entry,
  while a headed Chrome renderer reached the login page and created a QR. Linux
  server login therefore currently requires a headed display such as Xvfb;
  routine verification and sync remain headless-first.
- Native headed Chrome with a manual click successfully sent a phone OTP; a
  Playwright-driven click on the same flow received `403` from
  `/login/v2/pincode/send`. Production login should therefore launch the system
  browser and leave challenge interaction to the user.
- Email/password login triggered CAPTCHA and was not bypassed.
- Phone-number login with a human-provided OTP succeeded.
- OTPs were used ephemerally and were not persisted.
- Orca/computer-use is a research aid, not a runtime requirement. Machines with
  no headed renderer should consume normalized exports rather than transferring
  cookies or automating CAPTCHA/OTP; Xvfb can supply the headed renderer used by
  the QR-output adapter.
- Doppler project/config: `cli-mcp-lab/dev_coupang`.
- Secret names: `COUPANG_EMAIL`, `COUPANG_PASSWORD`, `COUPANG_PHONE`.

### Public and product surfaces

- Search results are server-rendered and can be extracted without scrolling.
- Optional Coupang Partners deeplinks are implemented behind the official
  HMAC-signed API adapter. Canonical URLs are never replaced; `affiliate_url`
  is separate, failures preserve canonical output, disclosure is definite,
  buyer price verification and self-purchase ineligibility are explicit, and
  CLI/MCP callers can opt out per request or process-wide. Business signup is
  complete; final activity-channel approval and live API-key validation remain
  pending, so the capability is experimental.
- Typed product search supports an ordinary query or a numeric source-native
  category ID. Verified source controls are `scoreDesc` (쿠팡 랭킹순),
  `saleCountDesc` (판매량순), `latestAsc`, `salePriceAsc`, and
  `salePriceDesc`. Rating/review-count orders are local observed-field sorts.
  Responses preserve the selected scope, provenance, and original page
  position; 판매량순 does not claim an unavailable absolute sales count.
- A live private-local catalog-to-search pass selected an observed leaf ID
  without logging it and returned five current category results with category
  scope, `sales` applied as a source-native sort, and positive source positions.
  A later rapid multi-layout probe was denied in both headless and
  CDP-controlled headed Chrome. No bypass was attempted; the detail-layout
  coverage check remains a later retry, and the sanitized CLI error no longer
  incorrectly assumes every access denial came from headless mode.
- Listing options are collapsed by product ID by default. Search-card reviews
  can be product-page-wide, so they are labeled `product_page_observed` and are
  not attributed to a particular CPU/GPU/storage option. Exact vendor-item
  identities remain available for inspection and explicit cart addition.
- Computer memory, storage, CPU, GPU, OS, and explicit used/refurbished/display
  condition markers can be normalized from the observed option title. The
  evidence source remains `observed_product_title`; no missing part is guessed.
- `https://www.coupang.com/next-api/review` returns structured review data when replayed with a valid anonymous web session.
- Product detail pages expose JSON endpoints for review summaries, paged reviews, quantity/price/delivery, promotions, inquiries, related products, recommendations, banners, and brand data.
- `https://reco.coupang.com/recommend/widget` exposes rich product metadata including identifiers, images, prices, discounts, delivery badges, rating counts, and rating averages.

### Orders and receipts

- Order list URL: `https://mc.coupang.com/ssr/desktop/order/list`.
- The bootstrap response contains `__NEXT_DATA__` at
  `props.pageProps.domains.desktopOrder`.
- Pagination uses `GET /ssr/api/myorders/model`; the response is a direct JSON
  model with `orderList`, `hasNext`, `nextYear`, and `nextPageIndex`.
- First response contained five order groups and structured pagination.
- Pagination fields: `hasPrev`, `prevYear`, `prevPageIndex`, `hasNext`, `nextYear`, `nextPageIndex`.
- Order data includes order date/ID, totals, product and vendor-item IDs, names, quantities, list/discounted prices, images, shipment status, carrier and invoice information, estimated/promised/delivered dates, seller metadata, cancellation/return/exchange state, review eligibility, reorder eligibility, fees, and brand information.
- Routine extraction does not require DOM selectors; parse the structured model
  directly.
- The receipt page bootstraps same-origin cash/card request-status, paged
  download-history, and period-summary reads. Those routes are adopted behind
  a narrow adapter; response URLs and card identifiers do not cross into the
  typed core.
- The credit-card receipt summary exposes selected-card, date range, amount,
  and count shapes, but no installment-month field is verified. Typed receipt
  summaries therefore expose observed payment-method totals while reporting
  installments as `unavailable` rather than guessing lump-sum/installment
  splits.

## Important limitation

Pure Node HTTP replay of the order-list document returned `403` with minimal headers and `406` even after replaying captured browser headers. Review JSON replay succeeded, but strict browser-process-free order retrieval has not been proven. The product restores its private Coupang session into a real installed Chrome process and performs the model fetch in the authenticated same-origin page.

The pragmatic first architecture is:

1. Use installed Chrome for authentication and protected structured retrieval.
2. Parse the order-model JSON directly; do not click, scroll, or scrape rendered order cards during routine sync.
3. Normalize results into local SQLite.
4. Run normalization, analysis, comparison, export, CLI, and MCP orchestration
   locally; isolate live product and order retrieval inside the installed-browser
   source adapters.
5. Keep newly discovered APIs in the redacted endpoint catalog and promote them
   only after read/write classification and synthetic contract tests.

## Authentication architecture decision

The inspected `tossinvest-cli` is useful prior art, but its authentication
implementation should not be copied directly. Toss exposes a QR and phone-app
approval flow, after which its authenticated web session can be imported into a
typed HTTP client with a coherent browser user agent. Even there, a person must
perform the second-factor approval.

Coupang's observed phone flow and protected order document have different
constraints: OTP sending is coupled to genuine browser interaction, and direct
HTTP replay of the protected document was denied. The product therefore keeps
the session inside a dedicated Chrome profile instead of copying cookies or
storage state. The user-visible goal remains similar: human participation only
for the authentication challenge, with a browser-owned implementation that is
headless-first for routine reads. An official read-only API, sandbox, or
developer allowlist can replace this source adapter later without changing the
rest of the product.

## Roadmap

See `ROADMAP.md` or run `coupangctl capabilities`. P0 order-history and local
analytics, explainable shopping types, achievements, and a public-safe local
HTML recap, private local product receipts, and experimental membership-benefit
snapshots are implemented. Product-type/category rankings and computer-title
spec normalization are experimental; category-label discovery and
selected-option coverage across layouts are the next search tasks.
Source-native purchase-category enrichment is also experimental while its
stability is validated. Cash/card receipt reads are now experimental; completed
artifact and vendor-receipt validation remain. Exact-option local price history
and repurchase comparison are experimental; the scheduler-ready watch command
is implemented. Longitudinal threshold validation and optional platform-specific
scheduler installers are next.

## Security and compliance

- Never commit credentials or session cookies.
- The current session file is private mode `0600` and atomically replaced. OS
  keychain/envelope encryption remains a hardening item; Doppler is for
  development bootstrap only.
- Redact PII and stable identifiers from logs and fixtures.
- Use synthetic fixtures in tests.
- No checkout, purchase, payment, cancellation, return, or account-setting mutation without an explicit separately designed confirmation boundary.
- Review applicable terms and legal constraints before public release.

## Prior-art observation

The inspected `coupang-browser-mcp` project used Playwright CDP and DOM/embedded-data extraction. At the time of research it had no GitHub stars or forks and its order tooling parsed rendered markup. `coupangctl` should differentiate through durable local data, protocol-first extraction, broad history, and useful consumer analytics.
