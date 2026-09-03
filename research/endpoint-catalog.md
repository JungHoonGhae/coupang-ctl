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
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-status` | none observed | `{success:boolean,message:string,data:boolean}` | required | read | experimental behind the receipt adapter |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/download-request-histories` | `pageIndex`, `size` | paged request rows with date range, count, amount, status, and a bounded download list | required | read | experimental; URLs are consumed only in browser memory and discarded |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/receipt-summary` | `from`, `to`; card reads also use `cardId`, `cardNumber`, `displayCardName` | date range, total count/amount, and card-list shape | required | read | experimental; identifiers/numbers are discarded before typed output |
| P1 | completed receipt artifact URL from one history row | none added by the client | bounded PDF, HTML, PNG, JPEG, or octet-stream document | required | read | experimental CLI-only save to a new private file; source/final hosts are allowlisted |
| P1 | `POST https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-download-receipt` | request body contains a period and card selection where applicable | creates an asynchronous receipt archive request | required | external write | known but intentionally excluded from the product |
| P1 | `GET https://www.coupang.com/vp/products/<id>` | optional `vendorItemId` | HTML with JSON-LD `BreadcrumbList`; category nodes contain `position:number`, `name:string`, and `item:https://www.coupang.com/np/categories/<id>` | session restored when available | read | experimental behind the product-category adapter |
| P0 | `GET https://www.coupang.com/np/search` or `/np/categories/<id>` | search `q`; optional `sorter` | server-rendered bounded product cards with public identity links, name, image, current/original price, rating, review count, source position, and delivery/promotion badges when present | not required for some sampled public products | read | experimental behind the product-search document adapter |
| P0 | `GET https://www.coupang.com/vp/products/<id>` | optional `itemId`, `vendorItemId` | HTML with product JSON-LD plus bounded public description, specification, gallery, and detail-image fallbacks | not required for sampled public products | read | experimental behind the product-inspection adapter |
| P0 | `GET https://www.coupang.com/next-api/products/quantity-info` | `productId`, `vendorItemId` | array item with price/price-list, shipping, delivery, subscription, cashback, coupon, and discount key families | not required for sampled public products | read | experimental; normalized inside product inspection |
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
- Receipt state exposes separate cash, credit-card, and vendor domains plus
  download-history pagination. Status, cash/card history, and cash/card summary
  GETs are adopted. A completed history artifact is a bounded read; archive
  request creation is an external write and remains excluded. The credit-card
  summary exposes selected card metadata, period, amount, and count but no
  installment-month field, so installments remain explicitly unavailable.
- A live card-summary read echoed an end date different from the requested end
  date. The typed contract therefore preserves the caller's requested ISO range
  and adds a warning whenever a valid source-reported range differs.
- Current account-benefit adoption keeps only normalized membership fields,
  benefit aggregates, payment-method brand/type/issuer, and monthly WOW Card
  reward aggregates. Payment account identifiers and raw cash transaction text
  are discarded before the typed core response.
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
- A headed metadata-only product-option probe observed that `quantity-info`
  exposes price, delivery, promotion, quantity, and selection-index shapes but
  no option label in the sampled response. A selected-option DOM signal was
  observed on one layout and remains a bounded fallback; when absent, option
  identity stays with the exact search card and vendor-item ID rather than
  being inferred from a page-wide review total.
- A later bounded live pass reached zero pending product references without
  guessing missing categories. Both valid breadcrumb documents and explicit
  unavailable outcomes were observed; account-specific counts and labels were
  not recorded here.
- New endpoints require a synthetic contract fixture, strict URL allowlist,
  bounded pagination, safe error mapping, and a live metadata-only verification
  before their adoption status changes.

## Next discovery queue

1. Validate one already-completed receipt artifact with metadata-only output,
   and research the vendor-receipt read contract. Do not trigger request creation.
2. Capture a redacted sales-slip detail shape and adopt installments only if an
   explicit installment-month field exists.
3. Validate category breadcrumb stability across time and across more accounts.
4. Validate exact structured card-benefit and delivery-badge fields across
   more public product layouts without translating generic promotion text into
   a card claim.
