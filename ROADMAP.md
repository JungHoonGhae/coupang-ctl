# coupangctl roadmap

Status vocabulary:

- `available`: implemented, contract-tested, and verified against the live account flow.
- `experimental`: implemented and tested, but live backfill or longitudinal validation is still in progress.
- `researched`: protocol evidence exists; no supported product command yet.
- `planned`: valuable, but the data contract or safe operating boundary is not yet proven.

The machine-readable view is `coupangctl capabilities`.

| Priority | Capability | Status | Why it matters | Exit criterion / next work |
| --- | --- | --- | --- | --- |
| P0 | Native auth session | available | One human login can support later read-only runs, including headless runs where accepted. | Keep login renewal explicit and verify session rotation in release tests. |
| P0 | Full order history | available | All later analytics depend on complete, non-looping history. | Keep the private order-model endpoint behind its narrow adapter and synthetic contract tests. |
| P0 | Spend, cancellation, return stats | available | Keeps the gross ledger while separating observed product purchases, explicit membership fees, cancellations, and returns. | Add refund settlement data before labeling any figure as exact post-refund net spend. |
| P0 | WOW membership and benefits | experimental | Shows current membership state/fee, reported benefits, registered payment-method brands, and observed monthly WOW Card rewards. | Adopt historical membership-payment evidence before calculating lifetime fees or exact net value. |
| P0 | Purchase and delivery trends | available | Answers purchase hour/day/month patterns and whether delivery became slower. | Keep average/median/p90 and year sample counts in release verification. |
| P0 | Shopping type and recap | available | Turns local aggregates into four explainable axes, achievements, characters, and a public-safe HTML story. | Keep rule versions and thresholds stable; add image export only with a preview of the exact fields shared. |
| P0 | Private product insights | available | Shows which identified products lead by units, orders, and spend, plus paid-unit and spend-day receipts. | Keep exact names and dates outside shareable output; add current-price comparison only after its source contract is stable. |
| P0 | Natural-language product discovery | experimental | Lets an AI turn an ordinary Korean shopping request into bounded search filters, then inspect current public product evidence. | Validate more layouts and structured card-benefit coverage; keep unknown fields unknown. |
| P0 | Source-native product rankings | experimental | Supports product-type and real-category views using separate Coupang ranking, sales, latest, and price controls while labeling local rating/review sorts honestly. | Add source-native category-label discovery so callers do not need to know a numeric category ID; validate selected-option evidence across layouts. |
| P0 | Transparent affiliate deep links | experimental | Adds an optional operator-owned Partners URL without replacing the canonical product URL, while exposing the definite commission disclosure, price notice, self-purchase exclusion, and opt-out state. | The repository channel and disclosure evidence are registered; await final approval, issue the official API keys, and run a credential-redacted live contract check. |
| P1 | Explicit cart add | experimental | Completes the useful shopping loop without crossing into purchase or payment. | Run a separately authorized live mutation test; require exact vendor item plus confirmation and never auto-retry an unverified attempt. |
| P1 | Batch receipts | experimental | Enables accounting, reimbursement, and archive workflows. | Cash/card status, history, summaries, and private completed-archive download are implemented; validate a completed download live and add vendor-receipt reads without implementing request creation. |
| P1 | Payment method and installments | experimental | Answers which payment methods funded observed receipt totals without confusing registered cards with usage. | Payment-method counts and amounts are implemented; capture a redacted sales-slip detail shape and keep installment status `unavailable` until an explicit installment-month field is observed. |
| P1 | Product category enrichment | experimental | Enables source-native category totals without guessing from product names. | The first bounded live backfill completed; validate path stability over time and across more accounts while keeping coverage visible in every category chart. |
| P2 | Price history and repurchase | planned | Helps decide when and what to buy again. | Build after category enrichment; never automate final purchase or payment. |

## Current product boundaries

- Private endpoint details are unstable and remain isolated in Coupang-specific adapters.
- Read operations may use a real installed Chrome session. Public product reads
  are headless-first and may use narrowly isolated DOM fallbacks when no
  structured search document exists. No stealth or anti-detection bypass is
  part of the product.
- Cart addition is the only supported commerce mutation. It is reversible,
  non-idempotent, explicitly confirmed, and separated from all purchase and
  payment controls.
- Receipt status, history, and summaries are structured read operations. A
  completed archive can be downloaded only to a new private `0600` file; its
  source URL is never returned or logged. Receipt generation/request submission
  is a write operation and is intentionally not implemented.
- Category inference from product names is not used. The experimental source
  is the variable-length product-page breadcrumb path; no breadcrumb position
  is relabeled as a fixed Coupang “large/middle/small” field.
- Product-type ranking uses the caller's search query; category ranking uses an
  observed numeric category ID. `sales` preserves Coupang's 판매량순 ordering
  but does not claim an absolute sales count, which the observed page does not
  expose. Page-wide review totals are not attributed to a specific option.
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
