# Scrapling applicability to `coupangctl`

Validated: 2026-09-04 (Asia/Seoul)

Source snapshot: Scrapling `main` at
[`1f40d9b11b531ea46e9a2f9447f8757e4a71621c`](https://github.com/D4Vinci/Scrapling/tree/1f40d9b11b531ea46e9a2f9447f8757e4a71621c),
version 0.4.15. The source review was followed by a bounded live compatibility
check against the product-owned dedicated profile. The check retained only
status, final host/path, body size, and the presence or absence of expected
structure; it did not retain page bodies, cookies, headers, credentials, or
customer data.

## Decision

Do **not** adopt Scrapling in the production `coupangctl` binary, and do not
ship it as an optional Python sidecar. Standard `DynamicSession` 0.4.15 was
tested with installed Chrome, the persistent product profile, one reusable tab,
and otherwise ordinary documented settings. On 2026-09-04 both headless and
visible runs received HTTP 403 for the public product-search URL and the order
list URL. The existing dedicated CDP headless path received the same result,
while the already-approved ordinary-Chrome path remained the verified working
route. Scrapling therefore did not solve the actual compatibility problem.

It also duplicates the existing dedicated-profile/CDP adapter while adding
Python, Playwright/Patchright, downloaded browser assets, and a second lifecycle
and validation boundary. Its adaptive parser is a useful HTML drift heuristic,
not evidence that a relocated element still has the same commerce meaning.

This is a runtime and packaging decision, not a reason to discard its useful
mechanisms. Adopt the XHR/JSON-first extraction pattern directly in the narrow
Go CDP adapter: observe only allowlisted same-origin requests, validate response
shape and size, reduce the body immediately to typed fields, and never expose
cookies, raw headers, or raw authenticated payloads. Separately, a
development-only fixture tool may compare synthetic or redacted HTML revisions
and report candidate selector drift for human review. It must not become a
release artifact or promote a similarity match to an observed field.

## Live compatibility evidence

The live check used Scrapling's documented `DynamicSession` with
`real_chrome=True`, a persistent `user_data_dir`, `max_pages=1`, and no proxy,
challenge solver, fingerprint-spoofing option, or stealth session. Results were
identical with `headless=True` and `headless=False`:

| Route | Result | Retained structural observation |
| --- | --- | --- |
| Public `/np/search` | HTTP 403 | Small denial document; no JSON-LD product list or canonical product links |
| Authenticated order list | HTTP 403 | Small denial document; no order model |

This result is deliberately about compatibility, not authorization. It does
not justify adding CAPTCHA automation, rotating proxies, or identity/fingerprint
spoofing. A challenge remains a typed human handoff. Repeated Chrome approval is
instead avoided by keeping one approved MCP/current-browser process alive and
reusing its connection for subsequent reads.

## Fit summary

| Area | Scrapling capability | Fit for `coupangctl` |
| --- | --- | --- |
| Session reuse | HTTP sessions share a connection pool and cookies; browser sessions reuse a browser and tabs, and can persist browser state in a supplied user-data directory. ([HTTP sessions](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/static.md#session-management), [browser sessions](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/dynamic.md#session-management)) | Mechanically applicable, but no new product capability. The current Go adapter already reuses one browser session and keeps state in the product-owned Chrome profile. |
| Browser-backed fetching | `DynamicFetcher` uses Playwright with Chromium or installed Chrome, can launch a persistent context, or attach through a CDP URL. ([docs](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/dynamic.md#fetching-dynamic-websites), [implementation](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/scrapling/engines/_browsers/_controllers.py)) | Poor production fit. `coupangctl` already owns a narrower loopback CDP path, URL allowlists, profile locking, bounded reads, and typed failures. |
| Structured extraction | A `Response` can parse raw JSON, parse JSON embedded in selected text, use CSS/XPath/text selectors, and expose captured XHR responses. ([response/parser](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/parsing/main_classes.md), [XHR capture](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/dynamic.md#capturing-xhrfetch-requests)) | Useful extraction primitives, but not a typed Coupang domain contract. Structured JSON still needs the existing narrow DTO/parser/core boundary; DOM selectors remain a fallback, not the preferred source. |
| Parser resilience | Opt-in adaptive mode stores an element fingerprint and later selects the page element with the highest similarity. ([adaptive docs](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/parsing/adaptive.md)) | Suitable only as a diagnostic hint. A similarity match is inferred, can be wrong, and cannot establish field semantics. |
| Packaging | Python 3.10+; the fetcher extra adds `curl_cffi`, Playwright, Patchright, BrowserForge, a fingerprint dataset, and other packages. Browser setup downloads browsers, system dependencies, and fingerprint-manipulation dependencies. ([package metadata](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/pyproject.toml), [installation](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/README.md#installation)) | Conflicts with the one-Go-executable release decision and materially expands checksum, SBOM, attestation, cross-platform, and smoke-test scope. |
| Safety/legal | BSD-3-Clause permits redistribution subject to its terms, but the project itself requires users to comply with scraping/privacy law, site terms, and robots.txt. Spider robots handling is optional and defaults off. ([license](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/LICENSE), [disclaimer](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/README.md#disclaimer), [default](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/spiders/advanced.md#concurrency-control)) | License compatibility is not authorization to access Coupang. All access still needs product/legal review, bounded rates, applicable site-policy compliance, and the existing no-challenge-bypass boundary. |

## Detailed assessment

### Session reuse and browser fetching

Scrapling has two relevant session models:

- `FetcherSession` reuses HTTP configuration, connections, and cookies inside a
  session.
- `DynamicSession`/`AsyncDynamicSession` keep a browser open, reuse tabs, and
  preserve cookies and session state. A supplied `user_data_dir` persists
  cookies and local storage; the default is a temporary directory. A `cdp_url`
  can attach to a debuggable browser, including a remote one.
  ([dynamic arguments and lifecycle](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/dynamic.md#full-list-of-arguments))

Those mechanics overlap [`internal/browser`](../internal/browser), where the Go
adapter already launches installed Chrome against a dedicated profile, binds
the debugging endpoint to loopback, retains session state in that profile, and
reuses the session through narrow document methods. The service consumes a
small `DocumentSource`/`PageSource` interface rather than a general browser API
([`internal/orders/service.go`](../internal/orders/service.go)). Scrapling does
not remove any of those product-specific controls; wrapping it safely would
have to recreate them in Python and validate the result again in Go.

Scrapling's general `Response` also exposes cookies, headers, raw body bytes,
and captured XHR bodies ([response contract](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/fetching/choosing.md#response-object)).
That is convenient for scraping, but is too broad for a credential-adjacent
adapter unless every output is immediately reduced and validated. The existing
Go boundary is safer because cookie values never need to become an application
result.

### Structured extraction and provenance

Scrapling can parse an HTTP JSON body with `.json()` and JSON inside an HTML
element after selecting it. Its main HTML API is CSS, XPath, text, regex, and
similar-element selection ([selection methods](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/parsing/selection.md)).
These are parser conveniences, not a documented Coupang response shape or a
typed domain model.

`coupangctl` already extracts bounded structured documents, validates required
paths/types and amounts, and converts them into typed core values
([order parser](../internal/coupang/orders/parser.go),
[typed core](../internal/core/orders.go)). That boundary preserves the
observed/derived/inferred distinction in [`PRODUCT_PRINCIPLES.md`](../PRODUCT_PRINCIPLES.md).
Replacing it with a selector result would weaken rather than strengthen the
evidence contract. If Scrapling were ever used in a fixture experiment, it
should emit only a candidate selector and similarity diagnostic; the normal Go
parser must remain authoritative.

### Adaptive parsing

Adaptive mode is disabled by default. The save phase records the selected
element's tag, text, attributes, sibling tags, tag path, and parent properties
in SQLite by default. On a later miss, it scores page elements and returns the
highest-similarity candidate. Storage is replaceable behind a small interface.
([algorithm](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/parsing/adaptive.md#how-the-adaptive-scraping-feature-works),
[storage interface](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/development/adaptive_storage_system.md))

The official docs explicitly include troubleshooting for wrong matches and
state that only the first result's properties are saved for an ordinary
multi-element selection
([limitations](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/parsing/adaptive.md#known-issues)).
For shopping, account, order, or receipt evidence, silently accepting such a
relocation could turn a layout resemblance into a false observed fact. The
safe response to source drift remains a typed `source_shape_unavailable`-style
failure plus a redacted fixture update and semantic review.

### Deployment and maintenance cost

The parser-only install already brings native/performance-oriented Python
packages such as lxml and orjson. Browser fetching adds two browser automation
engines plus browser/fingerprint packages; the project is classified as beta in
its own package metadata
([`pyproject.toml`](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/pyproject.toml)).
The official install path then downloads browser and system assets, or uses a
container image containing them.

That is acceptable for a Python scraping application, but not for this
product's established distribution contract: one native executable, an
installed supported Chrome, and no Python/Playwright runtime or sidecar
([language decision](language-ecosystem-decision.md),
[browser distribution decision](browser-distribution-alternatives.md)). It
would also create a second dependency update cadence around a rapidly changing
browser API.

### Safety and legal boundary

Scrapling includes fingerprint modification, browser impersonation, proxy
rotation, and challenge-solving features in its advertised surface. Those
features are deliberately **out of scope** here. `coupangctl` must not use
StealthyFetcher, spoof fingerprints, rotate proxies to avoid blocks, solve or
bypass CAPTCHAs/challenges, or describe browser reuse as evasion. A source
challenge remains a typed stop that requires normal human action.

The spider framework can honor `Disallow`, `Crawl-delay`, and `Request-rate`,
but `robots_txt_obey` defaults to `False`
([robots behavior](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/docs/spiders/getting-started.md#robotstxt-compliance)).
Therefore merely adopting Scrapling would not supply a compliance policy.
Independent review of authorization, site terms, privacy obligations, request
bounds, and retention is still required. Regardless of library choice, no
credentials, cookies, OTPs, PII, raw order payloads, or real customer fixtures
may enter logs, tests, documentation, or MCP/CLI responses.

## Recommendation

1. Keep the production acquisition path in the existing Go browser adapters
   and keep structured endpoint/parser changes behind narrow interfaces.
2. Reproduce Scrapling's useful XHR-capture ergonomics in Go with URL
   allowlisting, bounded bodies, response-shape checks, redacted diagnostics,
   and immediate conversion to typed records.
3. Prefer source-native JSON and typed validation over adaptive DOM extraction.
4. If selector drift becomes a recurring maintenance cost, evaluate Scrapling's
   parser only against synthetic/redacted offline fixture revisions. Treat all
   relocated elements as inferred candidates requiring human confirmation and
   ordinary parser tests.
5. Do not add Scrapling, Python, Playwright, Patchright, bundled browsers, or
   Scrapling's MCP server to release artifacts.
