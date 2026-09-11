# Redacted endpoint catalog

Account-read live recheck: 2026-09-11 (Asia/Seoul). Other entries retain their
dated evidence below; this recheck does not validate every endpoint in the catalog.

This catalog records endpoint contracts without credentials, cookies, OTPs,
identifiers, query values, raw response bodies, or customer fixtures. A path
being listed as `researched` does not make it a supported product API.

| Priority | Method and path | Query names | Response contract (redacted) | Auth | Operation | Adoption |
| --- | --- | --- | --- | --- | --- | --- |
| P0 | `GET https://mc.coupang.com/ssr/desktop/order/list` | optional `pageIndex`, `periodYear` | Next.js document containing `domains.desktopOrder` | required | read | adopted as authenticated bootstrap only |
| P0 | `GET https://mc.coupang.com/ssr/api/myorders/model` | `requestYear`, `pageIndex`, `size` | JSON model with `orderList`, cursor fields, and active-order state | required | read | adopted behind the order-document adapter |
| P0 | `GET https://mc.coupang.com/ssr/api/member/auth` | none | JSON boolean: authenticated session, not account subject or resource-readiness proof | included session when present | read | adopted behind the shared authentication poller for CLI/MCP and explicit login completion |
| P1 | `GET https://mc.coupang.com/ssr/api/member/info` | static client optionally supplies `enableNoStore` | member labels/status and benefit objects; no stable account subject observed | required | read | researched with redacted shape only; not adopted for identity or normal authentication checks |
| P1 | `GET https://mc.coupang.com/ssr/api/member/privacy` | static client optionally supplies `enableNoStore` | object with `name:string`, `email:string`, `phoneNumber:string`, `isDefaultPassword:boolean`; no stable account subject observed | required | read | researched with field names/types only; not adopted or persisted |
| P1 | `GET https://mc.coupang.com/ssr/api/my-account` | none in the observed client call | `member`, `additionalInfo`, `fintechCampaign`, `overrideABTest` objects; no stable account subject observed | required | read | researched with field names/types only; not adopted for identity, spending or preferences |
| P1 | `GET https://mc.coupang.com/ssr/desktop/account-pc` | none in the observed navigation anchor | Next.js bootstrap with `domains.member` and `domains.account`; bounded traversal found no candidate subject field | required | read | researched from the current order page's own link; not adopted for account binding |
| P1 | `GET https://mc.coupang.com/ssr/api/pc/templates/dashboard/data/revamp` | none in the observed client call | object with numeric `coupangCashBalance` and `coupayMoneyBalance`; no account-subject field in the checked response | included session | read | researched from `fetchMyDashboardData`; neither identity nor balance semantics adopted |
| P0 | `GET https://loyalty.coupang.com/loyalty/management/home` | none observed | Next.js data containing membership state, current fee/period, registered payment-method summaries, and Coupang-reported WOW benefit usage | required | read | experimental behind the account-benefits adapter |
| P0 | `GET https://cash.coupang.com/coupang-cash/home` | client sends none; page may add opaque `cst` | authenticated document for fixed same-origin cash reads | required | read | experimental Camofox bootstrap; source-added state is not forwarded or returned |
| P0 | `GET https://cash.coupang.com/api/cash/expected-cash-accumulation` | none | `content.expectedWowCardAccumulationAmount` has integer `amount` and currency; optional current/next-month objects have nested money and earning date | required | read | experimental fixed GET behind the account-benefits adapter |
| P0 | `GET https://cash.coupang.com/api/cash/transactions` | `page` | `content.currentPageNumber:number`, `nextPageExist:boolean`, `list:array`; selected reward rows carry money and creation date | required | read | experimental fixed GET with sequential page validation; raw labels are classified and discarded in-page |
| P1 | `GET https://payment.coupang.com/rocketpay/mypage` | none observed | registered RocketPay method management surface; does not prove per-order usage | required | read | researched; not adopted |
| P1 | `GET https://mc.coupang.com/ssr/desktop/payment-receipt` | none observed | Next.js document containing `paymentReceipt.cash`, `creditCard`, `vendor`, and `form` | required | read | adopted as authenticated receipt bootstrap |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-status` | none observed | `{success:boolean,message:string,data:boolean}`; static reducer maps data true to `POSSIBLE` and false to `IMPOSSIBLE` | required | read | experimental behind the receipt adapter; does not claim why a request is impossible |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/download-request-histories` | `pageIndex`, `size` | paged request rows with date range, count, amount, status, and a bounded download list | required | read | experimental; URLs are consumed only in browser memory and discarded |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/receipt-summary` | `from`, `to`; card reads also use `cardId`, `cardNumber`, `displayCardName` | date range, total count/amount, and card-list shape | required | read | experimental; identifiers/numbers are discarded before typed output |
| P1 | `GET https://mc.coupang.com/ssr/api/payment-receipt/vendor-receipts/<orderId>` | none | array of vendor payment type, issued/product/delivery totals, payment and cancellation components, and product lines | required | read | adopted; raw order ID is resolved from a hashed `source_ref` in browser memory and never returned |
| P1 | completed receipt artifact URL from one history row | none added by the client | bounded PDF, HTML, PNG, JPEG, or octet-stream document | required | read | experimental CLI-only save to a new private file; source/final hosts are allowlisted |
| P1 | `POST https://mc.coupang.com/ssr/api/payment-receipt/{cash,card}/request-download-receipt` | request body contains a period and card selection where applicable | creates an asynchronous receipt archive request | required | external write | known but intentionally excluded from the product |
| P1 | `GET https://www.coupang.com/vp/products/<id>` | optional `vendorItemId` | HTML with JSON-LD `BreadcrumbList`; category nodes contain `position:number`, `name:string`, and `item:https://www.coupang.com/np/categories/<id>` | session restored when available | read | experimental behind the product-category adapter |
| P0 | `GET https://www.coupang.com/np/search` or `/np/categories/<id>` | search `q`; optional `sorter`, `page`; sidebar adds source-owned filter parameters | server-rendered bounded product lists with public identity links, name, price and source position when observed; optional fields remain unavailable when not read | not required for some sampled public products | read | experimental behind the shared Camofox product-search document adapter |
| P0 | `GET https://www.coupang.com/vp/products/<id>` | optional `itemId`, `vendorItemId` | HTML with product JSON-LD plus bounded public description, specification, gallery, and detail-image fallbacks | not required for sampled public products | read | experimental behind the product-inspection adapter |
| P0 | `GET https://www.coupang.com/next-api/products/quantity-info` | `productId`, `vendorItemId` | historically observed array item with price/price-list, shipping, delivery, subscription, cashback, coupon, and discount key families | optional; a 2026-09-03 headed sample returned `403 text/html` while the product page remained available | read | optional narrow read; unavailable does not fail inspection |
| P0 | `GET https://www.coupang.com/next-api/review` | product and bounded pagination/filter names | `rData` with bounded paging contents, review total, and rating-summary fields | not required for sampled public products | read | experimental; author identity is discarded and content PII is redacted |
| P1 | Product-page cart control | exact public product/item/vendor-item identity and bounded quantity | verified add result or explicit attempted-but-unverified result; no raw cart response retained | profile session when available | reversible write | implemented with synthetic contracts; no live mutation executed during verification |

