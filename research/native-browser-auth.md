# Native browser authentication research

Validated: 2026-09-01 (Asia/Seoul)

## Decision

Ship authentication as a browser-owned workflow with two human challenge
presentations and a separate read-only consumption phase:

1. Default QR login launches an installed, headed Chrome-family browser with a
   persistent profile owned only by `coupangctl`. A short-lived loopback CDP
   adapter navigates only to the protected order URL, selects the QR tab, and
   polls the final location. It does not read the QR value, verification number,
   cookies, storage, form fields, or network bodies.
2. `auth login --phone` opens the same dedicated profile in a visible browser.
   The adapter may enter the supplied phone number, click the visible request
   button, and enter one OTP supplied interactively by the user. It never calls
   the send/verify endpoint directly, retries an inconclusive submission, or
   solves CAPTCHA; any challenge remains a human browser action.
3. After a successful login, Chrome keeps its own session state inside that
   dedicated persistent profile. A later `auth status` or read-only sync opens
   the same profile in a short-lived browser process with an ephemeral,
   loopback-only DevTools port, opens an allowlisted read-only page, extracts
   documented structured data, and shuts down the process. `coupangctl` does
   not export or inject a second cookie/session file. CDP objects, cookies, and
   response bodies never cross into the typed core.
4. An explicit `orders sync --current-browser` mode may attach to Chrome 144+
   only after the user enables remote debugging in Chrome and approves the
   connection. It creates and closes only its own target, does not close the
   user's browser, and does not copy browser state. Because Chrome grants a
   debugger broad profile access, this remains an experimental power-user mode,
   not an unattended default.

The default distributed path needs no Orca, Playwright, accessibility
controller, agent skill, or user-installed extension at runtime. It also does not claim to make
automation undetectable. If Coupang rejects the read-only sync browser, the CLI
must stop with a structured error; it must not add stealth flags, spoof browser
signals, replay challenge endpoints, or copy session tokens from another
profile. The selected-tab extension is retained only as an optional compatibility
adapter.

## Why the profile must be dedicated

