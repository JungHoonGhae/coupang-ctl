# Ordinary-browser protected-data bridge

Validated: 2026-09-03 (Asia/Seoul)

## Scope

This note specifies a least-privilege bridge between an explicitly paired
Manifest V3 Chrome extension and the local `coupangctl` Go process. Its only
purpose is to let read-only source adapters use a user's already-authenticated,
ordinary Chrome context and return documented, typed data to the core.

This is not an authentication-challenge mechanism. It must not read or copy
cookies, automate CAPTCHA or OTP flows, add browser-fingerprinting workarounds,
or implement purchase or payment. Tests and documentation must use synthetic
fixtures and redacted metadata, never real customer payloads.

## Decision

Use **Chrome Native Messaging as the production browser-to-Go transport**.
The Chrome-spawned native host connects to a separately running CLI through a
private, authenticated, short-lived loopback rendezvous. The browser never
contacts that listener, so this internal Go-to-Go IPC does not require CORS,
host permission, or Chrome Local Network Access. A direct browser-to-loopback
transport remains only an explicitly experimental fallback for environments
where native-host registration is not possible.

The minimum first-release permission tier is:

```json
{
  "manifest_version": 3,
  "permissions": ["activeTab", "nativeMessaging", "scripting"],
  "incognito": "not_allowed"
}
```

