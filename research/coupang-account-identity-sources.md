# Coupang retail account identity: public-source research

Checked: 2026-09-11 (Asia/Seoul). Scope: public first-party documentation and
already-discovered public client assets. This investigation did not access a
browser profile, cookies, credentials, an authenticated account, or order data.

## Finding

No new, verified read-response contract for a stable signed-in retail account
subject was established. Public documentation supports `memberSRL` as an
internal identifier vocabulary lead, but does not establish its availability in
an ordinary authenticated response, immutability, or account-switch behavior.
That gap must remain explicit; this research does not make an existing database
safe to bind or merge automatically.

## Primary-source evidence

| Source | Observed evidence | Limit for account binding |
| --- | --- | --- |
| [Korean Coupang privacy notice](https://privacy.coupang.com/ko/center/coupang/) (effective 2026-05-22) | The overseas-processing notice names `memberSRL`, order/invoice numbers, and `PCID` as internally generated numeric information. | This is a data-processing notice, not a client API or lifecycle contract. It does not establish that these identifiers are interchangeable. |
| [Same notice: service-specific disclosures](https://privacy.coupang.com/ko/center/coupang/) | Separate rows describe customer-management numbers used with Coupang Pay, an encrypted customer-management number for a brand membership, and a Coupang Play customer-management number. | A payment, loyalty, encrypted partner, or Play identifier must not be promoted to the retail account subject without an explicit mapping and lifecycle evidence. |
| [Coupang Developer Center](https://developers.coupang.com/en/) and [getting-started index](https://developers.coupang.com/en/getting-started) | The published Open API targets sellers and integrators. Its setup uses a Wing seller account, API keys, an IP allowlist, and HMAC signing. | A seller/vendor identity contract does not establish the identity of the customer signed into the retail website. |
| [Coupang public GitHub organization](https://github.com/coupang) | Organization-scoped code searches for `memberId` and `memberSrl` returned no matches in this check. | A negative indexed search is a discovery limit, not evidence that no internal consumer-account API exists. |

The privacy notice also discusses CI for identity/age verification. It does not
document CI as a retail account response key; this research gives no basis for
using it as the local account-binding identifier. This is an engineering
interpretation of the source's distinctions, not a legal conclusion.

## Client-source leads and access limits

The parallel implementation investigation supplied these URLs from the actual
retail client's script references. Anonymous fetches in this research returned
HTTP 403, so their contents were **not independently verified here**:

- [My Coupang account-client chunk](https://assets.coupangcdn.com/front/mycoupang-ssr/_next/static/chunks/99b4e0d8eeec29a9.js)
- [My Coupang telemetry chunk](https://assets.coupangcdn.com/front/mycoupang-ssr/_next/static/chunks/6673e167b05a26e7.js)
- [My Coupang host-configuration chunk](https://assets.coupangcdn.com/front/mycoupang-ssr/_next/static/chunks/c466cadbdda6545f.js)
- [Coupang JavaScript logging client](https://asset2.coupangcdn.com/customjs/jserror/2.5.1/jslog.min.js)

The web-reader fallback could not interpret the account-client chunk's
`application/octet-stream` response. No authenticated retry or alternate
endpoint enumeration was attempted. A host name, telemetry field, function
name, or cookie-derived value is not itself a validated identity source.

## Existing evidence and remaining acceptance work

The repository's [endpoint catalog](endpoint-catalog.md) already distinguishes
the boolean `/ssr/api/member/auth` response from a stable account subject and
records that `/ssr/api/member/info` did not expose one in its checked shape.
Those authenticated checks were not repeated by this research task.

A candidate is actionable only after the source client's ordinary read flow
establishes its exact endpoint, response path, and subject semantics. Subsequent
authorized verification must show a nonempty subject with the same value after
relogin/profile recreation, a different value for another consenting account,
and no usable subject while signed out. It must also distinguish account
identity from optional subscription/payment identities. Comparisons can happen
inside the browser boundary, reporting only equality and presence outcomes.

These criteria remain unmet by this public-source investigation. No production
adapter, fixture, database binding, issue, or external message was changed.
