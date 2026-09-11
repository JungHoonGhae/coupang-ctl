# Agent instructions

## INTENT.md first — project instruction precedence

- The canonical intent document is the repository-root `INTENT.md`; references to
  `intent.md` or `intend.md` mean this same file.
- For every task in this repository, `INTENT.md` is always the highest-priority
  project document, regardless of the task type or files being changed.
  If this file, a nested `AGENTS.md`, a README, an implementation plan, or other
  project guidance conflicts with it, follow the current requirements in `INTENT.md`.
- At the start of every turn, including resumed work, read the repository-root
  `INTENT.md` in full before planning, editing, testing, or reporting. Re-read it
  whenever it changes; a prior conversation summary does not replace this read.
  If `INTENT.md` is missing or cannot be read in full, pause the task and ask the user
  to restore or provide it before proceeding.
- Within `INTENT.md`, explicit newer decisions take precedence over historical records.
- For the user's requested task, derive implementation decisions and verification
  criteria from the applicable requirements in `INTENT.md`. Do not silently narrow
  or redefine those requirements to fit existing code or an easier implementation.
- Apply the remaining project instructions wherever they do not conflict with `INTENT.md`.
- Before claiming completion, check the result against the applicable requirements
  and completion criteria in `INTENT.md`; explicitly report any unmet criteria.

## Project rules

- Product name: `coupangctl`; local workspace name: `oss-coupang-ctl`; GitHub repository name: `coupang-ctl`.
- Preserve the separation between the typed core, CLI adapter, and MCP adapter.
- Prefer structured JSON and documented response shapes over DOM selectors.
- Never commit or print credentials, cookies, OTPs, PII, raw order payloads, or real customer fixtures.
- Never implement final purchase/payment automation.
- Use synthetic fixtures and redacted network metadata in tests and documentation.
- Keep reverse-engineered endpoints behind narrow adapters because they are unstable.
- Evidence-first product work: before changing analytics, shopping types, recap
  visuals, or promotional claims, read `PRODUCT_PRINCIPLES.md` and preserve its
  observed/derived/inferred provenance boundary.
- Accept releases from downloaded artifacts: verify checksums, SBOMs, and
  attestations, then smoke-test `version`, no-argument help, and `--help` on the
  packaged binary.
- Doppler development config: `cli-mcp-lab/dev_coupang`; reference secret names only.
- Track work in GitHub Issues, not local ticket files. For the personal-shopping
  effort, start with https://github.com/JungHoonGhae/coupang-ctl/issues/12 and use
  native sub-issues/dependencies; record decisions in issue comments.
- For browser transport, product search, or window-visibility changes, read the
  implementation/verification records linked from `INTENT.md` before editing or claiming completion.