## Evidence and contract rules

- A subsequent 2026-09-11 check isolated six public chunks referenced by the
  account-pc document but not the order document (596,161 source bytes, fetched
  without credentials). A focused second pass found only the existing
  member reads, a cookie-derived feature-flag subject, and `fetchMyDashboardData`
  at `/pc/templates/dashboard/data/revamp`. One included-session GET through
  the existing `/ssr/api` prefix returned HTTP 200 with only the two numeric
  balance keys cataloged above (three traversed nodes). Values stayed in page
  memory; no balance, cookie or feature-flag value was promoted to account
  identity. The legacy DB content hash was unchanged after each check. This
  exhausts these particular client-code leads, not every possible source.
- A further 2026-09-11 investigation followed the current order page's exact
  same-origin `/ssr/desktop/account-pc` anchor with one included-session GET.
  HTTP 200 and 477 traversed bootstrap nodes yielded member/account state but
  no exact `memberSrl`, `memberId`, `memberNo`, `customerId`, `accountId`,
  `userId`, `subject`, or `sub` field. The traversal was bounded to depth 10,
  5,000 nodes and two elements per array; it does not prove absence elsewhere.
  Only paths, types and presence metadata left page memory. The HTML/bootstrap
  was not saved, evaluated as a second page, or used to bind/import a ledger.
  A separate bounded inspection of 20 currently loaded public script URLs
  retrieved 914,477 source bytes with credentials omitted. The JavaScript
  logging client copied the `member_srl` cookie into telemetry `memberSrl`;
  neither cookie values nor telemetry identities were adopted as account proof.
  [Public-source identity research](coupang-account-identity-sources.md)
  separately documents that privacy-notice terminology and seller API identity
  do not establish the missing retail read-response contract. Automatic
  account-scoped synchronization remains unverified; single-page preview is
  not a replacement for this gate.
