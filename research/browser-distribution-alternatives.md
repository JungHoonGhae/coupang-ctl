# Browser distribution alternatives

Validated: 2026-09-03 (Asia/Seoul)

## Executive decision

`coupangctl` should **not require a Chrome extension for its default consumer
flow**. The default should be the existing native Go executable plus a supported
Chrome installation already present on the desktop:

1. `coupangctl` discovers and launches installed Chrome with a private,
   product-owned persistent profile.
2. The first authentication or renewal opens that profile in a visible browser
   and leaves every challenge to the user.
3. Later read-only syncs launch the same profile through a short-lived,
   loopback-only CDP endpoint. They remain headless by default and return an
   explicit typed status when human-visible action may be needed.
4. Browser session state stays in the browser profile. It is not copied into a
   `coupangctl` cookie file, command output, MCP response, or server export.
5. The ordinary-browser extension bridge becomes an optional compatibility
   path, not a prerequisite.

This is zero **extension** setup and has no Node, Playwright, ChromeDriver,
Orca, accessibility controller, or separately downloaded browser dependency at
runtime. It is not literally dependency-free: a supported installed Chrome and
an interactive desktop are required for initial authentication.

There is no supported way for a local CLI to silently take over an already
running, normally launched, signed-in Chrome profile while also requiring **no
extension, no session-state copying, no prior browser setting, no relaunch, and
no user approval**. Current Chrome deliberately requires one of those trust
boundaries.

## What Chrome permits now

### An already-running ordinary Chrome

