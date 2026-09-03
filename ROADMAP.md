# coupangctl roadmap

Status vocabulary:

- `available`: implemented, contract-tested, and verified against the live account flow.
- `experimental`: implemented and tested, but live backfill or longitudinal validation is still in progress.
- `researched`: protocol evidence exists; no supported product command yet.
- `planned`: valuable, but the data contract or safe operating boundary is not yet proven.

The machine-readable view is `coupangctl capabilities`.
Schema v2 also exposes `implemented`, `next_step_kind`, `blocked_by`, and
`last_verified`. A blocker is an evidence or coordination dependency, not a
license to guess the missing data or bypass an access control.

| Priority | Capability | Status | Why it matters | Exit criterion / next work |
| --- | --- | --- | --- | --- |
| P0 | Native auth session | available | One human login can support later read-only runs, including headless runs where accepted. | Keep login renewal explicit and verify session rotation in release tests. |
| P0 | Ordinary-browser protected-data bridge | experimental | A same-account comparison found the order UI available in the user's already-running ordinary Chrome while both headed and headless dedicated Chrome contexts received HTTP 403. The CLI and MCP typed sync paths, single-use authenticated rendezvous, Chrome Native Messaging host, minimal-permission MV3 action, isolated page reader, embedded extension bundle, cross-platform per-user install/doctor/ownership-checked uninstall, digest-authorized resumable upgrades, and SQLite path are implemented. A managed macOS install passed doctor and four consecutive one-page live reads without copying cookies; the final read used Chrome's verified managed `extension_path`. Linux CI runs the per-user file contract and Windows CI runs isolated real-HKCU install/doctor/uninstall; neither is yet a clean desktop Chrome test. The six-target release contract builds allowlisted archives, per-archive SBOMs, checksums, and provenance-ready artifacts. | Validate clean Chrome desktop profiles on Linux and Windows, complete Chrome Web Store privacy review and distribution, and add native OS signing. Normalized export/import remains the browserless-server path. |
| P0 | Full order history | available | All later analytics depend on complete, non-looping history. | Keep the private order-model endpoint behind its narrow adapter and synthetic contract tests. |
| P0 | Spend, cancellation, return stats | available | Keeps the gross ledger while separating observed product purchases, explicit membership fees, cancellations, and returns. | Vendor receipts expose source-native cancellation payment components; verify settlement status and multiple canceled/returned samples before labeling any figure exact post-refund net spend. |
| P0 | WOW membership and benefits | experimental | Shows current membership state/fee, the source fee-change date as schedule metadata, the observed recent-benefit window, an explicitly inferred current-fee comparison, registered payment-method brands, and observed monthly WOW Card rewards. | Adopt membership-fee receipt evidence for exact historical costs. The complete live order history exposed no membership-specific item metadata, so it must not be treated as zero fees paid. |
| P0 | Purchase and delivery trends | available | Answers purchase hour/day/month patterns and whether delivery became slower. | Keep average/median/p90 and year sample counts in release verification. |
| P0 | Shopping type and recap | available | Turns local aggregates into four explainable axes, achievements, characters, a public-safe HTML story, and a preview-gated 4:5 PNG share card. | Keep rule versions, thresholds, the share-field preview, and rendered image content aligned in release tests. |
| P0 | Private product insights | available | Shows which identified products lead by units, orders, and spend, plus paid-unit and spend-day receipts. | Keep exact names and dates outside shareable output; add current-price comparison only after its source contract is stable. |
| P0 | Natural-language product discovery | available | Lets an AI turn an ordinary Korean shopping request into bounded search filters, then inspect current public product evidence. | Keep multi-layout selected-option coverage and honest unavailable states for unobserved card benefits in release checks. |
| P0 | Source-native product rankings | available | Supports product-type and real-category views using separate Coupang ranking, sales, latest, and price controls while labeling local rating/review sorts honestly. | Keep query/category scope and source-native versus local sort semantics in release checks. |
| P0 | Transparent affiliate deep links | experimental | Adds an optional operator-owned Partners URL without replacing the canonical product URL, while exposing the definite commission disclosure, price notice, self-purchase exclusion, and opt-out state. | A live portal check confirmed that API-key generation is disabled until final approval. Complete that external approval, issue the keys, and run a credential-redacted live contract check. |
| P1 | Explicit cart add | experimental | Completes the useful shopping loop without crossing into purchase or payment. | Run a separately authorized live mutation test; require exact vendor item plus confirmation and never auto-retry an unverified attempt. |
| P1 | Batch receipts | experimental | Enables accounting, reimbursement, and archive workflows. | Cash/card availability, history, single-period and multi-year summaries, hashed-reference vendor-receipt reads, and private completed-archive download are implemented; validate a completed download live without implementing request creation. |
| P1 | Payment method and installments | experimental | Answers which payment methods funded observed receipt totals without confusing registered cards with usage. | Multi-year receipt-source totals, payment-method rankings, and per-order vendor payment types are implemented; keep installment status `unavailable` until an explicit installment-month field is observed. |
| P1 | Product category enrichment | experimental | Enables source-native category totals, an AI-searchable label/ID catalog, and an evidence-bearing stability report without guessing from product names. | Append-only observations, explicit bounded rechecks, and same-product multi-day assessment are implemented. A five-product live recheck found no path change; broaden the sample and validate across more consenting accounts. |
| P2 | Price history and repurchase | experimental | Helps decide when and what to buy again using exact-option evidence instead of name similarity. | Search/inspection observations, watchlist refresh, 24-hour staleness labels, last-paid-unit comparisons, and reviewable daily scheduler artifacts are implemented; validate real longitudinal price changes before tuning the threshold. |

