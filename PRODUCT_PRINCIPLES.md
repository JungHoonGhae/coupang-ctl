# Product principles

## Evidence first

Every user-facing fact has one provenance class:

- **Observed**: a source-native structured field read from the user's own data.
- **Derived**: a deterministic calculation over observed fields.
- **Inferred**: a heuristic or model output that the source did not provide.

Prefer observed data, then derived data. Use inference only when source-native
data is unavailable and the product value justifies the uncertainty. Label an
inference at the point of use with its method and coverage; never present it as
a Coupang classification or an observed fact.

For a reverse-engineered field, verify semantics across multiple redacted
samples before adoption. A populated enum is not evidence of the desired
meaning. Record only response paths, key/type shapes, sample counts, and
provenance in research artifacts. Keep the parser and endpoint behind a narrow
adapter.

## Show the receipts

Analytics and promotional recaps should make the evidence visible without
exposing private details. Show:

- the analysis period, using month precision in public-safe output;
- the relevant sample size and denominator;
- the source in plain language, without implying an official public API;
- exclusions such as cancellations, returns, missing timestamps, or unknown
  categories;
- the metric rule and threshold when a type or badge depends on one.

Visualizations must preserve those definitions. Choose a chart whose scale and
baseline match the metric, include an honest missing-data state, and avoid
population claims unless a real comparison dataset exists.

## Private local is a product surface

Privacy does not mean hiding a signed-in user's own requested facts from that
user. A response explicitly marked `private_local` may include useful product
names, exact dates, card brands, and derived aggregates when the command needs
them. It must remain separate from shareable recaps and copy/share actions.

Credentials, cookies, OTPs, full account numbers, raw order/payment payloads,
and customer fixtures are never output. Logs and tests use redacted metadata or
synthetic data. Registered payment methods must not be presented as actual
order funding methods without transaction evidence.

Benefits and costs with different observation windows remain separate. Never
sum a short-window membership-benefit total and a longer card-reward history
into one headline unless their periods are aligned and overlap is excluded.

## Natural language outside, typed evidence inside

People should ask in ordinary language: “10만 원 아래, 후기 많은 맥북 허브
찾아줘.” The AI adapter translates that intent into bounded typed filters. The
core does not contain a brittle Korean sentence parser, and the Coupang adapter
does not decide what the user meant. It only returns observed product evidence
and explicit field coverage.

Search, inspection, and action remain separate steps. A search result is not
permission to mutate a cart. Cart addition requires an exact observed vendor
item and explicit user confirmation, is reported as a reversible external
state change, and is never automatically retried after an inconclusive result.
Checkout, order submission, and payment are permanent out-of-scope boundaries.

Ranking names must preserve source semantics. 쿠팡 랭킹순, 판매량순, and a
local review-count sort are different facts. Store the requested sort, applied
sort, scope, provenance, and source position. Never turn a product-page review
total into a variant-level review count or a 판매량 estimate. When several
options share one product page, collapse them by default and keep the exact
vendor-item identity for explicit inspection.

## Completion gate

A new metric is complete only when its typed response shape, provenance,
denominator, sample count, missing-data behavior, synthetic tests, and recap
wording all agree. A new source-native field is complete only after redacted
live metadata verifies its meaning and the endpoint catalog records it.
