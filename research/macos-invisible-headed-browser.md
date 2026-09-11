# Invisible non-headless Chrome on macOS

Validated 2026-09-04 against Chrome for Testing Stable 152.0.7977.82
(Chromium revision 1669021). Chromium links below are pinned to the matching
source commit `d04cdb24d67b081f6cf80200ffc5233f44b61109`; library links are
pinned to the repository commits listed under “Source snapshot.”

## Decision

A normal, non-headless Chrome window cannot be guaranteed to remain completely
invisible inside the active macOS GUI session. `open -g -j`, off-screen
coordinates, minimization, and a helper repeatedly calling
`NSRunningApplication.hide()` are all presentation controls around a real
AppKit window. None establishes a durable “no WindowServer surface may appear”
invariant.

For `coupangctl`:

1. **Adopt `--headless=new` as the only automatic local background renderer.**
   Chrome documents that unified headless creates but does not display platform
   windows. Keep the existing Go typed core and narrow CDP adapter; continue to
   extract documented/structured network JSON before considering DOM parsing.
   ([Chrome Headless mode](https://developer.chrome.com/docs/automation-and-testing/headless))
2. **Make normal headed Chrome an explicit, user-approved mode** for login,
   session renewal, or compatibility work. It is visible by contract. If a site
   rejects headless execution, return a typed next action rather than silently
   claiming an ordinary Chrome window is invisible.
3. **Use CDP `Target.createTarget(hidden:true)` for the version-gated macOS
   compatibility work pool, while retaining its ordinary bootstrap page.** This
   creates a non-tab `WebContents`, not an ordinary hidden Chrome window. The
   production-path synthetic matrix below validates navigation, evaluation,
   concurrency, idle shrink, and orderly shutdown. The field remains
   experimental, cannot bootstrap cold Chrome by itself, and does not make a
   hard OS-level invisibility promise before the red-sentinel gate passes.
4. **Do not ship `open -j` plus a hide watcher as a hard guarantee.** It can be a
   clearly labelled best-effort convenience only. Do not use off-screen or
   minimized windows as substitutes for invisibility.
5. **Do not make Linux/Xvfb virtualization the default.** It is the robust
   non-headless isolation option when a physical host window is forbidden, but
   it changes the renderer from macOS Chrome to Linux Chrome and adds a guest
   image, Chrome, Xvfb, profile lifecycle, and visible login handoff. Consider
   it only as a separate opt-in compatibility backend.

This means the current macOS background-headed design (`open -j` followed by
ordinary `Target.createTarget(background:true)`) can be quiet in favorable
runs, but cannot support a “completely invisible throughout navigation and
parallel tab creation” product promise.

## Why `open -g -j -n -W` can still show Chrome

The flags describe launch behavior, not permanent ownership of application
visibility. Their meanings below were also checked against Apple's installed
`open(1)` manual on macOS 26.5.2 (25F84):

- `-g` asks LaunchServices not to bring the app to the foreground; `-j` asks it
  to launch hidden; `-n` creates another instance; `-W` merely makes `open`
  wait; and arguments after `--args` are delivered to Chrome. AppKit exposes
  the corresponding launch configuration as activation, hiding, and
  new-instance preferences. These do not forbid the launched process from
  subsequently creating or ordering windows.
  ([NSWorkspace.OpenConfiguration](https://developer.apple.com/documentation/appkit/nsworkspace/openconfiguration))
- Apple explicitly warns that `NSRunningApplication`'s time-varying properties
  are race-prone and that a hidden application may unhide itself at any time.
  Its `hide()` API only *attempts* to hide and returns whether the attempt
  succeeded. A polling or notification-based daemon can therefore re-hide only
  after a state transition; a visible frame may already have been composited.
  ([NSRunningApplication](https://developer.apple.com/documentation/appkit/nsrunningapplication),
  [`hide()`](https://developer.apple.com/documentation/appkit/nsrunningapplication/hide%28%29))
- Chrome's own macOS controller starts a background/app-shim launch with
  `NSApplicationActivationPolicyProhibited`, but its source says the OS changes
  that policy when Chrome creates **any windows**. Chrome then updates its
  keep-alive behavior after observing the policy change.
  ([`app_controller_mac.mm`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/chrome/browser/app_controller_mac.mm))
- Corroborating upstream source at Chromium main HEAD on the validation date
  marks newly created browser windows for `Show()`/`ShowInactive()`; the Cocoa
  bridge can then call `activateIgnoringOtherApps:` and
  `makeKeyAndOrderFront:`. This is an explicit order-to-front path after the
  launch request, not something `open -j` continuously prevents.
  ([`browser_navigator.cc`](https://chromium.googlesource.com/chromium/src/+/14f422b2bb11c8856a2a0cf9331786fe6ad487bf/chrome/browser/ui/navigator/browser_navigator.cc#432),
  [`native_widget_ns_window_bridge.mm`](https://chromium.googlesource.com/chromium/src/+/14f422b2bb11c8856a2a0cf9331786fe6ad487bf/components/remote_cocoa/app_shim/native_widget_ns_window_bridge.mm#1055))
- Passing `about:blank` asks Chrome to make page UI. Likewise,
  `Target.createTarget(background:true)` means “do not focus this normal
  target”; it does not mean “make no window.” A restored startup window, popup,
  permission prompt, crash UI, or parallel normal target can independently
  cross the same boundary.

`--no-startup-window` is a better bootstrap than `about:blank`: Chromium defines
it as suppressing the automatic startup window and treats the launch as silent.
It still does not turn later ordinary targets into hidden surfaces.
([switch definition](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/chrome/common/chrome_switches.cc),
 [startup handling](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/chrome/browser/ui/startup/startup_browser_creator.cc))

An `LSUIElement` helper avoids giving the **helper** a normal Dock presence. It
does not alter Google Chrome's bundle metadata, activation policy, or windows;
that conclusion follows from Apple defining `LSUIElement` as a property of the
application whose `Info.plist` contains it.
([`LSUIElement`](https://developer.apple.com/documentation/bundleresources/information-property-list/lsuielement))

## What CDP can and cannot hide

The stable CDP Browser domain has only `normal`, `minimized`, `maximized`, and
`fullscreen` window states—there is no hidden state.
([`Browser.pdl`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/third_party/blink/public/devtools_protocol/domains/Browser.pdl))

The Target domain makes three materially different promises:

- `background:true` creates an ordinary target without bringing it forward.
- `focus:false` leaves browser focus unchanged.
- Experimental `hidden:true` creates a target observable through CDP but absent
  from the tab strip; it cannot be combined with `forTab:true`,
  `newWindow:true`, or `background:false`, and its lifetime is limited to the
  CDP session.

([`Target.pdl`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/third_party/blink/public/devtools_protocol/domains/Target.pdl))

Chromium implements a hidden target by owning a `WebContents` directly rather
than adding it to a browser window. This is the only non-headless local path
found that avoids normal tab/window UI by construction.
([`hidden_target_manager.cc`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/content/browser/devtools/protocol/hidden_target_manager.cc))

It is not a general production replacement for headless rendering:

- the protocol field is experimental;
- the handler requires remote debugging and at least one existing frame
  target, so a hidden target cannot itself cold-start a windowless Chrome;
  ([`target_handler.cc`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/content/browser/devtools/protocol/target_handler.cc))
- Chromium's own BiDi runner added retries for the cold-start interval before
  the initial frame target is registered;
  ([Chromium change `cb8a25f1`](https://chromium.googlesource.com/chromium/src/third_party/+/cb8a25f1f138378f6e561c4bf18dd0bc1c63faed))
- it is not a normal browser tab. Default `WebContentsDelegate` behavior and
  UI-dependent flows can differ, so popup and dialog compatibility must be
  measured rather than assumed.
  ([`web_contents_delegate.cc`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/content/public/browser/web_contents_delegate.cc))

The safer path is therefore: keep the app-owned Chrome/CDP process warm, retain
one blank launch-hidden ordinary bootstrap page for the entire session, and
create a bounded pool of session-scoped hidden targets for real work. This
preserves Chrome-owned profile state without copying cookies and satisfies the
hidden-target manager's lifetime requirement. It still must fail closed if
Chrome creates any independent UI, and it does not make an
`NSRunningApplication.hide()` watcher a guarantee.

## Measured hidden-target lifecycle

The production page-pool path was exercised on 2026-09-04 with isolated empty
temporary profiles and synthetic `data:` content only. Chrome for Testing
152.0.7977.77, Stable 152.0.7977.82, and Canary 155.0.8041.0 each created four
hidden work targets, navigated and evaluated them, shrank the idle pool, stayed
healthy for two seconds, and shut down normally. No Coupang page, login state,
cookie, screenshot, or customer payload was used or retained.

One negative control on 152.0.7977.82 closed the last ordinary bootstrap target
after hidden targets had navigated successfully. Chrome immediately ended with
`EXC_BREAKPOINT` / `SIGTRAP` on `CrBrowserMain`, matching the lifecycle invariant
in Chromium's hidden-target manager. The product therefore retains the ordinary
bootstrap target until the whole process closes, and automated tests must not
repeat this known crash path.

These runs prove the CDP target lifecycle and the absence of normal work tabs;
they do not prove that macOS never composites the launch bootstrap, browser UI,
or a permission/crash prompt. That separate OS-level claim requires the
ScreenCaptureKit/CGWindow positive-control gate below and explicit Screen
Recording permission.

## Alternatives

| Approach | Completely invisible? | Session/handoff and cost | Decision |
|---|---|---|---|
| Unified `--headless=new` | Yes, by Chrome's no-displayed-platform-window contract | Reuses an app-owned persistent profile; explicit headed login must be a separate approved phase | **Default** |
| CDP `hidden:true` WebContents | No normal tab/window for that target; browser-level UI still needs testing | Same live profile and cheap Go/CDP integration; experimental, session-scoped, needs bootstrap frame target | **Feature-gated experiment** |
| `open -j` + `NSRunningApplication.hide()` daemon | No; mutable app state and reactive race | Easy reuse of current Go adapter and warm profile | **Reject as guarantee** |
| Off-screen window | No; still a WindowServer window | Cheap, but Chrome/AppKit constrain regular windows back toward visible work areas and each popup needs separate control | **Reject** |
| Minimized window | No; CDP/WebDriver expose minimization, not hiding | Cheap but remains user-visible in OS UI and window-manager behavior varies | **Reject** |
| Linux Chrome in VM + Xvfb | Yes with respect to the macOS WindowServer | Guest-owned persistent profile; login needs a temporarily attached guest display; high image/runtime/update cost; Linux is not macOS Chrome | **Opt-in backend only** |
| `LSUIElement` native helper + unattached `WKWebView` | Can avoid an AppKit window, but is not Chrome | WebKit engine, separate `WKWebsiteDataStore`, no Chrome profile/CDP compatibility | **Reject for Chrome fallback** |

Chromium's macOS window sizing deliberately pulls partially off-screen regular
windows back into the work area, while its native window layer bypasses physical
display constraints only for headless operation. Off-screen positioning is
therefore especially brittle.
([`window_sizer.cc`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/chrome/browser/ui/window_sizer/window_sizer.cc),
 [`native_widget_mac_nswindow.mm`](https://chromium.googlesource.com/chromium/src/+/d04cdb24d67b081f6cf80200ffc5233f44b61109/components/remote_cocoa/app_shim/native_widget_mac_nswindow.mm))

Xvfb is an X server backed by virtual memory rather than display hardware;
Playwright's official CI guidance requires it for headed Linux execution.
Apple's Virtualization framework can host the Linux guest. This combination is
structural host isolation, not a macOS window-hiding trick.
([pinned X.Org Xvfb manual source](https://gitlab.freedesktop.org/xorg/xserver/-/blob/0da4d2480066fb2c783325e7594902aacc23413e/hw/vfb/man/Xvfb.man),
 [Playwright CI](https://playwright.dev/docs/ci#running-headed),
 [Apple Virtualization](https://developer.apple.com/documentation/virtualization))

WKWebView is a WebKit `NSView`, with storage supplied by `WKWebsiteDataStore`.
It cannot validate Chrome-specific compatibility or reuse a Chrome profile.
([`WKWebView`](https://developer.apple.com/documentation/webkit/wkwebview),
 [`WKWebsiteDataStore`](https://developer.apple.com/documentation/webkit/wkwebsitedatastore))

## Automation-library reality

Changing the controller does not change the macOS constraint:

| Controller | Official surface relevant here | Result |
|---|---|---|
| Playwright | Headless by default; headed Linux uses Xvfb | No headed-hidden contract. ([debugging](https://playwright.dev/docs/debug), [CI](https://playwright.dev/docs/ci#running-headed)) |
| Puppeteer | `headless: false` selects headed; `waitForInitialPage: false` can accompany `--no-startup-window` | No headed-hidden contract. ([pinned `LaunchOptions.ts`](https://github.com/puppeteer/puppeteer/blob/a908d92d1541d4fbc5506fcd972f8872e38d12de/packages/puppeteer-core/src/node/LaunchOptions.ts)) |
| chromedp | Defaults to `--headless`; exposes command customization and generated CDP, including `WithHidden` | Can invoke the experimental protocol field, but supplies no OS guarantee. ([pinned allocator](https://github.com/chromedp/chromedp/blob/7963c203ed5458147d27dc39a5c06d2b12e81664/allocate.go), [pinned generated target API](https://github.com/chromedp/cdproto/blob/e85f50dbfd325d0ff790b0b9c4fbc88849a822c3/target/target.go)) |
| rod | Launcher defaults to headless and `no-startup-window`; disabling headless plus creating a page creates a normal CDP target | No headed-hidden contract. ([pinned launcher](https://github.com/go-rod/rod/blob/d38c75327872c72a1cf2010dad7527f9e52eebf3/lib/launcher/launcher.go)) |
| Selenium/WebDriver | Positions and minimizes top-level windows | No hidden state; Selenium notes exact minimize behavior is window-manager-specific. ([window interactions](https://www.selenium.dev/documentation/webdriver/interactions/windows/)) |
| Chrome for Testing | Pinned, non-auto-updating browser binaries | Improves reproducibility, not visibility semantics. ([official documentation](https://developer.chrome.com/blog/chrome-for-testing), [pinned Stable data](https://github.com/GoogleChromeLabs/chrome-for-testing/blob/0d1aeb8df35edce14632811dfebe7d60facab54b/data/last-known-good-versions.json)) |

Keeping `coupangctl`'s small direct-CDP Go adapter is consequently lower cost
than adding a Node runtime or another Go controller. The meaningful change is
the target/renderer contract and fail-closed policy, not the client library.

## Red-capable macOS visibility gate

A green result from checking CDP `Browser.getWindowBounds` is insufficient: it
asks Chrome about its logical state and cannot detect a transient AppKit surface.
Use a native test helper and synthetic local content only:

1. Start capture **before process launch**. Enumerate windows continuously with
   `CGWindowListCopyWindowInfo`, record Chrome owner PIDs, on-screen status,
   layer, and non-empty bounds, and capture every display with ScreenCaptureKit
   at the smallest supported frame interval. The test runner needs Screen
   Recording consent.
   ([`CGWindowListCopyWindowInfo`](https://developer.apple.com/documentation/coregraphics/cgwindowlistcopywindowinfo(_:_:)),
   [`kCGWindowOwnerPID`](https://developer.apple.com/documentation/coregraphics/kcgwindowownerpid),
   [`kCGWindowIsOnscreen`](https://developer.apple.com/documentation/coregraphics/kcgwindowisonscreen),
   [ScreenCaptureKit sample](https://developer.apple.com/documentation/screencapturekit/capturing-screen-content-in-macos))
2. Serve a local synthetic page containing a dynamically generated, time-coded
   red/magenta checker sentinel. Navigate repeatedly, create the configured
   number of parallel targets, exercise a synthetic `window.open`, resize, and
   close them, then keep capture active through browser teardown.
3. **Positive control:** first launch an ordinary headed Chrome window showing
   the sentinel. The harness must observe both a Chrome-owned on-screen window
   and matching pixels. If it does not, the run is invalid—not green.
4. **Candidate:** fail on any captured sentinel frame or any Chrome-owned,
   on-screen, non-empty normal window. For the app-hide strategy, additionally
   fail whenever `NSRunningApplication.isHidden` becomes false. Run across all
   displays/Spaces and repeat startup plus parallel creation enough times to
   expose races; include Stable and Beta in the experimental-target matrix.
5. Report window metadata and boolean/timing summaries only. Do not retain raw
   screenshots, page payloads, cookies, or user profile data.

Even a 60 fps capture can miss a sub-frame flash. The positive control makes
the test capable of turning red and useful for regression gating, but sampling
cannot prove absence. The product guarantee must rest on a no-host-window
architecture (`--headless=new`, or a guest/Xvfb backend), not repeated green
runs from app hiding.

## Safety and scope

The background adapter remains read-only, bounded, and behind explicit browser
and endpoint boundaries. Authentication remains user-controlled, with no cookie
export or profile copying. CAPTCHA bypass and bot-detection evasion are out of
scope, and this research does not authorize purchase or payment automation.

## Source snapshot

Released-browser baseline and repository snapshots used on 2026-09-04:

- Chromium tag 152.0.7977.82: `d04cdb24d67b081f6cf80200ffc5233f44b61109`
- Chromium main HEAD (corroborating source only): `14f422b2bb11c8856a2a0cf9331786fe6ad487bf`
- Chrome for Testing data: `0d1aeb8df35edce14632811dfebe7d60facab54b`
- Playwright: `d1dcd6bc0a138ec0fd943df19e07458dc426ee22`
- Puppeteer: `a908d92d1541d4fbc5506fcd972f8872e38d12de`
- chromedp: `7963c203ed5458147d27dc39a5c06d2b12e81664`
- chromedp/cdproto: `e85f50dbfd325d0ff790b0b9c4fbc88849a822c3`
- rod: `d38c75327872c72a1cf2010dad7527f9e52eebf3`
- Selenium: `34c744284a5726b252b3adc7cdcc6d2c362ff71e`

Chrome 152 is the released behavior baseline. The named controller revisions
and Chromium main HEAD are repository snapshots, not assertions that those
commits are release tags. The cold-start retry evidence is the fixed Chromium
change `cb8a25f1f138378f6e561c4bf18dd0bc1c63faed`.

Apple Developer Documentation and the Chrome/Playwright/Selenium documentation
sites are living references and do not publish immutable page revisions. The
behavioral claims that determine the recommendation are therefore anchored to
the pinned Chromium and controller source above; living documentation supplies
the public API contracts.