- A bounded 2026-09-11 identity probe inspected the order/search bootstrap,
  membership and cash documents, and one current-year order-model page without
  exporting values. No populated account-subject field was found within those
  traversal limits. Membership `loyaltyMemberInfo.memberId` and
  `loyaltyMemberInfo.loyaltyMemberId` were null; a numeric
  `paymentProperty.loyaltyMemberId` was not adopted as a source-wide account
  identity. This does not prove that no identity contract exists elsewhere.
- The order model returned a boolean `partial` flag, observed as false on repeated
  fixed first-page reads. The currently loaded public client retains it separately
  from pagination in its order entity and exposes `selectOMSPartial`; this is not
  the item-level `partialOOS` field. The document and Camofox page parsers now reject
  an explicit true flag with `partial_order_data`; malformed flags remain invalid
  data. Older shapes without the flag retain their prior parser support, not a
  new completeness guarantee. Synthetic bootstrap/continuation and SQLite sync
  tests reproduce the former acceptance, then verify no partial-page commit or
  cursor advance and resumption from the rejected page. CLI/MCP retain the typed
  error without raw causes. A live post-change Camofox reader check returned five
  validated orders on each of two consecutive pages, both with continuation.
  Live partial=true handling was not observed; that branch is synthetic evidence.
  No real order sync, DB binding or account-completeness claim was made.
- A further 2026-09-11 account-identity check rediscovered
  `fetchMemberPrivacy` and its GET path in a currently loaded public chunk.
  The fixed same-origin read returned HTTP 200 with the four fields listed
  above. Values were discarded in the page; names, email and phone numbers
  were not used as identity substitutes. This closes one candidate contract,
  not the stable-account-identity gate, and does not justify binding a legacy DB.
- A subsequent check traced the loaded public client's `fetchMyAccount` through
  its shared GET adapter to `/ssr/api/my-account`. Two bounded same-session reads
  returned HTTP 200 at that exact URL. Key/type inspection found membership
  labels, benefit objects and additional marketing fields, but no stable account
  subject. Fields named `paymentAmountForOneMonth`, `frequentlyBoughtCategoryName`
  and `lastOrderTotalPrice` were not adopted: their names do not establish period,
  exclusions or preference semantics. Values and raw responses stayed in page
  memory; no history sync, account binding or profile change was performed.
- On 2026-09-11 the protected order bootstrap reported
  `props.pageProps.context.isLogin:boolean` while `domains.member` was empty.
  Public page chunks exposed `GET /member/auth` and `GET /member/info`; the page's
  own member-name request established the `/ssr/api/member/` client prefix.
  Two included-session auth reads returned HTTP 200 JSON `true`. One read that
  omitted credentials returned HTTP 200 JSON `false` without clearing cookies
  or logging the user out. This differential establishes authentication only.
  The info response's key/type inspection found no stable account subject;
  membership-benefit IDs, names, profile paths and cookies were not adopted as
  substitutes. Existing databases remain unbound to a verified source account.
- Authentication now uses that fixed bounded GET independently of order parsing.
  Missing/non-boolean/oversized replies remain unknown; 401 and source `false`
  require authentication, whereas 403/429 remain access failures. Redirects and
  changed documents cannot verify the session. CLI/MCP responses expose
  `verification_scope=authenticated_session`, not order/history coverage.
  The development CLI and actual MCP stdio `auth_status` were live-checked after
  the change. MCP `auth_login_if_needed` with `confirmed=false` reused the
  authenticated session and returned `visible_browser_opened=false`. Fresh
  manual/mobile authentication and cross-account identity isolation remain
  separate gates; no order synchronization or database reassignment was done.
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
- The 2026-09-11 dedicated headless Camofox recheck observed both fixed cash
  GET paths twice with HTTP 200 and matching key/type shapes. Source-added `cst`
  on the bootstrap page caused the former exact-URL check to reject an otherwise
  valid read. Only one nonempty bounded `cst` value on the exact cash origin/path
  is now tolerated; its meaning is not inferred and it is never forwarded.
  Other query names, duplicates, fragments, and credential-bearing URLs fail.
  Production CLI (two cash pages) and MCP (one page) then returned typed schema-v5
  snapshots with membership-fee, benefit-total, and expected-reward coverage.
  Both reported partial cash history. These are repeated reads of one consenting
  session, not cross-account coverage or evidence of historical fees paid.
