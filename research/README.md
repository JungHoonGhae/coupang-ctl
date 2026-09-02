# Research probes

These are the executable probes used during the initial Coupang protocol-feasibility study. They are evidence and exploration tools, not the production architecture.

Product research notes include [`viral-shopping-insights.md`](viral-shopping-insights.md), which records first-party recap patterns, privacy boundaries, and the prioritized story backlog.

## Preconditions

Launch a dedicated Chrome profile with CDP enabled. Never attach research scripts to a personal everyday browser profile.

```bash
mkdir -p /tmp/coupangctl-chrome
open -na "Google Chrome" --args \
  --remote-debugging-port=9223 \
  --user-data-dir=/tmp/coupangctl-chrome \
  --no-first-run \
  --no-default-browser-check
```

Install dependencies:

```bash
npm install
```

The probes default to `http://127.0.0.1:9223`. Override with `COUPANG_CDP_URL` when necessary.

## Probe inventory

- `network-probe.ts`: captures public search-page JSON/XHR metadata.
- `product-network-probe.ts`: discovers product, review, promotion, recommendation, and quantity endpoints.
- `product-option-metadata-probe.ts`: records only the key/type shape of the
  public quantity response for an explicitly supplied product/vendor-item pair;
  it never writes the identifiers or response values into the repository. Set
  `COUPANG_PRODUCT_ID` and `COUPANG_VENDOR_ITEM_ID`; set
  `COUPANGCTL_PRODUCT_PROBE_HEADED=1` when the sampled headless layout is
  rejected.
- `direct-replay-probe.ts`: proves review JSON can be replayed through direct HTTP with session cookies and appropriate headers.
- `login-probe.ts`: email/password bootstrap experiment; may trigger CAPTCHA and must not attempt to bypass it.
- `phone-login-probe.ts`: initiates phone login and sends an OTP using Doppler-injected `COUPANG_PHONE`.
- `qr-login-metadata.ts`: compares headed/headless QR reachability and emits
  only redacted paths, status codes, and response key/type shapes. It never
  prints or persists QR values.
- `submit-otp.ts`: consumes an ephemeral `COUPANG_OTP`; never persist the OTP.
- `order-network-probe.ts`: records only endpoint names and response shapes while opening the authenticated order list.
- `direct-order-replay.ts`: negative/limitation probe. Pure HTTP order replay produced `403`/`406`; retain it to reproduce and investigate that boundary.
- `pagination-ui-probe.ts`: verifies the redacted year/page transition and the
  structured order-model endpoint without emitting order values.
- `receipt-ui-probe.ts`: records only receipt endpoint metadata, key/type
  shapes, and normalized control kinds. It must not submit receipt-generation
  requests.
- `account-benefits-probe.ts`: uses the persisted authenticated session to
  discover WOW membership, payment-method, card, cash, and benefit surfaces.
  It emits only URL shapes, response shapes, and boolean DOM signals; cookie
  values, card identifiers, account text, and response bodies are discarded.
- `order-shape/`: prints only order response keys/types, array lengths, and
  sanitized normalization status.
- `pagination-sequence/`: prints pagination cursors only.

## Credential injection

```bash
doppler run -p cli-mcp-lab -c dev_coupang -- npm run <script>
```

Do not add secret values to commands, source files, fixtures, screenshots, or logs. OTP values should be passed through an ephemeral environment variable or stdin and immediately discarded.

## Known rough edges

- Several probes use a fixed AirPods search and a sampled product identifier. Convert these to explicit CLI arguments before promoting code into `src/`.
- Endpoint paths and response shapes are private implementation details and may change without notice.
- The phone authentication UI is frame-based; selectors must be scoped to the frame containing the visible OTP input.
- The probes intentionally summarize response shapes rather than printing personal order data.