Chromium documents that the user-data directory contains cookies, history,
bookmarks, and other local state, that `--user-data-dir` overrides the normal
location on desktop platforms, and that two running Chrome instances cannot
share one user-data directory. It also documents different default paths by OS,
brand, and release channel. Therefore, an everyday profile is neither a safe
input nor a portable artifact for `coupangctl`.
([Chromium user-data directory documentation](https://chromium.googlesource.com/chromium/src/+/master/docs/user_data_dir.md))

Since Chrome 136, Google Chrome ignores `--remote-debugging-port` and
`--remote-debugging-pipe` for the default user-data directory. Remote debugging
requires a non-standard `--user-data-dir`; Google says that directory uses a
different encryption key and recommends it to isolate debugging from real
profiles. This independently reinforces the dedicated-profile boundary.
([Chrome remote-debugging security change](https://developer.chrome.com/blog/remote-debugging-port))

Chrome also warns that profiles are not backwards-compatible and that sharing
a profile between mismatched browser versions or channels can cause crashes or
data loss. Persist the selected browser family/channel with profile metadata,
reject a mismatched executable, and never place the profile on a network share.
([Chrome supported directory variables](https://support.google.com/chrome/a/answer/9866158))

Recommended per-user roots are:

| OS | Persistent `coupangctl` state root |
| --- | --- |
| macOS | `~/Library/Application Support/coupangctl/` |
| Windows | `FOLDERID_LocalAppData\coupangctl\` |
| Linux | `$XDG_STATE_HOME/coupangctl/`, defaulting to `~/.local/state/coupangctl/` |

These locations follow the platform-owned definitions for application support,
per-user local application data, and persistent application state.
([Apple Application Support](https://developer.apple.com/documentation/foundation/url/applicationsupportdirectory),
[Windows known folders](https://learn.microsoft.com/en-us/windows/win32/shell/knownfolderid),
[XDG Base Directory specification](https://specifications.freedesktop.org/basedir/))

On POSIX, create the application and profile directories with mode `0700` and
verify they are owned by the current user. The XDG specification explicitly
requires `0700` when an application creates a missing destination directory.
On Windows, create the profile below `FOLDERID_LocalAppData` and preserve or
restrict its user ACL; Node's `mkdir` mode is not supported on Windows, so a
POSIX mode argument is not a Windows security control.
([XDG directory creation rule](https://specifications.freedesktop.org/basedir/),
[Node.js file-system modes](https://nodejs.org/api/fs.html),
[Windows file security and ACL inheritance](https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights))

The profile is sensitive browser state. Normal output must never reveal its
contents or copy cookies out of it. Backup, export, support bundles, and MCP
responses must exclude it. A profile reset should be an explicit, confirmed
operation and should explain that it signs the user out.

## Browser discovery and launch

Use this deterministic discovery order:

1. An explicit `--browser-path` or configuration value.
2. OS-native discovery.
3. A small documented vendor-path and `PATH` fallback list.
4. `browser_not_found` with install and override instructions.

Validate that the result is a regular executable and obtain its version before
launch. Cache only browser kind, version, and discovery source; do not emit a
home-directory-bearing profile path in normal output.

- **macOS:** resolve known bundle identifiers with
  `NSWorkspace.urlForApplication(withBundleIdentifier:)`, then resolve the
  executable through `Bundle.executableURL`. Apple exposes arguments as an
  array through `NSWorkspace.OpenConfiguration`, so no shell parsing is needed.
  ([NSWorkspace bundle lookup](https://developer.apple.com/documentation/appkit/nsworkspace/urlforapplication(withbundleidentifier:)),
  [bundle executable URL](https://developer.apple.com/documentation/foundation/bundle/executableurl),
  [open configuration arguments](https://developer.apple.com/documentation/appkit/nsworkspace/openconfiguration/arguments))
- **Windows:** query per-user and machine `App Paths` registrations for
  `chrome.exe` and `msedge.exe`, then try documented vendor locations.
  Microsoft calls App Paths the preferred executable registration mechanism.
  Launch with the resolved absolute path; `CreateProcessW` warns that an
  unqualified, unquoted path containing spaces can select the wrong executable.
  ([Windows App Paths](https://learn.microsoft.com/en-us/windows/win32/shell/app-registration),
  [CreateProcessW security note](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw))
- **Linux:** inspect desktop entries and honor `TryExec`; parse `Exec` according
  to the Desktop Entry specification, then try common executable names on
  `PATH`. Do not pass an `Exec` string through a shell.
  ([Desktop Entry specification](https://specifications.freedesktop.org/desktop-entry/latest-single/))

All platforms should launch the resolved executable directly with an argv
array. For the manual phase, the minimal arguments are the absolute dedicated
`--user-data-dir` and the login URL. Do not add remote-debugging, headless,
automation, credential, phone, or OTP arguments. Keep the process attached so
the CLI can report exit and launch failures. Node's `spawn` supports direct
argv-based child processes on all target platforms.
([Node.js child-process documentation](https://nodejs.org/api/child_process.html))

Use an application-level lock before launch and treat a browser-held profile
lock as `profile_in_use`. Never force-kill an unrelated browser to obtain the
lock. Because the profile is dedicated, opening it cannot silently reuse the
user's ordinary Chrome process or profile.

## Manual challenge boundary

During phone login, `coupangctl` may use the narrow page adapter to navigate the
allowlisted login flow, detect coarse states, fill the supplied phone/OTP values,
and activate the corresponding visible UI controls. It must not call or replay
the send/verify endpoints directly, record a HAR, screenshot the OTP page,
export form values, solve a challenge, or automatically retry an inconclusive
submission. Authentication succeeds only after the exact protected order page
and structured document are observed.

The explicit `--qr-output` presentation mode is narrower: it may navigate to
the allowlisted protected order URL, activate the QR tab, capture the QR page to
a private ephemeral PNG, and poll only the resulting location. It must not read
or log the QR payload, verification number, cookies, storage, form fields, or
network bodies. It succeeds only after return to the exact order-list path.

After the window closes, `auth status` performs the proof in phase two by
opening an allowlisted, read-only account document and returning one of a small
set of states such as `authenticated`, `reauth_required`, `challenge_required`,
or `access_blocked`. A login redirect is not an authenticated result.

This separation matters operationally: the successful human interaction and
the programmatic data acquisition have different trust and failure boundaries.
It also makes retries safe. A failed status check must never resend an OTP.

## CDP constraints and safe use

For read-only sync, launch a short-lived browser process against the persistent
dedicated profile and use `--remote-debugging-port=0`; do not export and restore
session state through a second storage file. Chromium's API states that port `0` selects an
ephemeral OS port and writes it to a well-known file in the user-data directory.
The implementation writes the chosen port and browser WebSocket path to
`DevToolsActivePort`.
([Chromium DevToolsAgentHost API](https://chromium.googlesource.com/chromium/src/+/master/content/public/browser/devtools_agent_host.h),
[Chromium DevTools active-port implementation](https://chromium.googlesource.com/chromium/src/+/main/content/browser/devtools/devtools_http_handler.cc))

The production adapter should:

- wait for a newly written `DevToolsActivePort`, validate its format, and then
  verify `/json/version` before connecting;
- accept only a numeric ephemeral port and the browser-generated WebSocket path;
- connect only to `127.0.0.1` or `::1`, never a host supplied by a remote client;
- keep the endpoint alive only for the sync operation and never print the port
  or WebSocket URL;
- allowlist the exact read-only origins and paths the adapter may navigate to;
- parse the required structured document in memory, discard the raw document,
  and return only a typed, redacted response to the core;
- respect browser policy that disables remote debugging and return
  `remote_debugging_disabled` instead of attempting a workaround.

Chromium's desktop remote-debugging server binds to IPv4 or IPv6 loopback, and
its source rejects remote debugging of Chrome's default profile and respects
the `DevToolsRemoteDebuggingAllowed` policy.
([Chromium remote-debugging server](https://chromium.googlesource.com/chromium/src/+/HEAD/chrome/browser/devtools/remote_debugging_server.cc))

Loopback is not authentication: another process running as the same local user
may be able to reach the endpoint while it exists. The dedicated directory,
ephemeral port, short browser lifetime, private directory permissions, and
non-disclosure of `DevToolsActivePort` are therefore defense in depth, not a
claim that CDP is a secret channel.

Microsoft documents that Edge's DevTools Protocol matches Chrome's protocol,
that a separate instance can use `--user-data-dir`, and that clients discover
targets through the local JSON endpoint and WebSocket URL. Edge can be a tested
fallback behind the same adapter, but it needs its own compatibility tests and
profile; a Chrome profile must never be opened by Edge.
([Microsoft Edge DevTools Protocol](https://learn.microsoft.com/en-us/microsoft-edge/devtools/protocol/),
[Microsoft Edge UserDataDir policy](https://learn.microsoft.com/en-us/deployedge/microsoft-edge-browser-policies/userdatadir))

## Cross-platform support boundary

The first release should support current vendor-supported desktop Chrome on
Windows, macOS, and mainstream 64-bit Linux distributions, then add Edge and
Chromium as explicitly tested fallbacks. Google publishes the current Chrome
desktop OS support floor; packaging tests should exercise those supported
families rather than promise every Chromium-derived browser.
([Chrome system requirements](https://support.google.com/chrome/a/answer/7100626))

GUI preflight is advisory. On Linux, `DISPLAY` identifies an X server and
`WAYLAND_DISPLAY` identifies a Wayland socket, but environment presence does not
prove a usable interactive window. The authoritative check is whether the
headed browser actually launches and remains available. Windows services run
in a noninteractive window station and cannot directly show UI to the user, so
service/session-zero execution is unsupported for login.
([X11 display model](https://www.x.org/guide/concepts/),
[Wayland display connection](https://wayland.freedesktop.org/docs/html/apb.html),
[Windows interactive services guidance](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services))

If no usable GUI exists during initial authentication, return a structured result such as:

```json
{
  "ok": false,
  "code": "desktop_required",
  "message": "Run coupangctl auth login on a trusted interactive desktop."
}
```

Do not silently fall back to headless login. Read-only verification and sync
should be headless after a dedicated authenticated profile exists locally. A
server with Xvfb or another private headed display may use `--qr-output` for
human-approved initial authentication; a server with no headed renderer can
only import normalized, redacted SQLite or JSON produced on a trusted desktop. It cannot
import a Chrome profile, cookies, tokens, a HAR, Playwright storage state, or a
raw order payload. `--qr-output` is a local file presentation mechanism, not a
network relay; remotely transporting that file requires a separately reviewed
channel.

## Local smoke test

On 2026-09-01, the development macOS host had Google Chrome
`152.0.7977.65`. Launching that installed executable directly with a fresh
temporary `--user-data-dir`, `--remote-debugging-port=0`, and `about:blank`
successfully created a two-record `DevToolsActivePort` file. No Coupang page,
credential, cookie, OTP, or private payload was used. This verifies the native
launch and ephemeral-port bootstrap on the current development host only; it is
not a cross-platform compatibility result or an authentication test.

## Acceptance tests implied by this research

- Phone login starts a headed browser, accepts no phone, OTP, cookie, or
  credential command-line flags, leaves CAPTCHA to the user, and submits a
  user-supplied OTP at most once through the visible page UI.
- The default dedicated mode never discovers, reads, attaches to, copies, or
  modifies the everyday browser profile. The explicit current-browser mode is
  unavailable until Chrome's own remote-debugging opt-in and approval succeed.
- A second `coupangctl` process receives `profile_in_use` without killing the
  first browser.
- Sync uses headless Chrome with an ephemeral loopback CDP endpoint, rejects stale or malformed
  `DevToolsActivePort`, and never prints connection details.
- Managed-browser policy rejection becomes `remote_debugging_disabled`.
- Browser-family or major-version mismatch becomes an actionable profile
  compatibility error.
- Login redirects, CAPTCHA pages, WAF blocks, and account pages map to distinct
  typed states and cannot trigger OTP resend.
- Logs, traces, crashes, exports, and MCP responses contain no profile path,
  cookies, OTPs, raw responses, PII, or real order fixtures.
- A machine without an interactive GUI returns `desktop_required`; no headless
  authentication code path exists. A private Xvfb display counts as a headed
  renderer for the explicit `--qr-output` mode.
- Production dependency and package-content checks reject Orca, Playwright,
  accessibility automation, and agent-skill imports.
