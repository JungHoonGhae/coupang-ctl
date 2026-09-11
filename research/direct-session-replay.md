# Direct authenticated read replay

Validated: 2026-09-04 (Asia/Seoul)

## Question

Can `coupangctl` reuse the authenticated metadata of a user-approved Chrome
session for a bounded, read-only Go HTTP request, as `tossctl` does, without
persisting a second cookie jar or raw HAR?

## Evidence

The test used the existing allowlisted order-model adapter on macOS with Chrome
152. It retained cookies and request metadata only in memory, never printed or
persisted them, and accepted a result only when the existing typed order parser
validated it.

| Variant | Result |
| --- | --- |
| Same-origin `fetch` inside the approved Chrome session | 2xx; typed order document accepted |
| Go HTTP with matching Chrome cookies and actual User-Agent | 403 |
| Go HTTP with HTTP/2 and browser-equivalent cookie domain/path selection | 403 |
| Go HTTP with HTTP/2 plus Chrome Fetch Metadata and Client Hint headers | 2xx; typed order document accepted |
| Go HTTP built from a redacted CDP request contract captured from the actual Chrome request | 2xx; typed order document accepted |
| Production path: first model page learns the contract, next page replays through the Go adapter | 2xx; typed order document accepted |

This proves that the endpoint is not categorically unavailable to a direct
client. It requires request metadata coherent with the browser session. The
result does not prove that every Coupang endpoint, OS, Chrome version, account,
or future source revision has the same contract.

## Adopted design

- Chrome remains the durable session authority; cookies are read through CDP
  for one request and are never written to a product session file.
- The first order-model read uses Chrome and observes only an allowlisted host,
  path, method, query-key set, and reviewed non-secret headers.
- Cookie, Authorization, XSRF, unknown headers, raw request bodies, raw response
  bodies, and HAR files are excluded from the captured contract.
- Later pagination reads may use Go HTTP/2 with the captured contract and
  in-memory matching cookies.
- Direct reads are paced by 250 ms. Any rejection, redirect, invalid JSON,
  typed order-contract mismatch, or oversized response
  clears the contract and immediately returns to the browser-backed path.
  `Set-Cookie` emitted to the direct transport is deliberately ignored: it is
  neither injected into Chrome nor retained in a Go cookie jar. Each replay
  starts from Chrome's unchanged cookie state. If the source requires a
  rotation, the next replay is rejected and falls back so only Chrome's own
  response can change the durable session.
- CAPTCHA, fingerprint mutation, proxy rotation, and challenge circumvention
  are not involved.

A same-page control comparison on 2026-09-04 normalized identically across a
Chrome-backed read, a direct HTTP/2 read, and a second forced Chrome-backed
read. A subsequent public CLI run completed a bounded three-page
current-browser sync and persisted the next cursor. After direct-response
cookie isolation was tightened, a one-connection pipeline repeated the
Chrome/direct/forced-Chrome comparison successfully. This verifies the current
cookie-authority boundary for one bounded page; no test logged order contents,
request headers, or cookie data.

An official Go MCP SDK client also started `coupangctl mcp --current-browser`
and completed `order sync → product search → order sync` through the same
process. Both order calls returned typed current-browser provenance and one
processed page, while product search returned a typed result; the retained
browser connection avoided a second debugger attachment.

After the navigation capture learned late safe ExtraInfo headers, a separate
live product test completed the first JSON-LD search through Chrome and the
second through direct HTTP/2 without creating another page target. No product
document, request header, or cookie value was logged.

## Remaining verification

- Repeat current-browser bootstrap from an empty local ledger after the source
  access window has cooled down; a 2026-09-04 attempt reached a typed access
  denial on the initial order-list read while an existing-cursor MCP sequence
  had passed earlier. Do not reinterpret that denial as logout.
- Repeat the live contract on clean Windows and Linux Chrome profiles.
- Extend the current isolated-cookie equality check from one page to bounded
  multi-page pagination.
- Repeat the live request-contract and long-lived-process checks on clean
  Windows and Linux Chrome profiles.
- Measure latency and fallback counts without recording order payloads.
- Product detail remains a separate endpoint family and is not directly
  replayed.