This tier requires a visible user gesture on the intended Coupang tab for each
read. Chrome grants `activeTab` access only after actions such as clicking the
extension action, and revokes it when the tab closes or navigates to a different
origin. It is specifically documented as a lower-risk alternative to broad,
persistent host access and shows no install warning.
([Chrome `activeTab`](https://developer.chrome.com/docs/extensions/develop/concepts/activeTab))

If unattended local refresh is later proven necessary, expose it as a separate
opt-in tier using only exact, audited Coupang HTTPS origins in
`optional_host_permissions`. Chrome requires optional permissions to be
requested from a user gesture, and host-permission paths are ignored, so the
adapter must still enforce exact path and method allowlists in code.
([Chrome optional permissions](https://developer.chrome.com/docs/extensions/reference/api/permissions),
[Chrome match patterns](https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns))

Do not request `<all_urls>`, `cookies`, `tabs`, `debugger`, `webRequest`,
`declarativeNetRequest`, clipboard, history, or persistent background access.
Do not add a permission in anticipation of a future feature. Chrome recommends
the narrowest permissions necessary, and the Chrome Web Store applies that
minimum-permission requirement to both required and optional permissions.
([Chrome permission guidance](https://developer.chrome.com/docs/extensions/develop/concepts/declare-permissions),
[Chrome Web Store user-data FAQ](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq))

## Recommended architecture

```text
CLI / MCP adapter
      |
      | typed request/result over private per-user local IPC
      v
Go bridge broker / native host
      |
      | Chrome Native Messaging: framed JSON, exact extension origin
      v
MV3 extension service worker
      |
      | chrome.scripting.executeScript, ISOLATED, top frame only
      v
Explicitly selected ordinary Coupang tab
      |
      | fixed same-origin, read-only request or structured document read
      v
Browser-owned authenticated session
```

The bridge remains an adapter at both ends:

- The CLI and MCP layers call the same typed `ProtectedDocumentSource`
  boundary as the existing installed-browser source. They do not receive a
  tab, DOM node, cookie, network header, or Chrome API object.
- The Go bridge accepts a small operation enum, not a URL or script supplied by
  the caller. Each operation maps to an allowlisted HTTPS origin, path template,
  method, response type, maximum size, and timeout.
- The extension returns normalized records or an explicitly versioned
  structured source shape. It never returns raw HTML, a raw order response,
  cookies, storage, request headers, or bearer values.
- The typed core validates the versioned shape again and records source
  provenance. Reverse-engineered source details stay inside the narrow adapter.

### P0 interaction

1. `coupangctl browser-bridge install` registers the native-host manifest and
   verifies that its `allowed_origins` contains only the released extension ID.
2. The user installs the signed extension and opens the relevant authenticated
   Coupang page in ordinary Chrome.
3. A CLI/MCP read creates a bounded pending job in the private local broker.
   The terminal tells the user to click the extension action; it never asks for
   a cookie or OTP.
4. The action click grants `activeTab`. The service worker connects to the
   native host, receives one versioned operation, confirms that the active top
   frame has an allowlisted HTTPS origin and path, and runs packaged extraction
   code in the isolated world.
5. The extension and Go adapter independently validate the result, then the Go
   adapter hands only the typed result to the core. The pending job and all
   transient buffers are discarded on success, error, timeout, tab navigation,
   or disconnect.

This user-gesture tier is deliberately not advertised as background sync. A
future opt-in tier can request narrowly scoped host permission and reconnect to
the native host at browser startup, but it needs a separate consent screen,
revocation control, lifecycle tests, and Chrome Web Store disclosure.

## Why Native Messaging is the default transport

Chrome Native Messaging was designed for extensions to exchange JSON messages
with registered native applications. Chrome starts the host as a separate
process, uses stdin/stdout framing, and requires the extension to declare
`nativeMessaging`. Content scripts cannot call it directly; an extension page
or service worker must relay the operation.
([Chrome Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging))

The native-host manifest supplies an absolute executable path on macOS and
Linux and an `allowed_origins` list whose extension IDs cannot use wildcards.
Chrome also passes the caller origin as the host's first argument. The host
should still compare that argument with the one compiled/configured extension
ID before accepting a frame.
([native-host manifest and locations](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host))

Native Messaging has useful built-in boundaries that a local HTTP listener has
to recreate:

- no listening TCP port exposed to arbitrary webpages or local processes;
- an exact Chrome extension ID allowlist enforced before the host is used;
- no CORS, mixed-content, or Local Network Access dependency;
- a length-delimited JSON protocol with documented limits;
- Chrome-managed host process lifetime and disconnect reporting.

The documented maximum is 64 MiB from extension to native host and 1 MiB from
native host to extension. The bridge should impose a much smaller limit, such
as 256 KiB per frame, and chunk normalized records. A size limit is not
permission to transmit raw responses.
([Native Messaging protocol](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-protocol))

Use `runtime.connectNative()` rather than spawning a new host with
`sendNativeMessage()` for every frame. Chrome documents that a native port
keeps the host process running, and Chrome 105+ keeps the MV3 service worker
alive while that port is connected. The implementation must reconnect after a
disconnect rather than assume that an MV3 global remains alive.
([native connection lifetime](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#connecting-to-a-native-application),
[extension service-worker lifecycle](https://developer.chrome.com/docs/extensions/develop/concepts/service-workers/lifecycle))

`nativeMessaging` produces the explicit warning “Communicate with cooperating
native applications.” This is appropriate and should be explained in the
store listing and pairing UI rather than hidden.
([Chrome permissions list](https://developer.chrome.com/docs/extensions/reference/permissions-list))

## Page access and execution world

### `activeTab` versus host permissions

`activeTab` is the correct default because P0 needs one intentionally selected
tab, not permanent access to every matching page. Together with `scripting`, it
allows programmatic injection after a recognized user gesture. The grant is
temporary and tied to the active tab's main-frame origin.
([Chrome `activeTab`](https://developer.chrome.com/docs/extensions/develop/concepts/activeTab),
[`chrome.scripting`](https://developer.chrome.com/docs/extensions/reference/api/scripting))

Persistent host access is required only for a genuinely unattended feature. If
that feature is implemented, request each exact HTTPS origin at runtime via
`optional_host_permissions`; do not declare a wildcard Coupang subdomain unless
live, redacted endpoint evidence proves that every included host is needed.
Because Chrome ignores the path portion of host permissions, application code
must reject all unlisted paths, query shapes, and non-GET methods.
([Chrome match patterns](https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns))

### Isolated world by default; no main-world bridge

Use `chrome.scripting.executeScript()` in the default `ISOLATED` world and the
top frame only. Chrome documents that isolated-world variables are not visible
to the host page, while `MAIN` shares the page's JavaScript environment and lets
the page access and interfere with the injected code.
([content-script execution worlds](https://developer.chrome.com/docs/extensions/reference/manifest/content-scripts))

The injected function must be packaged with the extension, self-contained, and
limited to one predefined read operation. `chrome.scripting` waits for a
returned promise and sends its result back to the extension, so P0 does not
need a page-global event bridge or a web-accessible script.
([`chrome.scripting` results](https://developer.chrome.com/docs/extensions/reference/api/scripting#handle-the-results))

If a source is available only through a page-owned JavaScript variable, treat
that as `source_shape_unavailable`, not as permission to switch silently to
`MAIN`. A separately reviewed main-world adapter would need to prove that an
isolated same-origin fetch or structured DOM script element cannot supply the
same evidence, and would require a much tighter output validator.

Content scripts execute in the page renderer and must be treated as untrusted
even when isolated. Chrome explicitly warns that a hostile page may manipulate
DOM inputs or compromise the renderer; messages from content scripts must be
validated and must not be allowed to trigger arbitrary URLs or privileged API
arguments.
([Chrome extension security guidance](https://developer.chrome.com/docs/extensions/develop/security-privacy/stay-secure#use-content-scripts-carefully))

### Structured reads, not DOM selectors

Prefer a same-origin, read-only structured request with a hardcoded adapter
route. Chrome documents that content-script fetches act on behalf of the page's
web origin and remain subject to the same-origin policy. This keeps the browser
session browser-owned without granting the `cookies` API or exporting cookie
values.
([Chrome cross-origin request model](https://developer.chrome.com/docs/extensions/develop/concepts/network-requests))

Do not use `chrome.cookies`, `document.cookie`, storage export, or a profile
copy. The Cookies API would require both the `cookies` permission and host
permission and exposes cookie values, which the bridge does not need.
([Chrome Cookies API](https://developer.chrome.com/docs/extensions/reference/api/cookies))

Every response must be checked before leaving the isolated execution:

- exact final HTTPS origin and allowlisted path family;
- GET only, bounded redirect count, deadline, byte count, and record count;
- expected status, content type, top-level keys, and primitive types;
- explicit login/challenge/access-denied classification;
- no HTML fallback, header dump, cookie-like field, raw body, or unknown key;
- stable schema version and provenance tag.

The result must be validated again in the service worker and Go adapter. This
is defense in depth against a compromised page renderer, source-shape drift,
and protocol-version mismatch.

## Messaging boundaries

Use internal extension messaging only when a persistent content script is
unavoidable. Chrome provides one-time and long-lived JSON-serializable messages
between content scripts and the extension service worker, and recommends
validating even internal senders because the content script may be compromised.
([Chrome message passing](https://developer.chrome.com/docs/extensions/develop/concepts/messaging))

Do not register `runtime.onMessageExternal` or `runtime.onConnectExternal`.
`externally_connectable` controls webpage/other-extension access to those APIs;
it is not a native-app transport and does not affect content scripts. Omitting
the manifest key prevents webpages from connecting, though other extensions
can attempt a connection by default; with no external listeners there is no
supported external command surface.
([`externally_connectable`](https://developer.chrome.com/docs/extensions/reference/manifest/externally-connectable))

Do not add web-accessible resources for the bridge. Chrome notes that such
resources make an extension visible to websites and expand its attack surface.
([Chrome extension security guidance](https://developer.chrome.com/docs/extensions/develop/security-privacy/stay-secure#limit-manifest-fields))

## Versioned protocol

The wire contract should use a closed set of envelopes. Illustrative field
names follow; these are protocol shapes, not real payloads:

```json
{
  "protocol": 1,
  "type": "request",
  "request_id": "opaque-random-id",
  "operation": "orders.read_page",
  "deadline_ms": 15000,
  "arguments": {"page": 1}
}
```

```json
{
  "protocol": 1,
  "type": "result",
  "request_id": "opaque-random-id",
  "schema": "orders.normalized.v1",
  "chunk": 0,
  "final": true,
  "provenance": {"source": "ordinary_browser", "kind": "observed"},
  "records": []
}
```

Required protocol rules:

- operation is an enum; URLs, JavaScript, headers, HTTP bodies, file paths, and
  shell arguments are never accepted from a caller;
- random request IDs correlate frames but grant no authority;
- one operation is active per selected tab, with explicit cancellation and a
  hard deadline;
- all integers, strings, arrays, and nesting depths have small bounds;
- unknown fields, versions, operations, and state transitions fail closed;
- error envelopes contain stable codes and redacted metadata only;
- native-host stdout contains protocol frames only; diagnostics go to stderr
  and must still exclude private values;
- no extension storage contains order data, auth material, or response bodies.

## Loopback fallback

This section concerns a direct extension/browser-to-loopback transport. It does
not describe the implemented native-host-to-CLI rendezvous, which is invisible
to Chrome and carries only the closed bridge protocol between two local Go
processes.

A browser-to-loopback Go server is easier to prototype because the already
running CLI can own the listener, but it is not the least-privilege production
choice.

Chrome extension service workers need host permission for CORS-bypassing
cross-origin fetches. Without it, the Go server must implement CORS for the
extension origin. A host permission such as `http://127.0.0.1/*` covers all
ports by default, and its path is ignored, so it grants more network reach than
the single ephemeral listener actually needs.
([Chrome cross-origin requests](https://developer.chrome.com/docs/extensions/develop/concepts/network-requests),
[Chrome localhost match patterns](https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns#special-cases))

Chrome 142 introduced Local Network Access permission gating for webpage
requests to local and loopback destinations. Chrome 145 split that grant into
`local-network` and the narrower `loopback-network`, and Chrome 147 extended
the prompt to WebSockets. The exact interaction among a current MV3 extension
origin, its service worker, host permission, and the LNA grants is not stated as
a stable extension-specific contract in those release notes. It therefore needs
a release-matrix test; CORS success alone is not a durable compatibility claim.
([Chrome 142 release notes](https://developer.chrome.com/release-notes/142#local-network-access-restrictions),
[Chrome 145 LNA split permissions](https://developer.chrome.com/release-notes/145#local-network-access-split-permissions),
[Chrome 147 LNA WebSocket restrictions](https://developer.chrome.com/release-notes/147#local-network-access-restrictions-for-websockets))

If loopback is retained as an experimental fallback, require all of the
following:

- bind only to an explicit loopback address, never `0.0.0.0`, `::`, LAN, or a
  hostname supplied by a client;
- select an ephemeral port and validate the `Host` header against that exact
  address and port;
- have the user paste a high-entropy, single-use pairing value into the
  extension UI without requesting clipboard permission;
- keep the pairing secret only in Go memory and, if service-worker suspension
  must be survived, `chrome.storage.session`, which requires the fallback
  variant to add `storage` and is not exposed to content scripts by default;
- require the secret on every request, rotate it after pairing, expire it
  quickly, and bind it to protocol version plus extension ID;
- allow exactly the released `chrome-extension://...` origin in CORS, never
  `*`, while treating `Origin` as a filter rather than authentication;
- accept only fixed POST routes and JSON media types, with body, rate, timeout,
  and concurrency limits; all GET routes must be inert;
- never put a secret in a URL, query string, log, browser storage, or error;
- shut down the listener immediately after the bounded operation or disconnect.

These controls treat CORS and `Origin` only as browser-side filters. The
WebSocket standard notes that non-browser clients can send an arbitrary
`Origin`; OAuth's native-app security guidance likewise treats loopback port
interception as a real threat and requires a per-request proof rather than a
secret statically embedded in distributed software. Applying those principles
here is an architectural inference, not a claim that this bridge implements
OAuth.
([RFC 6455, Origin considerations](https://www.rfc-editor.org/rfc/rfc6455#section-10.2),
[RFC 8252, loopback interception](https://www.rfc-editor.org/rfc/rfc8252#section-8.1),
[RFC 8252, statically included secrets](https://www.rfc-editor.org/rfc/rfc8252#section-8.5))

Chrome documents that `chrome.storage.session` is in-memory, cleared on browser
restart/extension reload, and hidden from content scripts by default. If the
fallback adds the `storage` permission, this area is suitable for an ephemeral
pairing value; `storage.local` and `storage.sync` are not.
([Chrome Storage API](https://developer.chrome.com/docs/extensions/reference/api/storage#property-session))

A loopback WebSocket does not remove these requirements. On Chrome 116+, traffic
over an active WebSocket can extend an MV3 service worker lifetime, but periodic
keepalives create a longer-lived local capability; since Chrome 147, loopback
WebSockets also trigger the LNA permission path. Use a single short operation
rather than an always-on socket if the fallback is ever shipped.
([WebSockets in extension service workers](https://developer.chrome.com/docs/extensions/how-to/web-platform/websockets),
[extension service-worker lifecycle](https://developer.chrome.com/docs/extensions/develop/concepts/service-workers/lifecycle),
[Chrome 147 release notes](https://developer.chrome.com/release-notes/147#local-network-access-restrictions-for-websockets))

## Cookies, profiles, and incognito

The bridge relies on the currently selected ordinary tab's browser-owned
session. It does not copy state into a dedicated profile or into Go.

Set `"incognito": "not_allowed"`. Chrome does not enable extensions in
incognito by default, and split-incognito mode has a separate in-memory cookie
store while extension local/sync storage remains shared. Supporting it would
create ambiguous data ownership and persistence behavior without helping the
ordinary-profile use case.
([Chrome incognito manifest key](https://developer.chrome.com/docs/extensions/reference/manifest/incognito),
[Chrome privacy guidance](https://developer.chrome.com/docs/extensions/develop/security-privacy/user-privacy#saving-data-and-incognito-mode))

Pairing state and private results must never use `storage.local` or
`storage.sync`. Chrome warns that extension storage is not encrypted, and
Chrome Web Store policy treats authentication information, browsing activity,
page content, and locally processed data as user data that requires accurate
disclosure.
([Chrome privacy guidance](https://developer.chrome.com/docs/extensions/develop/security-privacy/user-privacy),
[Chrome Web Store user-data FAQ](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq))

## Installation and distribution

Production distribution needs a stable extension ID because the native-host
manifest must allowlist an exact `chrome-extension://<id>/` origin. Chrome
documents using the Web Store public key to keep the same ID during development.
([Chrome manifest `key`](https://developer.chrome.com/docs/extensions/reference/manifest/key))

Recommended release path:

1. Publish an MV3 extension through the Chrome Web Store, initially private or
   unlisted for testing. All visibility modes undergo the same policy review.
2. Bundle all executable JavaScript with the extension. MV3 forbids remotely
   hosted code; fetched JSON is data and must never be evaluated.
3. Have `coupangctl browser-bridge install` write the per-user native-host
   registration with an absolute binary path and exact released extension ID.
4. Have `browser-bridge doctor` verify file ownership/permissions, manifest
   shape, executable version, extension ID, and a redacted ping without opening
   a protected page. **Implemented:** the seventh check runs the authenticated
   rendezvous, exact-origin Native Messaging framing, and a synthetic typed
   empty-page round trip without starting Chrome or reading private state.
5. Make uninstall remove only the registration created by the same installation;
   it must not remove Chrome profiles, cookies, extension data, or user orders.

Chrome documents platform-specific native-host manifest locations: a registry
entry on Windows, `NativeMessagingHosts` below the Chrome application-support
directory on macOS, and system/per-user Chrome configuration locations on
Linux.
([native-host locations](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location))

Ordinary users on Windows and macOS install extensions through the Chrome Web
Store; outside-store self-hosting is limited to enterprise policy. Linux also
supports self-hosting, but the Store-signed path is simpler for one stable ID
across platforms.
([Chrome extension distribution](https://developer.chrome.com/docs/extensions/how-to/distribute),
[alternative installation methods](https://developer.chrome.com/docs/extensions/how-to/distribute/install-extensions))

The Store listing and in-product consent must accurately disclose that the
extension reads the user's selected Coupang page/order data locally for the
requested `coupangctl` feature. Local-only processing is still user-data
handling under Store policy. No analytics or telemetry should be enabled by
default.
([Chrome Web Store user-data FAQ](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq),
[Chrome Web Store distribution visibility](https://developer.chrome.com/docs/webstore/cws-dashboard-distribution/))

## Threat model and required mitigations

| Threat | Required control |
| --- | --- |
| A hostile or compromised page forges DOM/state | Isolated top-frame execution; fixed GET operations; no arbitrary URL; strict response schema; revalidate in service worker and Go. |
| A compromised content script becomes a confused deputy | Treat every renderer result/message as untrusted; never send a secret or cross-origin data into the renderer; allow no privileged free-form arguments. |
| Another extension or webpage sends commands | No external listeners, no webpage messaging, no web-accessible bridge resources; validate sender tab/frame/document for all internal messages. |
| A malicious local process contacts a loopback listener | Prefer Native Messaging; for fallback use a short-lived high-entropy capability, exact origin/host checks, bounded routes, and immediate shutdown. Loopback alone is not authentication. |
| A native-host manifest or binary is replaced | Per-user ownership/ACL checks, absolute paths, exact `allowed_origins`, caller-origin verification, signed extension updates, and a doctor command that fails closed. |
| An update broadens access silently | Permission snapshot tests, optional grants for new hosts, visible release notes, and Chrome Web Store review; no remotely hosted code. |
| Replay, cross-job mix-up, or concurrent tab race | Random request IDs, single active operation per tab, state-machine validation, deadlines, cancel-on-navigation, and result-to-request correlation. |
| Private data leaks through logs/storage | No raw bodies, cookies, headers, PII, order IDs, or product text in logs; no extension persistence of results; typed Go storage only; redacted error metadata. |
| Incognito data crosses contexts | `incognito: not_allowed`; reject incognito tabs defensively even if browser settings are inconsistent. |
| Source endpoint changes | Narrow reverse-endpoint adapter, schema versioning, fail-closed unknown keys, synthetic contract tests, and redacted live metadata probes. |

Chrome's MV3 model also requires all executable code to be inside the reviewed
extension package. Keep a restrictive extension-page CSP and never evaluate
data returned by Coupang or the native host.
([Manifest V3 remote-code restriction](https://developer.chrome.com/docs/extensions/develop/migrate/what-is-mv3),
[Chrome extension CSP guidance](https://developer.chrome.com/docs/extensions/develop/security-privacy/stay-secure#include-an-explicit-content-security-policy))

## Verification gates before implementation is called available

Implementation status on 2026-09-03: the macOS per-user host installer and all
seven local doctor checks passed, followed by four consecutive bounded one-page
reads through the selected ordinary Chrome tab into the typed SQLite sync
path. Before the fourth run, Chrome's detail page confirmed the installer-
managed `extension_path` as the loaded bundle location. The test retained only
counts and cursors. A separate clean macOS 26.3.1 arm64 VM with Chrome 152 then
passed fresh install, all seven doctor checks including the synthetic native-
host ping, ownership-checked uninstall, and a residue check without starting
Chrome or accessing a profile or account. This narrows packaging uncertainty
but does not close selected-tab clean-profile or Linux/Windows live gates.

- Manifest test asserts the exact permission set and `incognito: not_allowed`;
  forbidden permissions and external listeners fail the build.
- Packaged-extension test proves there is no remotely hosted executable code,
  main-world script, broad host match, or web-accessible bridge resource.
- Native-host tests reject every extension ID except the released/test ID,
  malformed framing, oversized/deep JSON, unknown operations, duplicate or
  expired request IDs, and stdout diagnostics.
- Synthetic page tests cover success, login redirect, challenge, access denial,
  schema drift, oversized response, timeout, cross-origin redirect, navigation,
  and disconnect without recording raw bodies.
- End-to-end tests verify that a click grants one active-tab read, a cross-origin
  navigation revokes it, and an incognito tab is rejected.
- Clean-machine packaging tests cover current supported Chrome on macOS,
  Windows, and Linux, including install, doctor, update, revoke, and uninstall.
- A release-only live smoke test records status codes, schema/key types, counts,
  and error classes only. It must prove that the ordinary browser context can
  read the protected structured source repeatedly without ever emitting a real
  value or raw payload.
- If loopback remains, test current Stable/Beta Chrome with LNA enforcement,
  missing/wrong `Origin`, DNS rebinding-style host changes, port collisions,
  browser restart, service-worker suspension, and local connection races.

## Open uncertainties

1. **Protected source behavior:** a redacted live test still has to prove which
   fixed same-origin structured read works from an isolated-world injection in
   the ordinary Chrome tab. Some source calls may require page-generated
   non-cookie state; P0 must return an honest unavailable error rather than
   extract or export that state broadly.
2. **Permission tier sufficiency:** confirm on supported Chrome versions that
   `activeTab` plus `scripting` covers the full selected-tab operation and that
   every redirect/navigation invalidates the job as designed.
3. **Native-host broker lifecycle:** resolved for the experimental CLI path.
   The Chrome-spawned Go host connects to the separate waiting CLI using an
   exact loopback address and 32-byte one-time token from a private `0600`
   rendezvous. The file expires after two minutes, is removed immediately after
   authentication, rejects a second writer, and cannot be deleted by an older
   owner. Clean-machine crash and update recovery remain packaging gates.
4. **Chrome Web Store review:** local order-data processing, optional host
   access, and native messaging require complete single-purpose and privacy
   disclosures. Approval is an external release gate, not something unit tests
   can prove.
5. **Loopback compatibility:** the precise Chrome 142+ LNA and CORS behavior for
   an MV3 extension service worker is not sufficiently guaranteed by the
   general webpage documentation. Do not promote loopback to the default until
   a versioned Chrome matrix proves it.
6. **Host allowlist:** exact Coupang origins and read-only paths must come from
   the project's redacted endpoint evidence. Do not infer or wildcard them for
   convenience.

## Implementation order

1. **Done:** define the closed, versioned request/result/error schema and synthetic
   fixtures in the typed browser adapter package.
2. **Done:** build the Go native-host framing layer and origin/size/state-machine tests.
3. **Done:** build the MV3 action flow with only `activeTab`, `nativeMessaging`, and
   `scripting`; use an isolated synthetic test origin first.
4. **Partial:** add the typed `PageSource` CLI path and complete one redacted
   ordinary-Chrome first-page live read. Multi-run live repeatability remains.
5. **Next:** package the extension and native-host installer for the supported desktop
   OSes, then complete Store privacy/review work.
6. Evaluate the optional persistent-origin tier only after P0 is reliable.
7. Retain loopback as experimental unless native-host registration proves to be
   the larger verified reliability problem.