Chrome 144+ has an official extension-free **auto-connect** flow for agents.
It can inherit the running profile's tabs and live application state, but the
user must first enable remote debugging at
`chrome://inspect/#remote-debugging`, configure the client, and click **Allow**
when Chrome asks to approve a connection. Chrome warns that the agent can then
access all data in that browser profile, including open tabs, cookies, and
storage. This is useful as an explicit power-user mode, but it does not meet a
zero-configuration or unattended-consumer requirement.
([Chrome auto-connect guide](https://developer.chrome.com/docs/devtools/agents/use-cases/auto-connect),
[Chrome DevTools MCP advanced usage](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/main/docs/advanced-usage.md#connecting-to-a-running-chrome-instance))

The product exposes this experimentally as `orders sync --current-browser` with
the following truthful contract:

- `current-browser status` and MCP `current_browser_status` passively validate
  only the private endpoint metadata, without a WebSocket attach or approval
  prompt;
- no extension install;
- one-time Chrome remote-debugging opt-in;
- Chrome-controlled approval when attaching;
- access is broader than a selected-tab `activeTab` extension;
- never suitable for unattended scheduled sync;
- never silently enabled or described as an anti-detection feature.

Keeping one approved connection alive in a local broker could reduce repeated
prompts during that process lifetime, but it would not eliminate the initial
browser opt-in or Chrome approval. That is an implementation inference, not a
Chrome guarantee, and must not be marketed as automatic reconnection.

On 2026-09-03, a clean macOS 26.3.1 arm64 VM with Chrome 152 returned the
expected passive `not_enabled` state without starting Chrome. A separate 0700
temporary Chrome profile launched with a browser-selected loopback port then
returned `endpoint_available`; two consecutive real CLI attachments both
classified its intentionally logged-out order page as `authentication_required`
and disconnected without closing Chrome. The exact process and temporary state
were removed afterward. This is redacted evidence for discovery, repeated
attachment, error normalization, and disconnect lifecycle. It does not verify
Chrome's settings-page approval prompt or an authenticated order response, so
the clean-profile release gate remains open.

### CDP remote-debugging flags

Since Chrome 136, `--remote-debugging-port` and
`--remote-debugging-pipe` are ignored when they target Chrome's default user
data directory. Chrome requires a non-standard `--user-data-dir`, which uses a
different encryption key, and recommends that separation for debugging.
([Chrome 136 remote-debugging security change](https://developer.chrome.com/blog/remote-debugging-port))

This closes the old pattern of relaunching ordinary Chrome with a port and
quietly attaching to the user's everyday profile. It directly supports the
recommended product-owned-profile design.

For a CLI-launched dedicated profile, `--remote-debugging-port=0` lets Chrome
choose a free port and write the browser endpoint to `DevToolsActivePort` in
that profile. The CLI can discover the endpoint without a fixed global port.
CDP itself is structured JSON, but the tip-of-tree protocol can change without
backwards-compatibility guarantees, so `coupangctl` must keep the few methods it
uses behind its narrow browser adapter.
([Chrome DevTools Protocol FAQ](https://chromedevtools.github.io/devtools-protocol/#faq))

Chrome profile state includes cookies and other private browsing data. Two
running Chrome instances cannot share one user-data directory, so the product
must lock its dedicated profile and return `profile_in_use` instead of copying
or racing it.
([Chromium user-data-directory documentation](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/user_data_dir.md))

### `--load-extension`

Starting in Chrome 137, official Chrome-branded builds removed command-line
loading of unpacked extensions because it was abused to load unwanted software.
The flag remains available in Chromium and Chrome for Testing, but those are
automation/test distributions, not a mechanism for injecting an extension into
an already-running consumer Chrome.
([Chrome Extensions June 2025 update](https://developer.chrome.com/blog/extension-news-june-2025),
[Chromium Extensions announcement](https://groups.google.com/a/chromium.org/g/chromium-extensions/c/1-g8EFx2BBY))

Puppeteer and WebDriver BiDi expose test-time extension installation for a
browser session they control. That can help extension QA, but it does not turn
an unpacked extension into a silent consumer installation or attach it to an
ordinary profile.
([Puppeteer extension testing](https://pptr.dev/guides/chrome-extensions),
[WebDriver BiDi `webExtension` module](https://w3c.github.io/webdriver-bidi/#module-webExtension))

### WebDriver and WebDriver BiDi

WebDriver's `New Session` command creates a WebDriver session with a remote end;
BiDi adds a WebSocket to that session. The protocol is a better long-term
cross-browser automation API, but it is not a discovery-and-consent mechanism
for arbitrary ordinary Chrome processes.
([W3C WebDriver sessions](https://w3c.github.io/webdriver/#sessions),
[W3C WebDriver BiDi](https://w3c.github.io/webdriver-bidi/#transport))

ChromeDriver can attach to an existing Chrome only when that Chrome already
exposes a debugger address. Some commands remain unavailable because
ChromeDriver's automation extension is loaded only when ChromeDriver starts a
new session. Thus WebDriver/ChromeDriver adds a driver compatibility boundary
without removing the prerequisite debug endpoint.
([ChromeDriver `debuggerAddress` limitations](https://developer.chrome.com/docs/chromedriver/help/operation-not-supported-when-using-remote-debugging),
[ChromeDriver capabilities](https://developer.chrome.com/docs/chromedriver/capabilities))

For this Go product's small, read-only protocol surface, WebDriver and BiDi do
not provide enough consumer-distribution benefit to justify another executable
or runtime adapter today. Keep them as test options, not the production data
source.

### External, Web Store, and enterprise extension installation

For normal users, Chrome's supported extension distribution path is a signed
Chrome Web Store item. A CLI cannot accept the install consent on the user's
behalf. External registration on Windows and macOS must point to a Chrome Web
Store item and Chrome still asks the user to enable it. Self-hosting is limited
to managed environments on those platforms.
([Chrome extension distribution](https://developer.chrome.com/docs/extensions/how-to/distribute),
[Chrome alternative installation methods](https://developer.chrome.com/docs/extensions/how-to/distribute/install-extensions))

Enterprise policy can install extensions silently, but only an administrator
of a managed Chrome environment should use it. On Windows and macOS,
force-installing an extension from outside the Web Store additionally requires
domain/MDM/Chrome Enterprise management. This is a valid enterprise deployment
option, not a consumer installer trick.
([Chrome Enterprise `ExtensionInstallForcelist`](https://chromeenterprise.google/policies/extension-install-forcelist/))

### Native Messaging

Native Messaging is extension-to-native IPC. Chrome accepts calls only from
allowed extension origins, and `runtime.connectNative()` is available only to
an extension page or service worker that declares `nativeMessaging`. Registering
a native host by itself cannot make the CLI discover or control a browser tab.
([Chrome Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging))

Therefore the current native host remains useful only with the optional
extension bridge. It is not part of the zero-extension default.

## Current open-source patterns

No popularity count is used in this decision; repository star and release
figures are intentionally omitted because they are volatile and do not alter
Chrome's security boundary.

| Project | Current first-party pattern | Implication for `coupangctl` |
| --- | --- | --- |
| [Chrome DevTools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp) | Starts dedicated persistent Chrome by default. Its ordinary-profile auto-connect mode requires Chrome 144+, manual remote-debugging enablement, and a Chrome approval dialog. | Strongest current evidence for dedicated-profile default plus a separately disclosed power-user current-browser mode. Its broad debugging surface is larger than this product needs. |
| [Playwright MCP](https://github.com/microsoft/playwright-mcp) | Uses a persistent separate profile by default. Current-tab reuse uses either the Playwright extension or an explicitly enabled CDP connection. | Confirms that even a large browser stack cannot skip Chrome's profile/extension/debug-consent boundary. Adding Playwright would add a runtime and browser-management layer without solving distribution. |
| [agent-browser](https://github.com/vercel-labs/agent-browser#chrome-profile-reuse) | Downloads Chrome for Testing for its normal managed-browser path. Its named ordinary-profile mode copies the Chrome profile to a temporary directory; its persistent-path mode logs in once and reuses a dedicated profile. | The persistent-path idea is relevant. Profile copying, saved storage-state files, and downloaded Chrome for Testing do not meet this repository's no-session-copy and low-dependency requirements. |
| [Browser Use](https://github.com/browser-use/browser-use) | Provides a Python agent framework, real-profile reuse, and optional profile synchronization/cloud browsers. | Useful as an agent framework reference, but it adds Python/model/cloud concerns and permits session transfer patterns outside this product's local typed-data boundary. |
| [chromedp](https://github.com/chromedp/chromedp) | Go CDP client that starts headless Chrome by default and can connect to an already debuggable browser through `RemoteAllocator`; its launcher creates a temporary user-data directory when none is supplied. | Could be compiled into one Go binary, but cannot bypass Chrome 136+ or the need for a debuggable process. The existing smaller `coupangctl` CDP adapter should remain unless maintenance evidence justifies replacing it. |
| [Rod](https://github.com/go-rod/rod) | Higher-level Go driver directly based on CDP, with browser discovery/download and automation helpers. | Useful for testing and prototyping; its larger general-purpose surface is unnecessary for fixed, allowlisted read operations and does not change browser attachment rules. |

Chrome for Testing is intentionally a versioned, non-auto-updating automation
browser. Bundling or downloading it would make the application larger and give
the product responsibility for browser security updates. It should be used in
CI, not as the primary consumer browser when a supported installed Chrome is
available.
([Chrome for Testing rationale](https://developer.chrome.com/docs/automation-and-testing/chrome-for-testing),
[Chrome for Testing availability API](https://github.com/GoogleChromeLabs/chrome-for-testing))

## Recommended product modes

| Mode | User action | Extension | Automation after login | Scope |
| --- | --- | --- | --- | --- |
| `dedicated` (default) | Complete first login/renewal in a visible product-owned Chrome profile | No | Yes, non-visible reads; headed only when explicitly requested | Only the product-owned profile and allowlisted read adapters |
| `current` (power user) | Enable Chrome remote debugging once and approve each new attachment | No | Only while the approved debugging connection remains available | Broad access to the selected Chrome profile; disclose prominently |
| `extension` (compatibility) | Install a signed Web Store extension and explicitly select/connect a tab | Yes | According to the extension's consent and permission tier | Narrow selected-tab access; useful when dedicated Chrome is rejected |
| `import` (server/browserless) | Transfer a normalized export created by the desktop CLI | No browser on server | Analytics only; no live refresh | Versioned typed records, never browser state |

The default UX should be closer to:

```text
coupangctl auth login   # opens Chrome once; user signs in
coupangctl sync         # automatic thereafter
coupangctl recap
```

It should not begin with `browser-bridge install`, `chrome://extensions`, a
downloaded automation browser, or an agent-specific skill.

## Desktop and server boundary

The desktop owns authentication and source acquisition. It may export only a
documented, versioned, normalized data bundle whose provenance and date range
are explicit. The server imports that bundle and runs analytics or renders the
recap without Chrome.

Do not export or import a Chrome profile, cookies, local/session storage,
DevTools endpoints, request headers, HAR files, or raw order responses. A
server that must refresh live data needs its own interactive desktop/browser
session and its own product-owned profile; a browserless server cannot inherit
the user's local Chrome authentication without introducing a credential/session
relay.

This boundary makes the common consumer path simple while keeping Linux server
support honest:

- desktop: authenticate, perform allowlisted reads, normalize, store/export;
- server: import normalized records, analyze, render, schedule browserless jobs;
- no hidden browser or credential tunnel between them.

## Implementation order

1. Make `dedicated` the implicit browser source and remove extension setup from
   the first-run path and default README quick start.
2. Use one private persistent Chrome profile for both human authentication and
   later read-only sync. Deprecate copying browser session state into a separate
   application session file.
3. Add a per-profile lock, browser-family/version metadata, ephemeral CDP port,
   bounded lifecycle, allowlisted navigation, and typed failure codes.
4. Run headlessly only after authentication exists. On access denial, return a
   typed result; a visible-browser attempt requires a separate explicit user
   request. Do not add stealth flags, fingerprint spoofing, challenge replay,
   or unbounded retries.
5. Keep `current` experimental until the official Chrome 144+ auto-connect
   consent flow and repeated order reads pass clean-profile live validation.
   Keep its broad-data warning distinct from the narrower extension disclosure.
6. Retain the Web Store extension and Native Messaging bridge as an optional
   compatibility adapter. Never ask ordinary users to load it unpacked.
7. Keep normalized export/import as the only browserless-server path.

## Release gates

- A new user with supported Chrome installed completes first authentication
  without installing an extension or another runtime.
- A later `sync` automatically reuses only the product-owned profile and does
  not expose or copy its browser session state.
- macOS, Windows, and Linux reject concurrent use of the profile with a stable
  `profile_in_use` result.
- Headless rejection, challenge, reauthentication, disabled remote debugging,
  and unsupported Chrome versions have distinct structured errors or readiness
  states.
- `current` mode cannot attach before Chrome's own opt-in and approval and
  never implies unattended operation.
- The default distribution contains no Playwright, Node, ChromeDriver, Chrome
  for Testing, Orca, or unpacked extension installation step.
- Server exports contain only normalized versioned records and pass tests that
  reject browser/session artifacts and raw source payloads.

## Bottom line

The consumer-friendly answer is not to find a hidden way to inject the existing
extension. It is to stop making the extension the default. A product-owned
persistent profile launched in the user's installed Chrome is the only current
architecture that is simultaneously extension-free, single-binary,
cross-platform, browser-owned, and automatable after one human login. Current
ordinary Chrome can be reused without an extension only through Chrome's
explicit remote-debugging opt-in and approval flow; anything claiming to do so
silently is either copying session state, relying on unmanaged injection, or
operating outside Chrome's supported security model.
