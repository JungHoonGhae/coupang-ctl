# Redacted endpoint catalog

Last live verification: 2026-09-03 (Asia/Seoul).

This catalog records endpoint contracts without credentials, cookies, OTPs,
identifiers, query values, raw response bodies, or customer fixtures. A path
being listed as `researched` does not make it a supported product API.

| Priority | Method and path | Query names | Response contract (redacted) | Auth | Operation | Adoption |
| --- | --- | --- | --- | --- | --- | --- |
| P0 | `GET https://mc.coupang.com/ssr/desktop/order/list` | optional `pageIndex`, `periodYear` | Next.js document containing `domains.desktopOrder` | required | read | adopted as authenticated bootstrap only |
| P0 | `GET https://mc.coupang.com/ssr/api/myorders/model` | `requestYear`, `pageIndex`, `size` | JSON model with `orderList`, cursor fields, and active-order state | required | read | adopted behind the order-document adapter |
| P0 | `GET https://loyalty.coupang.com/loyalty/management/home` | none observed | Next.js data containing membership state, current fee/period, registered payment-method summaries, and Coupang-reported WOW benefit usage | required | read | experimental behind the account-benefits adapter |
| P0 | `GET https://cash.coupang.com/coupang-cash/home` | none observed | authenticated document whose same-origin resources expose expected WOW Card accumulation and paged cash transactions | required | read | experimental bootstrap behind the account-benefits adapter |
| P0 | `GET https://cash.coupang.com/api/cash/...` | endpoint discovered from same-origin resource entries; transaction page uses `page` | expected accumulation shape or paged cash list; transaction descriptions are used only for narrow reward classification and then discarded | required | read | experimental; exact unstable path remains dynamically discovered and allowlisted to same origin |
| P1 | `GET https://payment.coupang.com/rocketpay/mypage` | none observed | registered RocketPay method management surface; does not prove per-order usage | required | read | researched; not adopted |
| P1 | `GET https://mc.coupang.com/ssr/desktop/payment-receipt` | none observed | Next.js document containing `paymentReceipt.cash`, `creditCard`, `vendor`, and `form` | required | read | adopted as authenticated receipt bootstrap |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-status` | none observed | `{success:boolean,message:string,data:boolean}`; static reducer maps data true to `POSSIBLE` and false to `IMPOSSIBLE` | required | read | experimental behind the receipt adapter; does not claim why a request is impossible |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/download-request-histories` | `pageIndex`, `size` | paged request rows with date range, count, amount, status, and a bounded download list | required | read | experimental; URLs are consumed only in browser memory and discarded |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/receipt-summary` | `from`, `to`; card reads also use `cardId`, `cardNumber`, `displayCardName` | date range, total count/amount, and card-list shape | required | read | experimental; identifiers/numbers are discarded before typed output |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/vendor-receipts/<orderId>` | none | array of vendor payment type, issued/product/delivery totals, payment and cancellation components, and product lines | required | read | adopted; raw order ID is resolved from a hashed `source_ref` in browser memory and never returned |
| P1 | completed receipt artifact URL from one history row | none added by the client | bounded PDF, HTML, PNG, JPEG, or octet-stream document | required | read | experimental CLI-only save to a new private file; source/final hosts are allowlisted |
| P1 | `POST https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-download-receipt` | request body contains a period and card selection where applicable | creates an asynchronous receipt archive request | required | external write | known but intentionally excluded from the product |
| P1 | `GET https://www.coupang.com/vp/products/<id>` | optional `vendorItemId` | HTML with JSON-LD `BreadcrumbList`; category nodes contain `position:number`, `name:string`, and `item:https://www.coupang.com/np/categories/<id>` | session restored when available | read | experimental behind the product-category adapter |
| P0 | `GET https://www.coupang.com/np/search` or `/np/categories/<id>` | search `q`; optional `sorter` | server-rendered bounded product cards with public identity links, name, image, current/original price, rating, review count, source position, and delivery/promotion badges when present | not required for some sampled public products | read | experimental behind the product-search document adapter |
| P0 | `GET https://www.coupang.com/vp/products/<id>` | optional `itemId`, `vendorItemId` | HTML with product JSON-LD plus bounded public description, specification, gallery, and detail-image fallbacks | not required for sampled public products | read | experimental behind the product-inspection adapter |
| P0 | `GET https://www.coupang.com/next-api/products/quantity-info` | `productId`, `vendorItemId` | historically observed array item with price/price-list, shipping, delivery, subscription, cashback, coupon, and discount key families | optional; a 2026-09-03 headed sample returned `403 text/html` while the product page remained available | read | optional narrow read; unavailable does not fail inspection |
| P0 | `GET https://www.coupang.com/next-api/review` | product and bounded pagination/filter names | `rData` with bounded paging contents, review total, and rating-summary fields | not required for sampled public products | read | experimental; author identity is discarded and content PII is redacted |
| P1 | Product-page cart control | exact public product/item/vendor-item identity and bounded quantity | verified add result or explicit attempted-but-unverified result; no raw cart response retained | profile session when available | reversible write | implemented with synthetic contracts; no live mutation executed during verification |