## Current product boundaries

- Private endpoint details are unstable and remain isolated in Coupang-specific adapters.
- Read operations may use a real installed Chrome session. Public product reads
  are headless-first and may use narrowly isolated DOM fallbacks when no
  structured search document exists. No stealth or anti-detection bypass is
  part of the product.
- Protected reads can still be rejected in a short-lived dedicated Chrome
  context even when the same account works in an already-running ordinary
  Chrome window. The native adapter makes one delayed, idempotent retry before
  its existing headed fallback. This is resilience, not a bypass or a success
  guarantee. Orca remains a research aid; the experimental ordinary-browser
  bridge
  uses Chrome Native Messaging with a user-selected `activeTab`, packaged
  isolated-world code, and a closed normalized response protocol. It does not
  request cookie access or a broad host wildcard. Chrome talks only to its
  allowlisted native host; the native host then authenticates once to the
  already-running CLI over a private short-lived loopback rendezvous.
- Cart addition is the only supported commerce mutation. It is reversible,
  non-idempotent, explicitly confirmed, and separated from all purchase and
  payment controls.
- Receipt status, history, and summaries are structured read operations. A
  completed archive can be downloaded only to a new private `0600` file; its
  source URL is never returned or logged. Receipt generation/request submission
  is a write operation and is intentionally not implemented.
- A vendor receipt is addressed by the hashed `source_ref` returned by
  `orders list`. The raw order ID is resolved and discarded inside the browser
  adapter. Source cancellation component fields are preserved as observed
  fields and are not relabeled as a completed refund settlement.
- Category inference from product names is not used. The experimental source
  is the variable-length product-page breadcrumb path; no breadcrumb position
  is relabeled as a fixed Coupang “large/middle/small” field.
- Product-type ranking uses the caller's search query; category ranking uses an
  observed numeric category ID. `sales` preserves Coupang's 판매량순 ordering
  but does not claim an absolute sales count, which the observed page does not
  expose. Page-wide review totals are not attributed to a specific option.
- Price history begins only when a successful coupangctl search or inspection
  observes `price.current_amount`. Series use vendor-item ID when available and
  never merge different options. Repurchase comparison uses the latest stored
  observation and latest fully retained paid-unit evidence; it does not claim a
  live checkout price or retroactive market history.
- Affiliate URLs are generated only through the official Partners deeplink API
  when credentials are configured. They stay separate from canonical URLs,
  never claim a guaranteed lower price, always carry a definite commission
  disclosure, and can be disabled per request or globally. The operator's own
  purchases are explicitly marked ineligible.
- Final purchase and payment automation are out of scope.
- Shopping types describe observed cart behavior only. They are not psychological diagnoses and never infer sensitive household traits.
- Product names and exact dates are private local data. They are excluded from
  `orders insights` and the default recap; the private recap requires the
  explicit `--include-products` flag and is not a shareable artifact.
- A private-local command may show the signed-in user's requested product
  names, dates, card brands, and aggregates. Logs, tests, public recaps, and
  shared artifacts still use redacted metadata or synthetic data, and account
  numbers and raw payloads are never emitted.
