# coupangctl

Consumer-focused Coupang CLI and MCP data layer.

The project aims to expose a user's own Coupang shopping data to local tools and AI agents without relying on DOM-driven browser automation for routine operations.

## Status

Early protocol research. Authentication, public product/review surfaces, and structured order-history extraction have been validated against a disposable test session.

## Credential setup

Development credentials live in Doppler and must never be committed:

```bash
doppler run -p cli-mcp-lab -c dev_coupang -- <command>
```

See [HANDOFF.md](HANDOFF.md) for validated findings and the proposed architecture.

## Safety boundary

- Read-only research and personal-data export first.
- Never automate final payment or purchase confirmation.
- Do not log cookies, passwords, OTPs, addresses, order IDs, or raw order payloads.
- Treat private web endpoints as unstable and subject to Coupang's terms and technical controls.