## Evidence and contract rules

- The order UI moves within a year using `periodYear`, but its structured model
  request uses `requestYear`. Treating these names as interchangeable caused a
  cursor loop; the production adapter now reproduces the JSON request.
- The model endpoint is fetched from an authenticated same-origin page. Direct
  HTTP replay is not the supported path.
- The response parser accepts integral JSON numbers written either as `25900`
  or `25900.0`, but rejects fractional KRW values.
- A bounded headed order-model metadata sample observed a cancellation bundle
  with explicit canceled quantities, status, and item-price fields. Those
  fields do not prove the settled refund after discounts, points, shipping,
  fees, or later adjustments. Import-tax refund fields were also present but
  null and remain scoped to import-tax over-collection. Exact post-refund net
  spend therefore remains unadopted; see
  [`refund-settlement-evidence.md`](refund-settlement-evidence.md).
- Receipt state exposes separate cash, credit-card, and vendor domains plus
  download-history pagination. Status, cash/card history, and cash/card summary
  GETs are adopted. A completed history artifact is a bounded read; archive
  request creation is an external write and remains excluded. The credit-card
  summary exposes selected card metadata, period, amount, and count but no
  installment-month field, so installments remain explicitly unavailable.
- Static bundle metadata placed `GET` six characters from the vendor-receipt
  path template. Five bounded redacted order samples then returned status 200
  with the same key/type shape. The response exposed payment type and explicit
  cancellation component fields but no installment-month field. The adopted
  command preserves those components as observed values without asserting a
  completed refund settlement.
- A later static-shape pass found installment-named identifiers only in
  cancellation/return-flow state and experiment flags, not in adopted receipt
  result fields. Identifier presence is therefore not installment evidence;
  installments remain unavailable until an explicit transaction field exists.
- Static reducer evidence maps a successful request-status GET response's
  `data=true` to `POSSIBLE` and `data=false` to `IMPOSSIBLE`. The same UI uses
  `IMPOSSIBLE` during request submission, but the GET does not expose a reason.
  Schema v2 therefore reports availability and leaves `request_in_progress`
  null instead of guessing that every impossible state is an active request.
- The live receipt UI exposed only cash/card request-history controls, refresh,
  pagination, period inputs, and summary query controls. A probe installed a
  route-level POST block before opening request history; no creation POST or
  submit control appeared. Request payloads therefore remain unadopted rather
  than being guessed from a route name.
- A live card-summary read echoed an end date different from the requested end
  date. The typed contract therefore preserves the caller's requested ISO range
  and adds a warning whenever a valid source-reported range differs.
- Current account-benefit adoption keeps only normalized membership fields,
  benefit aggregates, payment-method brand/type/issuer, and monthly WOW Card
  reward aggregates. Payment account identifiers and raw cash transaction text
  are discarded before the typed core response.
- A headed metadata-only check observed that the membership page labels the
  displayed savings as a recent-three-month window. The normalized response
  preserves that UI window separately from the amount. A complete live order
  history exposed no membership-specific product/division enum, so order rows
  cannot currently prove historical membership fees. Official Coupang guidance
  directs membership-fee cash receipts to the PC receipt screen; receipt
  evidence remains the required source for exact paid-history adoption.
- The same account-state probe observed a positive, plausible epoch-millisecond
  `loyaltyFeeChangeDate`. Schema v3 exposes only its normalized date as
  `source_fee_change_date`: schedule metadata distinct from historical fee
  amounts, charge dates, and actual payment evidence.
