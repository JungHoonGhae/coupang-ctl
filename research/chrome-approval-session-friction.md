# Chrome approval and persistent browser sessions

Validated: 2026-09-04 (Asia/Seoul)

## Answer

There is no repository that can legitimately make a new CDP connection to the
current default Chrome profile without Chrome's consent prompt. Chrome 144+
requires remote debugging to be enabled and asks the user to allow an incoming
session. The official project has a closed, not-planned request to persist that
approval, and its issue tracker confirms that every new/retried connection can
produce another prompt
([Chrome setup](https://developer.chrome.com/docs/devtools/agents/use-cases/auto-connect),
[persistence request](https://github.com/ChromeDevTools/chrome-devtools-mcp/issues/825),
[reconnection behavior](https://github.com/ChromeDevTools/chrome-devtools-mcp/issues/1794)).

Repositories that feel less cumbersome use one of three different tradeoffs:

1. Keep one approved WebSocket alive in a long-running local daemon.
2. Launch an automation-owned Chrome with a separate persistent profile, so the
   default-profile auto-connect permission gate is not involved.
3. Install an extension and authorize that extension's own relay. This replaces
   Chrome's CDP prompt with an extension permission/token boundary; it does not
   provide extension-free silent access.

The practical default for `coupangctl` should therefore be a dedicated,
product-owned persistent profile. Current-Chrome mode should reuse one
long-lived connection and be described as an explicit compatibility mode.

## Compared implementations

| Project | What it actually reuses | Lifecycle | Current default Chrome without repeated approval? | Relevance to `coupangctl` |
| --- | --- | --- | --- | --- |
| [Chrome DevTools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp/tree/f215c82d3f41b00472f858b80a5cda1f56f7380f) | A dedicated persistent profile by default; `--autoConnect` can attach to the running default profile. | One MCP server owns its connection; a new server/reconnect is a new CDP connection. | **No.** Chrome asks for permission when auto-connect creates the debugging session. | Keep the native adapter, but copy the one-server/one-WebSocket lifetime. Do not spawn a one-shot current-browser process per CLI command. |
| [Browser Use](https://github.com/browser-use/browser-use/tree/fe5ad353091fa2ed5499b94e8fe21094bc2e9e5a) | Current CLI can attach to a running Chrome; its profile path also copies an ordinary Chrome profile into a temporary directory. | Current CLI uses one local background daemon and an already-live CDP client when available ([CLI](https://github.com/browser-use/browser-use/blob/fe5ad353091fa2ed5499b94e8fe21094bc2e9e5a/browser_use/cli.py), [CDP session](https://github.com/browser-use/browser-use/blob/fe5ad353091fa2ed5499b94e8fe21094bc2e9e5a/browser_use/browser/session.py), [profile copy](https://github.com/browser-use/browser-use/blob/fe5ad353091fa2ed5499b94e8fe21094bc2e9e5a/browser_use/browser/profile.py)). | **Not when attaching.** A long-lived daemon reduces prompts, but any fresh connection to Chrome's protected endpoint still crosses the same gate. Profile-copy mode avoids the prompt by launching another browser, not by attaching silently. | Adopt connection single-flight and liveness checks, not the Python agent layer or default-profile copy. |
| [Playwright](https://github.com/microsoft/playwright/tree/d1dcd6bc0a138ec0fd943df19e07458dc426ee22) / [Playwright MCP](https://github.com/microsoft/playwright-mcp/tree/8a13ef8e9f7385a0f89477922127f31cbfde9761) | `launchPersistentContext(userDataDir)` and MCP's default workspace profile are separate persistent profiles. CDP and extension modes can attach to existing Chrome. | Persistent profile survives process restarts; MCP can also run as a standalone HTTP server. | **No over CDP.** Playwright explicitly warns that automating Chrome's default user-data directory is unsupported; use a separate directory. **Yes only with its installed extension and profile token**, described below. | Confirms the dedicated-profile default and long-running local server design; adopting Playwright itself would add Node/browser runtime without removing Chrome consent. ([persistent context](https://github.com/microsoft/playwright/blob/d1dcd6bc0a138ec0fd943df19e07458dc426ee22/docs/src/api/class-browsertype.md), [MCP profiles](https://github.com/microsoft/playwright-mcp/blob/8a13ef8e9f7385a0f89477922127f31cbfde9761/README.md)) |
| [Vercel `agent-browser`](https://github.com/vercel-labs/agent-browser/tree/4a98df79bd232fcde5ca3a4a48e1337b8108b160) | A custom persistent profile, a temporary copy of a named normal Chrome profile, or an existing CDP browser. | Auto-started Rust daemon persists across commands, with a one-hour default idle shutdown; CDP sessions retain their bound tab across daemon restarts. | **Not for a fresh auto-connect.** The daemon can keep the first approved connection alive; named-profile reuse works by copying the profile and launching another Chrome. | Strongest implementation reference for a transparent local broker: auto-start, health/status JSON, tab ownership, idle cleanup, and reconnect policy. ([profiles and sessions](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/README.md#chrome-profile-reuse), [daemon architecture](https://github.com/vercel-labs/agent-browser/blob/4a98df79bd232fcde5ca3a4a48e1337b8108b160/README.md#architecture)) |
| [PinchTab](https://github.com/pinchtab/pinchtab/tree/a59828d59df32381a027eb77b0dbc5f1b7dc8523) | Server-managed persistent profiles and Chrome instances; optional external-CDP attach. | A user-level daemon owns profiles, instances, tabs, and bridge processes. Stopping an attached bridge leaves external Chrome running. | **Not when attaching to protected current Chrome.** It can preserve the approved bridge, but a fresh external CDP connection still triggers Chrome's gate. Its managed profiles avoid that gate. | Best Go reference for daemon installation, instance registry, domain allowlists, per-agent sessions, and lifecycle cleanup. Its broad browser API is too wide to embed directly. ([process model](https://github.com/pinchtab/pinchtab/blob/a59828d59df32381a027eb77b0dbc5f1b7dc8523/README.md#process-model), [CDP attach](https://github.com/pinchtab/pinchtab/blob/a59828d59df32381a027eb77b0dbc5f1b7dc8523/docs/reference/instances.md#attach-an-existing-browser-cdp), [security](https://github.com/pinchtab/pinchtab/blob/a59828d59df32381a027eb77b0dbc5f1b7dc8523/docs/guides/security.md)) |
| [Steel Browser](https://github.com/steel-dev/steel-browser/tree/2b41124d8e2953b0afe355c534e3c9aa71edae26) | A self-hosted or Steel-hosted managed Chromium session; Profiles API snapshots a browser user-data directory across sessions. | Long-running browser service/API rather than the user's ordinary local Chrome. | **Not applicable.** It avoids the dialog by controlling its own local/container/cloud Chrome, not by silently taking over the default profile. | Too much infrastructure and, for cloud, a different data-custody boundary. Useful only as a service-lifecycle reference. ([server model](https://github.com/steel-dev/steel-browser/tree/2b41124d8e2953b0afe355c534e3c9aa71edae26), [profile persistence](https://docs.steel.dev/overview/profiles-api/overview)) |
| [OOMOL OpenConnector](https://github.com/oomol-lab/open-connector/tree/e00560bbecc71074924e6d637156ba4dba177505) | Durable OAuth/API credentials behind a connector gateway; optional provider Actions may call hosted browser services. | Long-lived auth/action runtime exposed through CLI, MCP, HTTP, and OpenAPI. | **No.** It is not a local Chrome session broker and does not remove Chrome's debugging consent. | Reuse its durable connection/error-contract pattern, but not as the Coupang browser transport. ([architecture](https://github.com/oomol-lab/open-connector/blob/e00560bbecc71074924e6d637156ba4dba177505/README.md#how-it-works), [credentials](https://github.com/oomol-lab/open-connector/blob/e00560bbecc71074924e6d637156ba4dba177505/docs/credentials.md)) |

## The extension exception

Microsoft's [Playwright Chrome Extension](https://github.com/microsoft/playwright/blob/d1dcd6bc0a138ec0fd943df19e07458dc426ee22/packages/extension/README.md)
connects to existing tabs and their logged-in state. It normally presents its
own approval screen, but documents a profile-specific
`PLAYWRIGHT_MCP_EXTENSION_TOKEN`; after explicit setup, that token permits later
connections without approving them one by one. This is a legitimate way to
avoid the Chrome 144 CDP dialog because the extension is the authorized browser
principal. It has different costs:

- the user must install and trust a Chrome Web Store extension;
- the token grants broad browser access and must be stored like a credential;
- the extension path can affect the user's visible browser. A current upstream
  issue reports that token-based unattended attachment can activate a tab and
  focus its window
  ([focus issue](https://github.com/microsoft/playwright/issues/42343)); and
- it does not satisfy an extension-free default installation.

[BrowserMCP](https://github.com/browsermcp/mcp/tree/9db12f2b4f61294f0bc11708986abc47db539d6c)
uses the same broad pattern—an MCP server plus Chrome extension over the
existing profile—but its repository says it cannot currently be built by
itself because required monorepo packages are missing. It is therefore a weaker
reference than Playwright's maintained extension.

## Recommended `coupangctl` design

1. **Default: managed profile.** Keep one product-owned profile, show Chrome
   only for login/challenge, then reuse the same profile for quiet background
   work. Chrome itself recommends a custom user-data directory for remote
   debugging rather than the default profile
   ([Chrome security change](https://developer.chrome.com/blog/remote-debugging-port)).
2. **One local session broker.** Add or retain an auto-started per-user daemon
   that owns one browser/CDP connection, exposes only typed `coupangctl`
   operations, uses owner-only local IPC, serializes reconnect, and reports
   health without printing endpoints or session material.
3. **Current Chrome: explicit compatibility mode.** Ask once when the broker
   first attaches, then keep that WebSocket alive across CLI and MCP calls.
   Chrome restart, broker restart, socket loss, or reconnection can require
   another approval; the UI and `doctor` output must say so before starting.
4. **Do not loop on denial.** One connection attempt should produce at most one
   pending consent request. Concurrent callers wait on the same single-flight
   attach instead of opening parallel sockets—the repeated-popup failure mode
   documented upstream.
5. **Optional extension only if demand justifies it.** A signed Web Store
   extension plus a revocable profile-scoped token can remove recurring attach
   prompts for users who choose it. It should not be required for normal use and
   should remain narrower than Playwright's full-browser surface.

This design reduces the normal flow to one login in the managed profile. For a
user who insists on controlling the already-open default Chrome profile, one
approval per broker/browser connection lifetime is the minimum legitimate
extension-free interaction available today.

## Implemented in coupangctl

The recommended local-broker design is implemented behind the typed core and
the existing CLI/MCP adapters. Independent processes discover one auto-started
broker through an owner-private descriptor; the descriptor's random token and
loopback address never appear in product responses. Initial attachment is
single-flight, independent reads may overlap, confirmed cart mutation and
session replacement are exclusive, and the broker stops after one hour idle or
an explicit `coupangctl current-browser stop`. Its HTTP surface contains only
the allowlisted coupangctl document operations and deliberately has no generic
navigate, evaluate, screenshot, or browser-control method.
