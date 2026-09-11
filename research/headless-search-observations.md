# Headless search observations — 2026-09-07

These are local diagnostic observations, not a successful bypass implementation.
No credentials, cookie values, page bodies, or customer fixtures are retained here.

## Observed

- Search: `/np/search`, query `미니 식기`, using the user's supplied URL.
- Dedicated profile, visible Chrome, no remote-debugging arguments: product
  cards and prices were readable using the OS accessibility interface.
- Same profile and URL, visible Chrome with `--remote-debugging-port=0`:
  Access Denied. No CDP client connected during this comparison.
- Reopening without remote debugging immediately afterward: still Access Denied.
- Dedicated profile, `--headless=new --dump-dom`, no remote-debugging argument:
  denial title and edgesuite error reference, 302 output bytes, zero product links.
- Fresh temporary profile, same headless command and URL: denial title and
  edgesuite error reference, 304 output bytes, zero product links.
- Both DOM-dump processes needed the outer deadline to terminate. Their exit
  status of zero does **not** establish a successful navigation or search.
- HTTP status was not measured for the DOM-dump probes; do not infer a measured
  403 from the visible denial document alone.
- A fresh-profile local HTTP control using the same installed Chrome and
  `--headless=new --dump-dom` completed without hitting its deadline: exit 0,
  435 output bytes, and the expected JavaScript-generated synthetic product
  marker. This is a local execution check, **not** a live search success.
- The local control measured a HeadlessChrome user-agent marker and
  `navigator.webdriver === false`. Do not assume webdriver is true for every
  headless invocation based on general documentation; execution paths differ.
- Installed versions: normal Chrome 152.0.7977.83; cached Chrome for Testing
  and Headless Shell 151.0.7922.34. No new browser was installed.
- Cached Headless Shell 151, fresh temporary profile, no debugging argument:
  completed before the deadline, 304 output bytes, denial title and edgesuite
  reference, zero product links. Its process exited zero despite the denial.
- Cached Chrome for Testing 151 with the same probe produced no stdout before
  the 15-second outer deadline. This result is inconclusive, **not** proof of
  either successful access or a 403 response.
- All newly created temporary profiles were removed after their corresponding
  browser process exited. The existing dedicated profile was not deleted.
- Follow-up on cached Chrome for Testing 151 through the existing native CDP
  headless acceptance harness, using a fresh temporary dedicated profile and
  the harness query `한편죽`: measured HTTP 403, denial title true, password
  field false, zero product links and JSON-LD scripts, headless marker true.
  The test failed with `browser access denied` after 18.59 seconds. This
  establishes a denial for this separate CDP run; it does not retrospectively
  classify the earlier DOM-dump timeout.

## Interpretation limits

Disabling remote debugging alone did not restore headless search. A blank
profile also did not restore it, so existing profile cookies are not a sufficient
explanation. The exact source-side rule remains unknown. Sequential runs may
change local and server-side state, so the visible A/B/A test does not establish
that the debugging flag is the sole cause.

## Completion remains unproven

The production headless path must return valid parsed product identities and
names, without a denial or login page. A visible-browser result, DOM byte count,
process exit code, or successful build is not a substitute for that check.
Existing bypass experiments and production code were not modified by these
DOM-dump probes; existing profile cookies were not explicitly cleared.

Across three consecutive goal turns, live headless access remained blocked.
The installed supported alternatives and non-CDP path tested here did not
return product data. Further progress needs a source-permitted access path or
an external change that makes a bounded live headless test meaningful. This
does not establish that all conceivable headless implementations are impossible.