- Three redacted live product samples returned one JSON-LD breadcrumb each,
  with 5, 6, and 5 list items. The source does not name fixed
  large/middle/small fields, so the adapter preserves every category ID, label,
  and source position. Aggregate insights explicitly use the most-specific
  breadcrumb node (`breadcrumb_leaf`) and report missing-product coverage.
- Review and promotion responses also contain fields named `categoryId`, but
  their contracts describe those subdomains and are not treated as the
  product's canonical category path.
- Live headless verification on 2026-09-02 returned bounded search cards with
  public identifiers, current/original prices, rating, and review count. A
  selected product inspection returned public description/specification,
  gallery and detail images, current price, delivery text, rating totals, and
  bounded reviews. The sampled item had no structured card-benefit field, so
  the response exposed `card_benefit` as unavailable rather than inferring one.
- A live public search on 2026-09-03 verified that an explicitly observed
  `price.current_amount` can be stored and read through the local exact-option
  history. This is a local observation ledger over the already adopted search
  surface, not a new endpoint and not retroactive Coupang price history.
- Search was observed as server-rendered; no dedicated search JSON request was
  promoted. Its DOM contract is isolated to the product adapter and backed by
  synthetic parser tests. Detail inspection uses JSON-LD and the narrow read
  endpoints above before bounded DOM fallbacks.
- Live sort controls exposed `scoreDesc` (쿠팡 랭킹순), `saleCountDesc`
  (판매량순), `latestAsc`, `salePriceAsc`, and `salePriceDesc`. Category browse
  at `/np/categories/<id>` accepted the same controls. 쿠팡 랭킹순 explicitly
  combines multiple signals, while 판매량순 is a separate ordering and does
  not expose absolute sales units. Local rating and review-count sorts are
  labeled as local observed-field sorts.
- The product workflow preserves source-native order for ranking, sales,
  latest, and price controls. It does not reinterpret an unobserved price as
  zero, and local rating/review sorting promotes only values whose field is
  explicitly observed.
- A headed metadata-only product-option probe observed that `quantity-info`
  exposes price, delivery, promotion, quantity, and selection-index shapes but
  no option label in the sampled response. A selected-option DOM signal was
  observed on one layout and remains a bounded fallback; when absent, option
  identity stays with the exact search card and vendor-item ID rather than
  being inferred from a page-wide review total.
- A 2026-09-03 metadata-only recheck found `quantity-info` returning HTTP 403
  with `text/html` on a headed installed-Chrome sample while the enclosing
  product page returned 200. No response body was retained. The inspected page
  exposed two non-empty selected-option rows; the production extractor now
  waits for their text instead of treating an empty picker shell as ready.
- Four live product layouts (hub, assembled PC, MacBook, and TV) verified
  structured price, delivery, images, bounded reviews, and honest field
  coverage. No card-benefit text was present in those samples, so generic
  promotion text was not relabeled as a card benefit.
- The typed product parser independently reconciles `selected_options` and
  `card_benefit` coverage after normalization. A missing value is explicitly
  unavailable even if a source document claimed it was observed, while a
  present value removes a contradictory unavailable label.
- A later bounded live pass reached zero pending product references without
  guessing missing categories. Both valid breadcrumb documents and explicit
  unavailable outcomes were observed; account-specific counts and labels were
  not recorded here.
- A 2026-09-03 explicit headless-first recheck retained five additional
  breadcrumb observations for the same exact product identities on a later
  UTC date. All five paths matched their prior observations. This is a bounded
  single-account sample, not evidence of population-wide taxonomy stability;
  the typed stability report keeps that limitation and its denominators
  visible.
- New endpoints require a synthetic contract fixture, strict URL allowlist,
  bounded pagination, safe error mapping, and a live metadata-only verification
  before their adoption status changes.

## Next discovery queue

1. Validate one already-completed receipt artifact with metadata-only output
   and research whether its cash-receipt rows can identify membership fees. Do
   not trigger request creation.
2. Capture a redacted sales-slip detail shape and adopt installments only if an
   explicit installment-month field exists. The bounded multi-page receipt
   sampler is ready; rerun it when the protected order walk is accepted.
3. Broaden the same-product category recheck sample and validate across more
   consenting accounts; preserve changed paths rather than overwriting their
   evidence.
4. Continue sampling exact card-benefit fields when a source-positive product
   is encountered; do not translate generic promotion text into a card claim.
