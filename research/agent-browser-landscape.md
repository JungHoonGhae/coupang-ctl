# Agent browser and headless-automation landscape

Validated: 2026-09-04 (Asia/Seoul)

## Decision

Keep the existing native Go browser path as the production default:

1. Open an installed Chrome-family browser visibly only for initial login,
   renewal, a source challenge, or explicit `--headed` use.
2. Reopen the same locked, product-owned profile headlessly for bounded reads.
3. Prefer source-native JSON obtained through the browser session, validate it
   in narrow Coupang adapters, and expose only typed core results.
4. Keep current-browser attachment an explicit Chrome-controlled consent mode.
5. On a non-desktop Linux/server deployment, import normalized typed records;
   do not upload or relay profiles, cookies, storage state, or raw payloads.

No reviewed agent framework, MCP browser, crawler, or hosted-browser service
improves that trust boundary enough to replace the current adapter. The best
bounded experiment is **Vercel `agent-browser` on synthetic/public fixtures**,
for its native JSON/diagnostic surface. It must remain a development tool, not
a shipped sidecar. Playwright and Lightpanda are reasonable compatibility-test
backends, also without live authentication.

That is a runtime decision, not a rejection of every capability in those
projects. `coupangctl` should selectively reproduce or reuse the parts that
strengthen authorized access without widening the trust boundary:

- one long-lived, user-approved browser connection and durable dedicated
  profile;
- headed authentication followed by quiet background reads;
- allowlisted XHR/fetch observation, response-shape validation, and immediate
  reduction to typed domain records;
- bounded retries, rate limits, backoff, and explicit challenge handoff;
- JSON diagnostics and offline selector-drift suggestions over synthetic or
  redacted fixtures; and
- cross-engine fixture testing where it exposes accidental browser assumptions.

These capabilities may be implemented directly in the Go adapter or exercised
by development-only tools. Adopting a useful mechanism does not require
shipping the framework that demonstrated it.

Hosted profiles and live views make remote human handoff convenient, but they
move browser state and page data to another operator. That is a different
privacy product, not an implementation detail. Browserbase, Steel, Browserless,
Browser Use Cloud, Hyperbrowser, and Lightpanda Cloud are therefore rejected for
authenticated Coupang access unless that boundary is separately designed and
approved.

This review does **not** evaluate CAPTCHA solving, challenge bypass, fingerprint
spoofing, proxy rotation, or bot-detection evasion. Those capabilities are not
adoption benefits and must remain disabled/out of scope. A challenge produces a
typed stop for normal user action.

## Baseline and evidence rules