- Current account-benefit adoption keeps only normalized membership fields,
  benefit aggregates, payment-method brand/type/issuer, and monthly WOW Card
  reward aggregates. Payment account identifiers and raw cash transaction text
  are discarded inside the browser page, before transport to the Go adapter.
  The in-page WOW Card label match is a derived classification, not a source
  enum. Missing classified-row money, currency, or date invalidates the read;
  rows are not silently skipped into a supposedly complete sum.
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
- Live query-search sort controls exposed `scoreDesc` (쿠팡 랭킹순), `saleCountDesc`
  (판매량순), `latestAsc`, `salePriceAsc`, and `salePriceDesc`. A 2026-09-11
  Camofox category recheck supersedes the earlier assumption that query and
  category default sort tokens are identical: category filtering adds
  `sorter=bestAsc`, while the selected control remains 쿠팡 랭킹순 before and
  after the click. Two metadata samples observed that transition. The adapter
  treats absent category sorter as `bestAsc`, absent query sorter as `scoreDesc`,
  and binds structured lists to category path, sort, page and filter state.
  A category ID from a product's native BreadcrumbList returned three public
  products and twenty sidebar groups. CLI and MCP then each returned three
  products with an observed memory filter selected and verified budget-eligible
  prices. The initial category-filter failure was reproduced and corrected;
  these reads do not verify every category or every sort. 쿠팡 랭킹순 explicitly
  combines multiple signals, while 판매량순 is a separate ordering and does
  not expose absolute sales units. Local rating and review-count sorts are
  labeled as local observed-field sorts.
- A separate category-navigation check exercised the observed `브랜드PC` sidebar
  choice from category `497139` (`조립PC`) to `497137`. The source breadcrumb uses
  a one-level nested `itemListElement`; the adapter accepts that bounded observed
  shape as well as flat lists, and checks ordered positions, the selected label
  and the exact destination category URL. A URL change alone cannot authorize
  the new scope. Query, sorter and page bindings remain checked.
  Actual MCP `products_recommend` retained the starting category and reported the
  verified destination plus both catalog scopes. Its status was `needs_input`,
  not a completed recommendation. Passing that response to
  `products_report_render` produced HTML showing both scopes without another
  source read. Full follow-on research after this category switch is covered by
  synthetic tests, not by this live refinement/report check. Transitions that
  reset the requested sort remain unsupported.
- Category-only recommendations now accept an ordered path of explicit
  `facet:카테고리` answers within the six-choice budget. A source-backed current
  category replaces its predecessor only after successful verification; earlier
  catalogs stay in `refinement.steps`, while other active filters are preserved.
  The development CLI followed `497139` → `497136` (`데스크탑`) → `497137`
  (`브랜드PC`), retained both navigation steps and returned 17 search candidates
  in `needs_input` status. Actual MCP `products_recommend` followed the same two-hop
  path, and its response rendered with both starting and applied category IDs via
  `products_report_render`. Synthetic tests cover continuation through all research
  sorts and detail, preserving other filters, missing/denied children, and keeping
  the last successful scope. Search inputs remain unique-group active selections;
  query-based navigation was still unsupported at that checkpoint.
