# coupangctl

Consumer-focused Coupang CLI and natural-language MCP shopping data layer.

The project aims to expose a user's own Coupang shopping data to local tools and AI agents without relying on DOM-driven browser automation for routine operations.

## Support via Coupang Partners

[쿠팡 홈 열기](https://link.coupang.com/a/gIEGRL0z7c)

이 링크를 통해 구매하면 쿠팡 파트너스 활동의 일환으로 일정액의 수수료를
제공받습니다. 제휴 링크 자체로 구매자에게 별도 수수료가 부과되지는 않으며,
상품 가격과 혜택은 쿠팡의 최종 화면에서 확인해야 합니다. 프로젝트 운영자의
본인 구매는 수익 인정 대상이 아닙니다.

## Status

The first usable Go product slice is implemented: native browser login with an
isolated profile, headless-first read-only verification and order sync, a
normalized SQLite ledger, local analytics and export/import commands, and an
MCP stdio adapter over the same typed services. A headed read-only fallback is
available for environments where the protected page rejects headless access.
The analytics surface includes cancellation and return rates, purchase
hour/weekday/month distributions, basket and repeat-purchase behavior, top
observed brands, and shipment-duration trends with average, median, and p90
values. A deterministic four-axis shopping type and achievements can be
rendered as a public-safe standalone HTML recap. The v4 recap shows each
axis's denominator and threshold, yearly delivery bars, 24-hour and zero-filled
monthly purchase charts, repeat and basket evidence, and a 16-character
embedded vector roster. Experimental category enrichment reads Coupang's own
product-page breadcrumb JSON-LD, preserves the
complete category path, and reports coverage instead of guessing categories
from product names.

Natural-language product discovery is now an experimental product slice. An AI
can translate “후기 좋은 10만 원 아래 맥북 허브, 광고 제외” into the typed
`products_search` request, inspect a selected candidate's public images,
description, current price, delivery, observed coupon/card benefits, rating,
and sanitized reviews with `product_inspect`, then add the exact vendor item to
the cart only after an explicit confirmation. Missing benefits remain missing;
the adapter reports field coverage instead of inventing them. Checkout,
ordering, and payment remain outside the product boundary.

Search keeps Coupang's source-native rankings distinct. `coupang_ranking`
reproduces “쿠팡 랭킹순”, `sales` reproduces “판매량순”, and `latest` and
price orders use their corresponding source controls. A query gives a
product-type ranking; a real numeric `category_id` gives a category ranking.
Rating and review-count orders are local sorts over observed card fields and
are never labeled as sales. Each response states its scope and provenance.
Repeated options from the same product page are collapsed by default because
their review total may be page-wide; callers can explicitly request every
observed variant.

When official Coupang Partners credentials are configured, product search and
inspection keep the canonical Coupang URL and add a separate `affiliate_url`.
The typed response distinguishes `applied`, `partial`, `unavailable`,
`unconfigured`, `not_applicable`, and explicitly `disabled` states. It also carries a definite
commission disclosure and a reminder to verify the current price and promotion
on Coupang. The link itself does not add a separate affiliate fee to the buyer,
but it is not a promise that every entry path has the same promotion. The
operator's own purchases are marked ineligible.

Private local product insights are a separate response surface. They identify
products by `vendor_item_id`, falling back to `product_id`, and expose leaders
by retained units, distinct orders, and recorded paid amount, plus paid-unit
extremes and the product composition of the highest and lowest positive-spend
days. Product names and exact dates never enter the default shareable insights
or recap.

Authenticated account benefits are also available as an experimental typed
surface. It reports current WOW membership state and fee, Coupang-reported
benefit usage, registered payment-method brands, expected WOW Card rewards,
and monthly reward transactions. It does not infer historical membership fees,
charged card annual fees, the payment method used for each order, or
lump-sum/installment usage when those source fields have not been observed.

`auth status` deliberately distinguishes profile presence from a verified
authenticated session. It never treats stored browser state as proof that the
session is still valid.

TypeScript is retained only for protocol research and is not a runtime
dependency of the distributed product.

## Build and run

Go 1.26 or newer is required for development.

```bash
go build ./cmd/coupangctl
./coupangctl version
./coupangctl capabilities
./coupangctl doctor
./coupangctl auth login                    # QR is the default
./coupangctl auth login --link             # one ephemeral phone app link + approval number
./coupangctl auth login --phone            # manual SMS fallback
./coupangctl auth login --qr-output /secure/path/login-qr.png
./coupangctl auth resend
./coupangctl auth verify
./coupangctl auth status
./coupangctl account benefits --cash-pages 50
./coupangctl orders sync
./coupangctl orders list --limit 20
./coupangctl orders spend --from 2026-01-01
./coupangctl orders stats --from 2026-01-01
./coupangctl orders insights
./coupangctl orders products
./coupangctl orders categories --max-products 25
./coupangctl orders recap --output /secure/path/shopping-recap.html
./coupangctl orders recap --include-products --output /secure/path/private-shopping-recap.html
./coupangctl orders reorder --limit 20
./coupangctl orders export
./coupangctl products search --query '후기 좋은 10만원 아래 맥북 허브' --max-price 100000 --min-rating 4.5 --exclude-sponsored
./coupangctl products search --query '게이밍 데스크탑 16GB 512GB' --min-memory-gb 16 --min-storage-gb 512 --exclude-used --sort sales
./coupangctl products search --category-id ID --sort coupang_ranking
./coupangctl products search --category-id ID --sort sales
./coupangctl products inspect --product-id ID --item-id ID --vendor-item-id ID
./coupangctl products inspect --product-id ID --no-affiliate
./coupangctl products cart-add --product-id ID --vendor-item-id ID --quantity 1 --confirm-add-to-cart
./coupangctl mcp
```

Commands emit documented JSON objects. MCP uses local stdio and currently
provides `auth_status`, `account_benefits`, local order queries, `orders_insights`, private
`orders_product_insights`, normalized export, order sync, `products_search`,
`product_inspect`, and explicitly confirmed `cart_add`. `orders recap`
is a CLI presentation adapter over the same typed insights. It creates a new
`0600` HTML file and never overwrites an existing target.

`orders products` is marked `private_local` because it contains real product
names and exact purchase dates. Its product spend uses only identified item
lines with zero canceled and returned quantity. Retained-unit counts subtract
observed canceled and returned quantities. The response reports both product
ID coverage and spend-eligible item-line coverage. Daily headline spend uses
the non-fully-canceled order total, so it can differ from the visible sum of
item paid amounts because of fees, discounts, missing product identity, and the
five-product display limit.

`orders spend` preserves its gross ledger totals for compatibility and adds a
`commerce` breakdown: `product_purchases`, explicit `membership_fees`, and
`unclassified`. Product behavior, streak, time, delivery, basket, repeat,
brand, and private product-spend metrics exclude explicit membership lines.
An unclassified legacy order remains visible instead of being silently called
a product purchase.

`account benefits` is `private_local`. It may show the user's requested card
brand/type, dates, and aggregates, while account identifiers and raw
transaction descriptions are discarded. Its `order_payments` field currently
returns `status: "unavailable"`: registered cards are not evidence of actual
order funding, and no adopted source yet exposes installment months. The
credit-card sales-slip surface is being researched; only an explicit field
will enable lump-sum/installment statistics.

The default recap remains `public_safe`. `--include-products` is an explicit
privacy opt-in and changes the result visibility to `private_products`. That
HTML contains product names, amounts, and exact dates even while the UI blurs
them before a click, so the file itself must not be shared. Its share button
still copies only the public-safe type text.
Browser login opens an installed Chrome-family browser headed with a dedicated
`coupangctl` profile. QR login is the default and starts from the protected
order URL so the login return context is preserved; it selects the QR tab and
returns `verified` only after the protected order page loads. `--phone` keeps
the manual SMS/CAPTCHA fallback; the CLI never includes those values in JSON,
logs, or the persisted session.
On macOS, `auth resend`
can press an already-visible send/resend control through the operating system's
accessibility API. It reports that the control was requested, not that an SMS
was delivered.

`--qr-output PATH` is intended for a headed renderer that is not directly
visible, such as Chrome running under Xvfb on a Linux server. It automatically
opens the QR tab and writes an enlarged crop of only the QR region to a new
`0600` PNG. The file
exists only while the command waits for phone approval and is removed on
success, expiry, timeout, or cancellation. The path is never included in JSON
output. The caller must provide an unused path in an already-secure directory.
Current live testing found that true Chrome headless mode receives HTTP 403 at
the protected entry page, so `--qr-output` deliberately uses a headed renderer
and does not claim a headless-login bypass.

`--link` is an explicit alternative presentation adapter modeled after the
simple phone handoff used by native investment CLIs. Chrome decodes the visible
QR locally, validates an allowlisted Coupang login URL and its two-digit
approval number, and writes them once to stderr while the same headed browser
waits for approval. The one-time URL never enters JSON, a file created by
`coupangctl`, the session store, fixtures, or an error message. Because stderr
can be redirected by the caller, do not send `--link` output to logs. Treat it
as short-lived authentication material and do not share it. This does not
remove the human approval step.

After successful login, `coupangctl` captures only Coupang session cookies into
a private, atomic `0600` `session.json`. Subsequent verification and sync inject
that state into a short-lived Chrome process, refresh it after successful
authenticated reads, and never emit it. Routine pagination uses the structured
`/ssr/api/myorders/model` JSON route used by the order UI rather than rendered
order cards. If a headless protected read is denied on an interactive machine,
the adapter can retry that read once in headed Chrome; `--headed` remains an
explicit override.

`orders sync --max-pages` is a per-run work budget, not a history cutoff. The
cursor is checkpointed after every page, so another invocation continues where
the previous one stopped. The 1000-page validation ceiling limits one process
run and does not discard older pages.

`orders categories` is a bounded, resumable enrichment pass over products
already present in the local ledger. The source does not expose fixed fields
named “large/middle/small category”; it exposes a variable-length
`BreadcrumbList` of real `/np/categories/<id>` links. The cache therefore keeps
each source position, ID, and label. Aggregate insights currently group by the
most-specific breadcrumb node and identify that rule as `breadcrumb_leaf`.
Unknown products stay unknown and are included in the reported coverage
denominator.

State locations:

- macOS: `~/Library/Application Support/coupangctl`
- Linux: `$XDG_STATE_HOME/coupangctl` or `~/.local/state/coupangctl`
- Windows: `%LOCALAPPDATA%\\coupangctl`

For isolated testing, set `COUPANGCTL_STATE_DIR` to an absolute directory. Set
`COUPANGCTL_BROWSER_PATH` only when browser auto-discovery is insufficient.

## Architecture

```text
cmd/coupangctl
  -> CLI adapter -----\
                       -> typed services -> native browser adapter
  -> MCP stdio adapter/                 -> SQLite repository
```

Private or reverse-engineered response formats stay behind narrow adapters.
CLI and MCP do not own separate browser implementations, and production code
does not import Playwright, Orca, or an agent-specific runtime.

Unlike tools that replay copied cookies through a generic HTTP client,
`coupangctl` restores its private session only into a real installed browser and
performs authenticated requests in that same-origin browser context. The
session store and reverse-engineered routes stay behind narrow adapters, so an
official API can replace them without changing the typed core, SQLite
repository, CLI, or MCP tools.

Public product search and inspection are headless-first and do not require an
authenticated profile for some sampled products, but Coupang can reject a
particular headless layout. On an interactive machine the read adapter can
retry once in a headed installed browser; this is an availability fallback,
not a stealth bypass. Search currently uses a bounded, narrow server-rendered
card adapter because no dedicated search JSON response was observed. Product
inspection prefers JSON-LD and the page's structured read endpoints, with
bounded DOM fallbacks for selected-option evidence, detail images, and benefit
text.
Reviewer identity fields are never extracted, obvious phone/email patterns in
review content are redacted, and only `coupangcdn.com` image URLs are returned.

`cart_add` is the sole supported commerce mutation. It requires the exact
`vendor_item_id` returned by search plus `confirmed=true` (or the CLI's
`--confirm-add-to-cart`). If the button press cannot be verified, the result is
`attempted=true, verified=false` and tells agents not to retry automatically.
It never navigates a purchase or payment control.

## Credential setup

Development credentials live in Doppler and must never be committed:

```bash
doppler run -p cli-mcp-lab -c dev_coupang -- <command>
```

The affiliate adapter reads only these names:
`COUPANG_PARTNERS_ACCESS_KEY`, `COUPANG_PARTNERS_SECRET_KEY`, and optional
`COUPANG_PARTNERS_SUB_ID`. Set `COUPANGCTL_AFFILIATE_DISABLED=true` for a
process-wide opt-out, or pass `--no-affiliate` / `disable_affiliate=true` per
request. Never put key values in the repository, command examples, fixtures,
logs, or structured output.

See [ROADMAP.md](ROADMAP.md) for priorities and implementation status, and
[HANDOFF.md](HANDOFF.md) for validated findings and architecture.
The four axes, thresholds, and all sixteen character mappings are documented in
[TYPE_SYSTEM.md](TYPE_SYSTEM.md).
The redacted private-route inventory is in
[`research/endpoint-catalog.md`](research/endpoint-catalog.md).
Executable feasibility probes are preserved under [`research/probes`](research/probes); see [`research/README.md`](research/README.md) for setup and limitations.

## Safety boundary

- Read-only research and personal-data export first; reversible cart addition requires explicit confirmation.
- Never automate final payment or purchase confirmation.
- Do not log cookies, passwords, OTPs, addresses, order IDs, or raw order payloads.
- Treat private web endpoints as unstable and subject to Coupang's terms and technical controls.
