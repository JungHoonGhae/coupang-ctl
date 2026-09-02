# Agent instructions

- Product name: `coupangctl`; repository/workspace name: `coupang-ctl`.
- Preserve the separation between the typed core, CLI adapter, and MCP adapter.
- Prefer structured JSON and documented response shapes over DOM selectors.
- Never commit or print credentials, cookies, OTPs, PII, raw order payloads, or real customer fixtures.
- Never implement final purchase/payment automation.
- Use synthetic fixtures and redacted network metadata in tests and documentation.
- Keep reverse-engineered endpoints behind narrow adapters because they are unstable.
- Evidence-first product work: before changing analytics, shopping types, recap
  visuals, or promotional claims, read `PRODUCT_PRINCIPLES.md` and preserve its
  observed/derived/inferred provenance boundary.
- Doppler development config: `cli-mcp-lab/dev_coupang`; reference secret names only.
