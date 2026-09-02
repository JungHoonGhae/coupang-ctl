# Authentication architecture

Validated: 2026-09-01 (Asia/Seoul)

## Evidence

- A Playwright-driven click on Coupang's phone-code button reached
  `POST /login/v2/pincode/send` but received `403`.
- Opening the same login page in the user's ordinary Chrome process and clicking
  the visible button through macOS accessibility successfully sent the code.
- The successful path did not replay the endpoint, override headers, copy
  cookies, or use browser-stealth settings.
- Coupang's headed desktop login offers QR login. Redacted protocol metadata
  identified narrow create/query endpoints, while true headless Chrome received
  HTTP 403 before reaching the login origin.

This means the product must separate **browser launch**, **human interaction**,
and **session consumption**. Orca proved the manual UI path during research, but
Orca, agent skills, Playwright, and external automation tools are not runtime
dependencies of `coupangctl`.

## Supported modes

### 1. Local desktop login (default)

`coupangctl auth login` defaults to QR, launches the installed system Chrome
with a dedicated `coupangctl` profile, and enters through the protected order
URL so the login return context is retained. `--phone` opens the manual phone
fallback.

- The CLI does not fill the phone number, solve CAPTCHA, enter OTP, or click the
  send/login buttons.
- The user completes the entire challenge in the visible browser.
- The CLI observes only a coarse completion signal, such as leaving the login
  origin or successfully loading a read-only account page.
- Coupang cookies are captured only from the dedicated login browser into a
  private atomic `0600` session file. They are never printed, returned in
  command output, or copied from the user's everyday profile.
- Browser discovery, process launch, profile isolation, completion polling, and
  cleanup are implemented in the distributed `coupangctl` binary.
- The user needs only `coupangctl` and a supported installed browser; no Orca,
  Codex skill, Python helper, or separate Playwright installation is required.

When a headed renderer exists but cannot be viewed directly,
`--qr-output PATH` uses the product's narrow native CDP adapter to select the QR
tab and capture the rendered login page. The new file is mode `0600`, is never
mentioned in JSON output, and is deleted after success, expiry, timeout, or
cancellation. This mode never fills credentials, solves CAPTCHA, enters OTP, or
exports cookies. It is a presentation adapter for a human-approved QR challenge.

### 2. Existing authenticated dedicated profile

`coupangctl auth status` and read-only sync commands restore the private session
into a short-lived dedicated profile in headless Chrome. Browser acquisition and
session injection remain behind narrow adapters; the
typed parser, database, CLI, and MCP layers never receive browser automation
objects.

### 3. Machine without a usable desktop

Authentication is not automated. CAPTCHA and OTP must not be bypassed. A true
headless renderer is not currently supported for initial login because the live
protected-entry probe received HTTP 403.

The supported workflow is:

1. Run headed authentication on a trusted desktop machine, or run
   `--qr-output` with a headed Chrome renderer under a private Linux display
   such as Xvfb. Subsequent sync can run headless on that same machine and
   profile.
2. Export normalized, redacted SQLite/JSON data.
3. Transfer that data to the non-GUI machine.
4. Run analysis and MCP locally against the exported data.

Do not transfer Chrome profiles, raw cookies, Playwright storage state, raw order
payloads, or personal browser data between machines. A future remote-login mode
would require a separately reviewed, end-to-end encrypted human browser relay;
it is not part of the first release.

## Adapter boundary

```text
NativeBrowserLauncher ─┐
FixtureDocumentSource ─┴─> ProtectedDocumentSource ─> typed parser
                                                     ├─> SQLite
                                                     ├─> CLI
                                                     └─> MCP
```

- `NativeBrowserLauncher` is code shipped with `coupangctl`. It discovers and
  launches supported Chrome/Chromium/Edge installations without Playwright
  automation flags.
- Orca/computer-use may validate behavior during development, but it is not an
  adapter, package, binary, or configuration exposed to users.
- `FixtureDocumentSource` supplies synthetic documents to unit tests.
- `ProtectedDocumentSource` returns a documented, narrow response shape rather
  than exposing cookies, pages, selectors, or raw customer payloads.

## Required tests

- Login launch never reads the everyday Chrome profile.
- Login launch never accepts phone numbers or OTPs as command-line flags.
- CAPTCHA/OTP pages pause for human interaction and are never auto-submitted.
- Logs and structured errors contain no phone number, OTP, cookie, order ID, or
  raw response body.
- Reusing the dedicated profile can load a read-only account page after login.
- A non-GUI environment can run read-only headless sync when the dedicated
  authenticated profile already exists locally. Initial QR authentication needs
  a headed renderer; an Xvfb-backed renderer can expose only the ephemeral QR
  screenshot without transferring a browser profile.
- Packaging and clean-machine tests fail if any production path imports Orca,
  Playwright, or an agent-specific skill.
- Export contains normalized data only and cannot include browser session state.