The adapter at the time of this research supplied the narrow capabilities below.
These links preserve that historical CDP baseline; the current Camofox adapter is
documented in the repository README:
an ephemeral loopback CDP endpoint, a persistent `0700` profile, a cross-platform
profile lock and browser-family marker, explicit headed and background
constructors, allowlisted URLs, bounded message/body sizes, and JSON validation
([historical browser adapter](https://github.com/JungHoonGhae/coupang-ctl/blob/57f88a5/internal/browser/cdp.go),
[historical profile identity](https://github.com/JungHoonGhae/coupang-ctl/blob/57f88a5/internal/browser/profile_identity.go),
[typed order parser](../internal/coupang/orders/parser.go)). Its source interface
remains below the service/core boundary rather than leaking a general browser API
([order service](../internal/orders/service.go)).

Accordingly, “typed” has three distinct meanings:

- **Observed:** a source-native structured field whose path, type, and semantics
  the Coupang adapter validates.
- **Transport-typed:** a JSON/CDP/SDK envelope. This reduces protocol mistakes but
  says nothing about the commerce meaning of a field.
- **Inferred:** a model or adaptive selector filled a requested schema. Schema
  validation does not promote that value to observed evidence.

The last two remain behind the provenance boundary in
[`PRODUCT_PRINCIPLES.md`](../PRODUCT_PRINCIPLES.md).

Default-branch source snapshots observed on 2026-09-04: Chrome DevTools MCP
1.8.0 at
[`401a119`](https://github.com/ChromeDevTools/chrome-devtools-mcp/tree/401a1192b6e745a360c2a011a4bb69a67bf2af7a),
Playwright `1.64.0-next` at
[`d1dcd6b`](https://github.com/microsoft/playwright/tree/d1dcd6bc0a138ec0fd943df19e07458dc426ee22),
Playwright MCP 0.0.80 at
[`8a13ef8`](https://github.com/microsoft/playwright-mcp/tree/8a13ef8e9f7385a0f89477922127f31cbfde9761),
Browser Use 0.13.8 at
[`5b50d1f`](https://github.com/browser-use/browser-use/tree/5b50d1f511189c9df93e7f0bcb9da943b2d5780b),
Stagehand TypeScript SDK 4.0.2 at
[`d4f16a9`](https://github.com/browserbase/stagehand/tree/d4f16a98a5061279bed997b98fd3f0c17334eedb),
`agent-browser` 0.36.0 at
[`4a98df7`](https://github.com/vercel-labs/agent-browser/tree/4a98df79bd232fcde5ca3a4a48e1337b8108b160),
Steel 0.5.3 at
[`2b41124`](https://github.com/steel-dev/steel-browser/tree/2b41124d8e2953b0afe355c534e3c9aa71edae26),
Browserless 2.56.2 at
[`d036070`](https://github.com/browserless/browserless/tree/d03607075f6909111159504a3e23255702beabe1),
Hyperbrowser Python SDK 1.4.1 at
[`e18265c`](https://github.com/hyperbrowserai/python-sdk/tree/e18265cd8feb4380fa9bb52803a60ca11979d297),
Lightpanda `1.0.0-dev` at
[`1d66b2f`](https://github.com/lightpanda-io/browser/tree/1d66b2fbc8e487ba65495b66b8ac277f7d0d4f4c),
Crawlee JS 4.0.0 at
[`fde8d84`](https://github.com/apify/crawlee/tree/fde8d84c46a2fbe6dfd71baa23ccd29b9d160999),
and Scrapling 0.4.15 at
[`1f40d9b`](https://github.com/D4Vinci/Scrapling/tree/1f40d9b11b531ea46e9a2f9447f8757e4a71621c).
Each hash matched the repository's `HEAD` during validation. These are reviewed
source snapshots, not promises about future releases or cloud service behavior.
The commit is the freshness/evidence pin; adjacent versions are manifest labels,
not independently verified published-release tags. In particular, `next` and
`dev` explicitly denote development state. Hosted pricing was classified, not
audited, because it is volatile.

## Candidate comparison

| Candidate | Session reuse and human handoff | Structured/network path | Deployment, integration, license | Actual browser coverage | `coupangctl` verdict |
| --- | --- | --- | --- | --- | --- |
| **Chrome DevTools MCP** | Persistent dedicated profile by default; temporary isolation is optional. `--auto-connect` can inherit ordinary Chrome state only after Chrome 144+ remote debugging is enabled and the user approves the connection; `--headless` is a launch choice, not an in-process UI switch. ([configuration](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/401a1192b6e745a360c2a011a4bb69a67bf2af7a/docs/configuration.md), [Chrome consent flow](https://developer.chrome.com/blog/chrome-devtools-mcp-debug-your-browser-session)) | Lists network requests, returns request/response bodies, and can return JSON-serializable evaluation results. This is useful transport evidence but broader than the allowlisted document source. ([tool reference](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/401a1192b6e745a360c2a011a4bb69a67bf2af7a/docs/tool-reference.md)) | Node LTS + npm + installed current Chrome; Apache-2.0. Adding its Puppeteer/MCP server duplicates the Go process, CDP, and MCP lifecycles. ([package](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/401a1192b6e745a360c2a011a4bb69a67bf2af7a/package.json)) | Officially Chrome and Chrome for Testing; its `executablePath` option does not make other Chromium builds, Firefox, or WebKit supported. | **Do not embed. Adopt the pattern:** explicit consent, direct WebSocket attachment, and one reusable connection in the native Go path. |
| **Playwright + Playwright MCP** | Playwright supports persistent contexts and portable storage-state snapshots. MCP defaults to a separate persistent workspace profile; isolated storage state and an extension that attaches to an existing Chrome tab are alternatives. Headed/headless is selected at launch. ([contexts](https://playwright.dev/docs/api/class-browsercontext), [MCP profile modes](https://github.com/microsoft/playwright-mcp/blob/8a13ef8e9f7385a0f89477922127f31cbfde9761/README.md)) | Excellent request/response events and `response.json()`. MCP also exposes accessibility snapshots and network tools, but its general tool output is not a Coupang DTO. ([network](https://playwright.dev/docs/network), [response API](https://playwright.dev/docs/api/class-response)) | Apache-2.0. Official bindings are JS/TS, Python, Java, and .NET—not Go—and each release expects matching downloaded browser builds/system dependencies. MCP adds Node. ([languages](https://playwright.dev/docs/languages), [browser installation](https://playwright.dev/docs/browsers), [MCP package](https://github.com/microsoft/playwright-mcp/blob/8a13ef8e9f7385a0f89477922127f31cbfde9761/package.json)) | Real Chromium, Firefox, and WebKit support, plus branded Chrome/Edge channels. CDP attachment is Chromium-only and officially lower fidelity than Playwright's own version-matched protocol. ([browser type](https://playwright.dev/docs/api/class-browsertype)) | **Dev/QA only.** Valuable cross-engine test harness; no production sidecar or storage-state export. |
| **Browser Use** | Can launch a visible or headless local Chrome profile, attach by CDP, and use cloud profiles. Its local profile code may copy a named ordinary profile; its cloud workflow uploads/synchronizes browser state. Both conflict with the no-session-copy default. ([profile implementation](https://github.com/browser-use/browser-use/blob/5b50d1f511189c9df93e7f0bcb9da943b2d5780b/browser_use/browser/profile.py), [cloud profiles](https://github.com/browser-use/browser-use/blob/5b50d1f511189c9df93e7f0bcb9da943b2d5780b/CLOUD.md)) | Uses typed CDP bindings and Pydantic agent outputs, but model-produced structured output is inferred; it is not a network-first domain contract. ([CDP implementation](https://github.com/browser-use/browser-use/blob/5b50d1f511189c9df93e7f0bcb9da943b2d5780b/browser_use/browser/session.py), [agent result types](https://github.com/browser-use/browser-use/blob/5b50d1f511189c9df93e7f0bcb9da943b2d5780b/browser_use/agent/views.py)) | Python 3.11+, a large model/browser dependency set, optional hosted API; MIT. A Go integration would be a sidecar or remote service. ([package metadata](https://github.com/browser-use/browser-use/blob/5b50d1f511189c9df93e7f0bcb9da943b2d5780b/pyproject.toml)) | Current local control is CDP/Chrome-family, not Firefox/WebKit. | **Reject.** Wrong runtime, state-transfer options, and agent abstraction for bounded reads. |
| **Browserbase / Stagehand** | Stagehand v4 can launch visible/headless local Chrome, persist a local `userDataDir`, keep a process alive, or attach by CDP. Browserbase Contexts persist Chromium user data server-side; Live View lets a person watch and take control. ([browser configuration](https://github.com/browserbase/stagehand/blob/d4f16a98a5061279bed997b98fd3f0c17334eedb/packages/docs/v4/configuration/browser.mdx), [contexts](https://docs.browserbase.com/platform/browser/core-features/contexts), [live view](https://docs.browserbase.com/platform/browser/observability/session-live-view)) | `extract()` validates model output against Zod/Pydantic/Go-derived JSON Schema. That makes an inference typed, not observed. The caller-owned page API can read response bodies separately. ([extract](https://github.com/browserbase/stagehand/blob/d4f16a98a5061279bed997b98fd3f0c17334eedb/packages/docs/v4/basics/extract.mdx), [Go SDK](https://github.com/browserbase/stagehand/blob/d4f16a98a5061279bed997b98fd3f0c17334eedb/packages/sdk-go/README.md)) | Stagehand is MIT and now has first-party TypeScript, Python, and Go SDKs. The Go SDK lowers bridge cost, but local operation still adds Stagehand's browser-extension/model runtime; Browserbase adds API/model charges and cloud custody. ([package](https://github.com/browserbase/stagehand/blob/d4f16a98a5061279bed997b98fd3f0c17334eedb/packages/sdk-ts/package.json)) | Local and hosted paths are Chromium/CDP; no official Firefox/WebKit path was found. | **Reject for authenticated reads.** At most benchmark selector recovery on synthetic fixtures. |
| **Vercel `agent-browser`** | Native daemon is headless by default and supports explicit headed launch, durable custom profiles, save/restore state, and attachment to existing CDP browsers. Named ordinary Chrome profile reuse explicitly copies a snapshot, so only a dedicated profile path is acceptable even in experiments. ([README](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/README.md), [session management](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/skill-data/core/references/session-management.md)) | Strong `--json`, typed command/MCP arguments, request inspection, response bodies, and HAR support. Raw state/HAR surfaces require redaction and are not safe application results. ([command surface](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/skill-data/core/references/commands.md)) | Apache-2.0 Rust binary/daemon; npm is an installer wrapper, and normal managed-browser setup downloads Chrome for Testing. Go would supervise another executable and translate its schemas. ([package](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/package.json), [Rust manifest](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/cli/Cargo.toml)) | Chrome/Chromium over CDP, Lightpanda as a selectable engine, and a separate iOS/Safari WebDriver path; no desktop Firefox/WebKit support. | **Best dev-only experiment. Adopt the ergonomics:** bounded JSON diagnostics and request-shape reporting on synthetic/public pages; do not ship it or give it authenticated state. |
| **Steel** | Session APIs keep browser state while a session lives; hosted live sessions provide interactive viewing/control. This is useful remote handoff, but it exposes a broad session capability rather than the current local consent/profile boundary. ([repository](https://github.com/steel-dev/steel-browser/tree/2b41124d8e2953b0afe355c534e3c9aa71edae26), [live sessions](https://docs.steel.dev/overview/sessions-api/embed-sessions/live-sessions)) | REST quick actions can return rendered content/markdown and clients can connect by CDP using Playwright, Puppeteer, or Selenium. None is automatically source-native evidence. | Apache-2.0, public beta, Node 22 or a Linux amd64/arm64 Docker service; cloud is optional and SDKs include Go. Self-hosting reduces vendor lock-in but not service/SBOM/operations cost. ([package](https://github.com/steel-dev/steel-browser/blob/2b41124d8e2953b0afe355c534e3c9aa71edae26/package.json), [Dockerfile](https://github.com/steel-dev/steel-browser/blob/2b41124d8e2953b0afe355c534e3c9aa71edae26/Dockerfile)) | The managed engine is Chrome/Chromium. Accepting multiple client libraries, including Selenium, is not cross-browser engine support. | **Reject.** Duplicates the local browser lifecycle and expands the authenticated control surface. |
| **Browserless** | Can briefly disconnect/reconnect to the same live browser; longer-lived state uses persistent sessions/profiles or mounted data in self-hosting. Hosted debugging/profile features change browser-state custody. ([reconnect](https://docs.browserless.io/examples/reconnect), [open-source deployment](https://docs.browserless.io/enterprise/open-source)) | CDP, Puppeteer, Playwright, and REST extraction endpoints are available. A Browserless-specific CDP extension manages reconnect, but source JSON still needs the same Coupang parser. ([CDP extensions](https://docs.browserless.io/api-reference/cdp-extensions)) | Self-hosted Docker or paid cloud. The server package is SSPL, not Apache/MIT/BSD, and carries Node plus browser images; redistribution/self-host terms need legal review. ([package](https://github.com/browserless/browserless/blob/d03607075f6909111159504a3e23255702beabe1/package.json)) | Actual Chrome, Chromium, Edge, Firefox, and WebKit images. Firefox/WebKit use Playwright transport because they do not speak CDP. ([deployment matrix](https://docs.browserless.io/enterprise/open-source)) | **Reject.** Strong service functionality, but wrong distribution, license-review, and state-custody shape. |
| **Hyperbrowser** | Cloud browser sessions expose a CDP WebSocket. The SDK exposes server-created profiles, `persist_changes`, session records, and expiring live-view access; no official self-hosted browser-runtime path was found. ([SDK](https://github.com/hyperbrowserai/python-sdk/blob/e18265cd8feb4380fa9bb52803a60ca11979d297/README.md), [profile manager](https://github.com/hyperbrowserai/python-sdk/blob/e18265cd8feb4380fa9bb52803a60ca11979d297/hyperbrowser/client/managers/sync_manager/profile.py), [session types](https://github.com/hyperbrowserai/python-sdk/blob/e18265cd8feb4380fa9bb52803a60ca11979d297/hyperbrowser/types/session.py)) | SDK request/response models are typed, and hosted AI extraction accepts a JSON schema. Schema-shaped AI extraction remains inferred. ([SDK metadata](https://github.com/hyperbrowserai/python-sdk/blob/e18265cd8feb4380fa9bb52803a60ca11979d297/pyproject.toml), [AI extract](https://www.hyperbrowser.ai/docs/web-scraping/extract)) | SDK is MIT; the browser service requires an account/API key and is the deployment. No first-party Go SDK/self-host evidence was found, so Go uses REST or a supported-language client. | Official examples attach Playwright's `chromium` client over CDP; no alternative engine evidence was found. | **Reject.** Maximum cloud lock-in and remote authenticated-data custody with no needed capability gain. |
| **Lightpanda** | Sessions have isolated cookies/memory, but the engine is deliberately headless and provides no graphical login window. No durable Chrome-profile-compatible authentication/handoff contract was found. ([Python session API](https://lightpanda.io/docs/reference/python), [agent UI contract](https://lightpanda.io/docs/usage/agent)) | CDP plus native HTML/markdown/semantic-tree/schema extraction are attractive for public reads. Selector/schema output is not source-native API evidence, and no pixel-accurate rendering means visual fallback differs materially from Chrome. ([CLI extraction](https://lightpanda.io/docs/reference/cli/fetch), [rendering limits](https://lightpanda.io/blog/posts/what-is-a-true-headless-browser)) | Local Zig binary, Docker, or paid cloud; Linux binaries require glibc, macOS is available, and Windows requires WSL. The browser is AGPL-3.0-only and nightly/`1.0.0-dev`, expanding release/license review. ([install matrix](https://github.com/lightpanda-io/browser/blob/1d66b2fbc8e487ba65495b66b8ac277f7d0d4f4c/README.md), [licensing](https://github.com/lightpanda-io/browser/blob/1d66b2fbc8e487ba65495b66b8ac277f7d0d4f4c/LICENSING.md)) | A genuine non-Chromium, non-WebKit browser written in Zig with V8 and partial CDP, WebDriver BiDi, and Web API support; Playwright/Puppeteer connect through Chromium CDP mode. Compatibility is not equivalence. | **Public/synthetic experiment only.** It cannot satisfy headed login and must prove each required protocol/Web API behavior. |
| **Crawlee** | Browser pools can run headless or visible and manage cookie-bearing sessions; PlaywrightCrawler can use a persistent user-data directory. It has crawler session rotation, not a consumer login-consent/handoff product. ([Python browser options](https://crawlee.dev/python/api/next/class/PlaywrightCrawler), [JS session management](https://crawlee.dev/js/docs/next/guides/session-management)) | Outputs datasets as JSON and offers browser/raw-HTTP crawling. JSON storage is not evidence that values came from stable structured endpoints. ([Python quick start](https://crawlee.dev/python/docs/quick-start)) | JS 4.0.0 requires Node 22 and separately installed Playwright/Puppeteer browsers; a separate Python implementation exists. Apache-2.0 and self-hostable, but Go integration is a large sidecar. ([package](https://github.com/apify/crawlee/blob/fde8d84c46a2fbe6dfd71baa23ccd29b9d160999/packages/core/package.json)) | Through Playwright: Chromium, Firefox, WebKit, and Chrome/Edge channels; through Puppeteer: Chrome-family. | **Reject.** Excellent crawl orchestration, but `coupangctl` performs bounded account reads, not a scalable crawl. |
| **Scrapling** | Browser sessions can reuse a supplied profile or attach over CDP; visible/headless launch is available. | Can parse JSON and capture XHR, while adaptive HTML matching is only an inferred drift hint. | Python 3.10+, Playwright/Patchright/browser assets; BSD-3-Clause. Shipping it creates a second runtime and browser lifecycle. ([metadata](https://github.com/D4Vinci/Scrapling/blob/1f40d9b11b531ea46e9a2f9447f8757e4a71621c/pyproject.toml), [detailed review](scrapling.md)) | Ordinary dynamic path is Chromium/installed Chrome. Firefox-like stealth paths are outside this evaluation and are not counted. | **Do not ship the sidecar. Adopt the primitives:** allowlisted XHR/JSON capture in Go and offline synthetic/redacted selector-drift diagnostics. |

## Protocol choice

| Protocol/control plane | Verified properties | Product consequence |
| --- | --- | --- |
| **CDP** | Chrome's command/event protocol uses JSON over WebSocket. The Network domain can return response bodies, but tip-of-tree changes have no backwards-compatibility guarantee. ([protocol](https://chromedevtools.github.io/devtools-protocol/), [Network domain](https://chromedevtools.github.io/devtools-protocol/1-3/Network/)) | Best fit now: the necessary surface already exists in Go and reaches Chrome/Edge/Chromium directly. Keep method use small, bounded, and adapter-owned. |
| **WebDriver BiDi** | A W3C Working Draft defines bidirectional JSON messaging and event subscription on a WebDriver session. It is intended as a cross-browser standard, but session creation/control is not ordinary-browser discovery or user consent. ([18 August 2026 Working Draft](https://www.w3.org/TR/2026/WD-webdriver-bidi-20260818/)) | Track for interoperability tests. It does not remove a driver/session boundary, profile custody decision, or headed login design, so replacing the working CDP adapter has no present payoff. |
| **Playwright protocol/API** | Playwright controls Chromium, Firefox, and WebKit through its own version-coupled client/server stack. `browserType.connect()` requires matching major/minor versions; `connectOverCDP()` is Chromium-only and lower fidelity. ([browser type](https://playwright.dev/docs/api/class-browsertype)) | Best cross-engine QA option, not a wire standard or Go-core dependency. Shipping it adds a supported-language runtime plus matching browser assets. |

“CDP-compatible” must not be read as “Chrome-compatible.” Steel, Hyperbrowser,
Chrome DevTools MCP, Browser Use, and the normal Stagehand path still run a
Chrome-family engine. Browserless and Playwright genuinely offer Firefox/WebKit
backends through Playwright's transport. Lightpanda is the only reviewed new
engine presenting both CDP and WebDriver BiDi surfaces, and its
headless/rendering/Web API differences require capability-by-capability proof
before even public-data use.

## Deployment and Go-core impact

| Shape | Desktop | Headless Linux/server | Go-core cost |
| --- | --- | --- | --- |
| Existing `coupangctl` CDP | One Go executable plus installed Chrome-family browser; visible login and later background reads share the dedicated profile. | A browser can run, but without an interactive desktop there is no acceptable initial-auth handoff. Use normalized-record import instead. | **Low/already paid.** Narrow interfaces and typed parsers exist. |
| Node/Python framework or MCP | Adds runtime, package graph, browser downloads, process supervision, and a second output schema. | Usually container-friendly; headed work needs Xvfb or a remote live-view service. A Deno-like/self-contained JS executable does not remove the browser and OS-library payload. | **High.** Sidecar IPC plus duplicated security, release, SBOM, and lifecycle work. |
| Native helper (`agent-browser`, Lightpanda) | Easier subprocess packaging than Node/Python, but still another signed/checksummed executable and often another browser. | Operationally viable on supported Linux/glibc/container targets. It still cannot receive local auth state under the product rules. | **Medium/high.** Stable JSON IPC is possible; semantic adapters and lifecycle remain in Go. |
| Hosted browser | No local browser package and often excellent interactive live view. | Easy to call from any server runtime. | **Medium technically, disqualifying architecturally:** API integration is simple, but private pages, cookies/profile state, recordings, retention, region, access control, outage, and billing become external dependencies. |
| Self-hosted browser service | Heavy for a consumer desktop; suited to shared infrastructure. | Docker/Kubernetes can host Chrome and expose CDP/REST. | **High operationally.** The team owns browser patching, sandboxing, capacity, service auth, observability, and release acceptance. |

The established one-binary decision remains sound
([language decision](language-ecosystem-decision.md),
[browser distribution decision](browser-distribution-alternatives.md)). A remote
Linux deployment should consume versioned normalized records produced on the
authenticated desktop. It must never become a tunnel for a browser profile,
cookie jar, storage-state file, OTP, or raw order response.

## Bounded experiments and gates

1. **`agent-browser` diagnostics:** use a pinned downloaded release only if its
   checksums, SBOM, and attestations can be verified. Run solely against
   synthetic/public pages. Compare its JSON/network metadata and domain policy
   behavior with the existing adapter; never save cookies, HAR containing
   private traffic, or an ordinary-browser profile.
2. **Cross-engine fixture QA:** use pinned Playwright browsers against local
   synthetic fixtures. Firefox/WebKit failures can reveal accidental
   browser assumptions, but do not create a production dependency.
3. **Lightpanda compatibility probe:** exercise only the exact CDP methods and
   Web APIs required by a public, unauthenticated read. Fail closed on any
   difference; do not infer Coupang compatibility from a successful generic
   page load.
4. **Offline drift diagnostics:** if selector maintenance becomes material,
   compare synthetic/redacted HTML revisions with Scrapling or Stagehand. A
   recovered selector is an inferred candidate requiring human semantic review
   and normal parser tests.

Reconsider production adoption only if the narrow Go adapter becomes measurably
more expensive than supervising the candidate, the candidate preserves local
browser-owned state without copying it, packaged artifacts pass the repository's
release-acceptance rules, and source-native JSON still crosses a small typed Go
adapter. Cloud execution additionally requires an explicit product/privacy
decision; convenience alone is insufficient.
