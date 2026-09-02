# Viral personal shopping insights

Validated: 2026-09-02 (Asia/Seoul)

## Recommendation

Build a private, deterministic **shopping recap** from the local ledger before
building a social product. The first release should turn already normalized
orders into a short sequence of positive, explainable stories:

1. lifetime or selected-period scale;
2. shopping rhythm;
3. consistency and streaks;
4. delivery experience and its change over time;
5. one optional shopping-style persona;
6. a user-selected, privacy-safe summary card.

This sequence has more sharing potential than a dashboard full of totals. It
combines the recurring patterns used by successful first-party recap products:
ranked favorites, milestones, a temporal journey, a playful identity, and one
deliberate sharing moment. Exact spend, product names, exact dates, cancellation
or return behavior, and inferred household traits should remain private by
default.

Do not add global percentiles yet. Comparison is a proven sharing mechanism,
but `coupangctl` has no representative comparison population and should not
invent one. Self-comparison (this year versus last year, or early delivery
performance versus recent performance) is both honest and locally computable.

## What first-party recap products actually do

| Product | Concrete story patterns | Product lesson for `coupangctl` |
| --- | --- | --- |
| Spotify Wrapped | Total minutes; top songs, artists, and genres; a month-by-month “Top Artist Sprint”; streak/day callouts; a quiz; a global fan rank; a behavior-based Club and role; share cards for individual stories. Spotify also publishes eligibility rules and calculation details. ([2025 experience](https://newsroom.spotify.com/2025-12-03/2025-wrapped-user-experience/), [methodology](https://newsroom.spotify.com/2025-12-03/how-your-wrapped-is-made/)) | Mix familiar totals with a temporal race, one surprise, and a playful identity. Publish definitions and minimum-data rules instead of treating the calculation as magic. |
| Apple Music Replay | Monthly and annual top lists, totals, milestones, comparisons, highlight reels, top-listener tiers, and explicit user-initiated sharing. ([Apple Support](https://support.apple.com/en-ie/109356)) | Let the same typed facts power both recurring monthly insights and a richer annual/lifetime recap. Sharing must be an explicit action. |
| Duolingo Year-in-Review | Active days, time, words, lessons, percentile rank, and learner styles. Duolingo reports that percentile cards raised sharing, but top-10% learners produced over half the shares; adding non-performance learner styles created a second sharing opportunity and materially increased sharing. ([Duolingo's first-party retrospective](https://blog.duolingo.com/year-in-review-behind-the-scenes/)) | A leaderboard-only recap rewards a minority. Pair achievements with inclusive behavior personas so an ordinary user still gets a delightful story. |
| Strava Year in Sport | Personalized standout moments, streaks, social engagement, training partners, monthly stat cards, and per-scene or customizable summary sharing. Strava selects scenes according to available data and relevance rather than showing every metric to every user. ([Strava Help](https://support.strava.com/en-us/articles/15401959-your-year-in-sport)) | Rank candidate stories and show only those supported by enough data. Do not dump a fixed wall of metrics. |
| Google Photos Recap | Longest streak, most-photographed people, top colors, “vibes,” and memorable moments; optional Gemini captions for opted-in users. ([Google Photos 2024 Recap](https://blog.google/products-and-platforms/products/photos/google-photos-2024-recap/)) | “Unexpected but true” facts are more memorable than totals. Any AI-written narrative should be opt-in and grounded in deterministic facts. |
| Klarna Money Story | Highest-spend month, prominent purchase, top spending category, and a gamified spending-pattern story that leads into budgeting tools. ([Klarna Money Story](https://www.klarna.com/international/press/2023-spending-decoded-klarnas-money-story-guides-smarter-budgeting/)) | A commerce recap can connect a playful reflection to private utility such as budgeting. It must state which payment channel is covered and should not confuse observed spend with a complete financial picture. |
| Steam Replay and Xbox Year in Review | Year-over-year comparison, achievements, longest streak, monthly activity, genre mix, top month, top-three favorites, a behavior-based gamer profile, visual theming, and one-click sharing. ([Steam 2024 Replay](https://store.steampowered.com/news/posts/?enddate=1734641168&feed=dates), [Xbox 2024 Year in Review](https://news.xbox.com/en-us/2024/12/04/xbox-year-in-review-2024/)) | Combine scale, streak, mix, peak period, and a deterministic persona. A recap can adopt a theme from a safe favorite such as a category, without exposing a product. |
| Reddit Recap | Top communities, generated persona, rarity tier, downloadable card, and independent controls to hide username and avatar. Reddit also limited the recap dataset and disclosed its date/content boundary. ([Reddit Recap 2022](https://redditinc.com/news/reddit-recap-2022-product)) | Export controls should be field-level, and the card must state its period and inclusion rules. Replace any identifying field with a neutral label by default. |

Spotify's official fourth-quarter report says its 2025 campaign reached more
than 300 million engaged users and generated more than 630 million social shares
across 56 languages. This does not isolate which card caused each share, but it
does establish that the short personalized-story and share-card format operates
at genuine viral scale. ([Spotify Q4 2025](https://newsroom.spotify.com/2026-02-10/spotify-q4-2025-earnings/))

These examples support five recurring design principles:

1. **A journey beats a ledger.** Use a small story arc: scale, rhythm, peak,
   change, identity, share.
2. **Relative facts beat raw totals.** A longest streak, fastest period, or
   change from one's own baseline is easier to remember than an isolated sum.
3. **Identity is inclusive.** A deterministic “style” lets users participate
   without needing to be a top spender or globally ranked.
4. **Select, do not enumerate.** Eligibility thresholds and salience ranking
   prevent weak or misleading cards.
5. **Sharing is a separate privacy boundary.** The private recap can be rich;
   the exported card should contain only fields the user deliberately chose.

## Prioritized insight backlog

The priorities below refer to product value and data readiness, not to endpoint
discovery priority. “Current ledger” means a calculation over normalized order
and fulfillment fields; it does not require another private Coupang endpoint.

| Priority | Candidate story | Definition | Why it can travel | Data / caution |
| --- | --- | --- | --- | --- |
| P0 | **Shopping clock** | Peak purchase hour and weekday; optional night/weekend share of orders | A recognizable “night owl” or “Tuesday shopper” identity | Current ledger. Public card should show a broad time bucket, never an exact timestamp. |
| P0 | **Consistency streak** | Longest run of consecutive calendar months with at least one retained order | Positive, milestone-shaped, and not dependent on wealth | Current ledger. Also disclose active-month count and selected period. |
| P0 | **Shopping tempo** | Distinct order days, multi-order days, maximum orders on one day, average and longest gaps | Produces surprising “burst versus steady” stories | Current ledger. Hide the actual busiest date in share mode. |
| P0 | **Delivery speedrun** | Median delivery hours, share delivered within 24/48 hours, and early-versus-recent trend | A service experience story with a clear direction | Current ledger. Use shipment count and median/p90; never imply causality. Exclude invalid or canceled events and publish that definition. |
| P0 | **Month on fire** | Busiest month by order count; separately, highest-spend month | Temporal peak similar to top-month recap cards | Current ledger. The safe card names the month and count; exact spend is opt-in and must be labeled gross or settled. |
| P0 | **Brand loyalty** | Retained item-line share for the top brand, with a minimum sample | Familiar “top artist” pattern translated to shopping | Current ledger when brand is present. Brand name and percentage should each be independently removable. Never fall back to seller as if it were brand. |
| P0 | **Shopping style** | One deterministic archetype chosen from documented rules | Gives every eligible user a shareable identity | Current ledger. Examples: Night Cart, Steady Stocker, Weekend Runner, Fast-Lane Regular, Brand Loyalist. Avoid claims about age, gender, income, health, children, or household makeup. |
| P0 private-only | **Second-thought index** | Fully canceled order rate and returned-unit rate, optionally compared between time buckets | Interesting for self-reflection and quality control | Current ledger, but negative and potentially embarrassing. Do not include in the default public card. Keep cancellation and return denominators distinct. |
| P1 | **Category eras** | Monthly leader among authoritative product categories, visualized as rank changes | Direct shopping analogue of Spotify's monthly artist sprint | Requires provenance-labelled category enrichment. Do not infer authoritative categories from names. |
| P1 | **Explorer versus loyalist** | Category diversity plus concentration of retained spend or units | Inclusive identity independent of total spend | Requires categories. Prefer an explainable top-category share to an opaque ML label. |
| P1 | **Repeat rhythm** | Median interval and regularity for repeatedly purchased products | Highly relatable “you restock this every N days” fact | Product identity matching must be stable. Product names stay private by default; public mode can say “your most regular staple.” |
| P1 | **Return magnet / keeper** | Category-level returned-unit rate with minimum line/unit count | Useful and surprising when statistically supported | Requires categories and strong sample thresholds; private-only by default to avoid shaming brands or sellers from tiny samples. |
| P1 | **True wallet story** | Gross orders, fully canceled value, refunds, discounts, shipping, and settled net | High utility and strong headline value | Exact settled totals require receipt/refund contracts. Never label the current non-fully-canceled sum as exact post-refund net spend. |
| P2 | **Personal bests** | Largest improvement in delivery time, lowest cancellation period, or longest active streak versus one's own history | Achievement framing without a central comparison database | Require minimum comparable periods and sample sizes. Phrase as correlation/observation, not causal advice. |
| P2 | **Community percentile** | Opt-in cohort rank for a precisely defined metric | First-party products show that ranks drive sharing | Do not ship until there is a representative, consented, privacy-reviewed aggregation service. Never derive it from one account or a convenience sample. |

### Suggested deterministic archetype rules

Archetypes should be computed from versioned facts, not generated by an LLM.
Evaluate only rules whose minimum sample is met, score their strength above a
neutral threshold, and select the strongest; otherwise show “Your Year in
Orders” without a persona.

| Archetype | Candidate signal | Safe public wording |
| --- | --- | --- |
| Night Cart | Highest standardized deviation is the 00:00–05:59 order share | “Your cart wakes up after midnight.” |
| Steady Stocker | Long active-month streak and low variation in monthly order days | “Month after month, you kept the essentials moving.” |
| Weekend Runner | Weekend order share materially exceeds five-sevenths | “Weekends are your shopping lane.” |
| Fast-Lane Regular | 24-hour delivery share is the strongest supported delivery signal | “Your orders often arrived within a day.” |
| Brand Loyalist | Top-brand retained-line share clears a documented threshold | “One brand kept earning a place in your cart.” |
| Curious Cart | No dominant brand/category and high supported diversity | “Your cart likes to explore.” |

Do not present the archetype as a psychological diagnosis. Include a machine-
readable `rule_version`, the facts used, and the eligibility threshold in the
structured response so the card can always be explained.

## Story-selection model

Generate many deterministic candidates, then select a small set. A simple
scoring model is sufficient:

```text
story_score = confidence * surprise * positivity * share_safety
```

- `confidence`: sample size, coverage, and data provenance;
- `surprise`: normalized distance from the user's own baseline, not from other
  users;
- `positivity`: prefer celebratory or neutral framing in the public sequence;
- `share_safety`: 1.0 for coarse aggregate facts, lower for exact amounts,
  names, dates, returns, or behavior that could reveal private circumstances.

Use deterministic tie-breaking so the same snapshot produces the same recap.
Return the unselected candidates and rejection reasons in debug JSON, not in
the normal CLI output. A practical default is five story cards plus one summary
card. Strava's first-party documentation supports relevance-based scene
selection, and Spotify publishes explicit story eligibility thresholds rather
than forcing every story onto every user.

## Privacy and truthfulness requirements

Shopping history can reveal health conditions, religion, relationships,
children, location, income, and presence at home. Its safe sharing boundary
must therefore be stricter than music or gaming recaps.

- Keep analysis local. Generating JSON or an image must not upload ledger data.
- Make export/share an explicit user action. The first render is private.
- Provide `safe` and `detailed` export modes. `safe` is the default.
- In `safe`, omit exact spend, product and seller names, order references,
  product IDs, exact dates/times, receipt data, delivery addresses, and account
  identity. Strip image metadata as well.
- Let users independently include or exclude brand/category labels, exact
  totals, persona, and comparison text. Reddit's official recap offered
  independent identity/avatar controls; this is the correct interaction model,
  not a single all-or-nothing switch.
- Let users exclude orders or correct category/persona inputs and regenerate.
  Amazon's first-party “About You” control lets customers view, update, or
  remove preferences derived from shopping activity; an analytics product
  should provide equivalent agency over the story it tells
  ([Amazon About You](https://www.aboutamazon.com/news/retail/amazon-about-you-personalization-preferences)).
- Never place cancellation or return metrics on the default share card.
- State period, timezone, last-sync time, coverage, unit, and denominator. A
  “rate” without its denominator is not a supported product fact.
- Distinguish gross ordered value, fully canceled value, non-fully-canceled
  value, refunds, shipping, and exact settled net. If settlement data is absent,
  say so in the response and card preview.
- Require minimum samples: at least 3 orders for a recap, 5 retained item lines
  for brand/category concentration, and 10 valid delivery events per period for
  a trend. These are conservative product defaults to validate, not claims from
  the cited products.
- Do not infer sensitive traits or use product-name text to label health,
  religion, political interest, pregnancy, sexuality, or household members.
- Any future AI-written caption must be separately opted in, receive only the
  selected aggregate facts, show those supporting facts, and be removable.
  Google Photos' first-party recap made its Gemini captions opt-in, while the
  deterministic insights remained available separately.
- Never implement a group comparison mode without an explicit disclosure that
  every participant can see and re-share the selected facts. Spotify gives
  that warning for Wrapped Party, including profile information
  ([official Wrapped Party guide](https://newsroom.spotify.com/2025-12-03/wrapped-party-how-to/)).

## Proposed response boundary

Keep the typed core independent from presentation. The core should return
facts, eligibility, provenance, and sensitivity; CLI and MCP adapters can
render or expose the same response without duplicating business rules.

```json
{
  "schema_version": 1,
  "period": {"from": "YYYY-MM-DD", "to": "YYYY-MM-DD", "timezone": "Asia/Seoul"},
  "coverage": {"complete": true, "orders": 0, "last_synced_at": "RFC3339"},
  "stories": [
    {
      "kind": "active_month_streak",
      "title_key": "recap.active_month_streak.title",
      "facts": {"months": 0},
      "eligibility": {"met": true, "minimum_orders": 3},
      "provenance": "local_order_ledger",
      "sensitivity": "safe_aggregate",
      "share_default": true
    }
  ],
  "omissions": [
    {"kind": "settled_net_spend", "reason": "refund_settlement_unavailable"}
  ]
}
```

Do not put fully composed prose in the typed core. Stable `kind`, `title_key`,
and numeric facts allow deterministic localization, tests, CLI JSON, MCP
responses, and future share-card renderers to evolve independently.

## Build order

1. Ship `orders insights` as structured JSON with current-ledger P0 facts,
   explicit definitions, sample counts, and no share renderer.
2. Add deterministic story eligibility/ranking and the versioned archetype
   rules, with synthetic fixtures for sparse, typical, and edge-case histories.
3. Add a local `recap` command that selects five stories plus one summary.
4. Add opt-in image export with `safe` as the default and a preview of exactly
   what will be exposed.
5. Add authoritative category enrichment and only then category eras,
   diversity, and category-level return analysis.
6. Add receipt/refund settlement before any “true net spend” headline.
7. Consider opt-in aggregate percentiles only after a separate privacy and
   representativeness design; this is not required for a valuable local recap.

The key product bet is not “copy Spotify Wrapped.” It is: **make private
shopping data feel like a truthful personal story, then let the user choose one
safe fragment to share.**