- Query-based recommendations now replay previously verified category labels
  through `category_trail` before the final active `카테고리` selection. Prior
  choices are navigation history, not simultaneous constraints. The query,
  sort and page remain bound; there is no transition to a category-only URL.
  Live development CLI discovery for `데스크탑` exposed `가전디지털`; selecting
  it exposed `데스크탑`. A subsequent two-step recommendation replayed both,
  verified the final selected label and returned 12 public search candidates
  with `needs_input`, while exposing `브랜드PC`, `조립PC`, `일체형PC`, and
  `미니PC` as further choices. These are list observations, not detail-verified
  recommendations. Synthetic tests cover trail replay, prior choices becoming
  unselected, missing children, lost active filters, denial, and reservation
  accounting across refinement and all research sorts. Trail plus active
  selections is bounded to six navigation steps; each is verified before the
  next click. A failed step retains only the last verified state.
  Actual MCP `products_recommend` then followed the three-choice query path
  `가전디지털` → `데스크탑` → `브랜드PC`, returned 17 candidates with
  `needs_input`, and retained the original query, two prior trail labels,
  only the final active category and all four catalog observations. Report
  rendering distinguishes prior trail labels from active filters; this report
  behavior was verified with synthetic evidence, not a live rendered page.
  A separate live CLI search replayed that trail with `메모리용량=32GB 이상`,
  a KRW 2,000,000 listing-price ceiling and `price_asc`, returning three public
  cards with the requested sidebar choices selected. The returned option names
  included lower-memory configurations. This verifies navigation and source
  selection state, not per-option memory suitability; neither category nor
  attribute-filter membership can replace exact-option detail evidence. Search
  warnings now state this distinction. Recommendation hard-condition support
  for exact memory/storage specifications remains unimplemented.
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
- 2026-09-11 KST: four public desktop detail samples exposed inline native
  `options.optionRows[]` with `name:string`, `selectedAttribute` and
  `attributes[]` (`valueId:string`, `name:string`, `selected:boolean`).
  Joining selected value IDs in row order with `:` indexed
  `options.attributeVendorItemMap[tuple]`; its numeric `itemId` and
  `vendorItemId` matched the independently identified selected option in all
  four samples. Compound value IDs retain their source commas. No account
  identifiers, raw scripts or real fixtures were retained.
  The shared reader adopts only the bounded label/value rows, checks strict
  selected flags and agreement with the selected list entry, and requires
  both map IDs to match. It decodes bounded JSON literals without evaluating
  source scripts or requesting another endpoint. Changed rows during auxiliary
  reads invalidate the inspection. Missing or ambiguous tuples remain unavailable.
  The typed parser and service preserve this as `selected_attributes` with
  observed `product_options` evidence bound to the exact option. It also works
  when no dropdown-selected DOM node exists. This is source-stated option
  information, not independent hardware verification.
  Two samples exposed compound capacity rows with suspicious label/value order
  or unequal label/value counts; two others had aligned-looking compound rows.
  Therefore the adapter preserves compound rows intact rather than asserting
  positional RAM/storage semantics. Title/option-text `computer_specs` and
  memory/storage discovery prefilters are now explicitly labeled heuristic.
  Real production CLI and MCP inspections returned the bound native rows and
  separate inferred specs; the MCP sample had no selected dropdown DOM row but
  retained the native tuple and exact identity. Synthetic shared-JS-to-Go, invalid-binding, response round-trip
  and report-escaping tests cover the contract. Numeric memory/storage hard
  conditions remain pending stronger field semantics.
- 2026-09-11 KST follow-up: six public laptop detail reads across two searches
  exposed bound native option tuples. Two independent products had standalone
  `RAM용량` and `저장용량` rows (GB strings for memory; GB/TB strings for storage);
  the other four retained compound capacity rows. In both standalone samples,
  the existing tuple reader verified item/vendor-item identity and observed
  `product_options` provenance. No new endpoint or raw source fixture was added.
  The shared core now accepts integer-GB hard conditions and preference axes
  `specifications.memory_gb` / `specifications.storage_gb` from these standalone
  labels only. Results use `derived_integer` plus raw source name/value and
  `standalone_capacity_decimal_gb` derivation metadata, retaining the original
  exact-option evidence. Decimal TB converts to 1000 GB; binary units, fractional
  GB, unitless values, aliases and compound labels are not guessed. An additional
  compound row mentioning the same specification also leaves it unknown.
  This validates source-stated option semantics, not installed hardware or
  performance. Earlier compound-desktop limitations remain unresolved.
  A live production CLI recommendation with minimum 32 GB memory and 512 GB
  storage inspected five public options: one met both conditions, one failed
  memory, and three remained unknown because of compound labels. The sixth
  attempted detail returned access denied; the run stopped with `incomplete`
  and preserved the five assessments. No further source retry was performed.
  The retained option included derived capacity comparison values and bound
  native evidence. This is not proof of market-wide optimality or whole-run
  completion. A regression also corrected excluded candidates' comparison
  reason: requested axes are not reported as absent merely because a hard
  condition excluded the candidate before comparison.
  Synthetic core/service/CLI/MCP/report tests cover met/unmet/unknown, exact
  identity, provenance, decimal arithmetic, invalid units, and inferred-title
  rejection. An actual MCP process exposed both capacity inputs in its tool
  schema and rendered a synthetic report with the derived values and incomplete
  status. The MCP report check made no source read and was not visual browser QA.
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
