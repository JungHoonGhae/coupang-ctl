# coupangctl shopping type system

The recap uses four deterministic binary axes. It is a shopping-behavior game,
not a personality test or psychological diagnosis. Each result exposes its
metric, threshold, numerator, denominator, sample size, observation window,
provenance, and rule version through `orders insights`.

## Axes

| Axis | Poles | Metric | Minimum usable data | Selection rule |
| --- | --- | --- | --- | --- |
| Rhythm | `S` steady / `B` burst | concentration of retained purchase days across the observed date window | 20 purchase days and 180 observed days | `B` when concentration is at or above the median of 512 deterministic uniform simulations using the same number of purchase days and the same window; otherwise `S` |
| Clock | `D` daytime / `N` night | share of timed retained orders placed from 20:00 through 05:59 Asia/Seoul | 20 timed orders and 90 observed days | `N` at 50% or above; otherwise `D` |
| Choice | `F` first choice / `R` repeat choice | repeat selections divided by identified retained purchase occasions | 20 occasions, 10 retained orders, 180 observed days, and 70% product-ID coverage | `R` at 50% or above; otherwise `F` |
| Basket | `T` together / `O` one product | share of composition-valid retained orders containing one distinct product | 10 composition-valid orders | `O` at 50% or above; otherwise `T` |

`shopping_profile_v4` fixes these definitions. A missing minimum produces `?`
for that axis instead of inventing a type. The 50% rules mean literal majority,
not a population norm. Rhythm uses a same-sample null model because a fixed
percentage would change meaning with history length and purchase frequency.

`repeat choice` counts every identified purchase occasion after the first for a
stable product ID. Basket composition uses distinct stable product IDs and only
includes orders whose retained lines are all identifiable. Spend, cancellation,
returns, brands, and delivery performance do not determine the four-letter code.

## Sixteen characters

Letter order is always rhythm, clock, choice, basket. Names are playful summaries
of the observed poles, not claims about identity or motivation.

| Code | Character | Plain-language pattern |
| --- | --- | --- |
| `SDFO` | 햇살 한 봉지 참새 | 구매일이 고르게 퍼지고, 낮 주문과 첫 선택, 한 상품 주문이 많음 |
| `SDFT` | 햇살 합배송 카피바라 | 구매일이 고르게 퍼지고, 낮 주문과 첫 선택, 여러 상품 주문이 많음 |
| `SDRO` | 낮 단골 다람쥐 | 구매일이 고르게 퍼지고, 낮 주문과 반복 선택, 한 상품 주문이 많음 |
| `SDRT` | 낮 비축 거북이 | 구매일이 고르게 퍼지고, 낮 주문과 반복 선택, 여러 상품 주문이 많음 |
| `SNFO` | 야밤 새것 부엉이 | 구매일이 고르게 퍼지고, 밤 주문과 첫 선택, 한 상품 주문이 많음 |
| `SNFT` | 야밤 합배송 부엉이 | 구매일이 고르게 퍼지고, 밤 주문과 첫 선택, 여러 상품 주문이 많음 |
| `SNRO` | 야밤 또삼 다람쥐 | 구매일이 고르게 퍼지고, 밤 주문과 반복 선택, 한 상품 주문이 많음 |
| `SNRT` | 야밤 단골 거북이 | 구매일이 고르게 퍼지고, 밤 주문과 반복 선택, 여러 상품 주문이 많음 |
| `BDFO` | 번개 한 봉지 까치 | 구매일이 몰리고, 낮 주문과 첫 선택, 한 상품 주문이 많음 |
| `BDFT` | 우르르 카피바라 | 구매일이 몰리고, 낮 주문과 첫 선택, 여러 상품 주문이 많음 |
| `BDRO` | 번개 또삼 햄스터 | 구매일이 몰리고, 낮 주문과 반복 선택, 한 상품 주문이 많음 |
| `BDRT` | 낮 비축 곰 | 구매일이 몰리고, 낮 주문과 반복 선택, 여러 상품 주문이 많음 |
| `BNFO` | 새벽 번개 라쿤 | 구매일이 몰리고, 밤 주문과 첫 선택, 한 상품 주문이 많음 |
| `BNFT` | 새벽 우르르 해파리 | 구매일이 몰리고, 밤 주문과 첫 선택, 여러 상품 주문이 많음 |
| `BNRO` | 새벽 또삼 박쥐 | 구매일이 몰리고, 밤 주문과 반복 선택, 한 상품 주문이 많음 |
| `BNRT` | 새벽 비축 박쥐 | 구매일이 몰리고, 밤 주문과 반복 선택, 여러 상품 주문이 많음 |

The typed core emits the stable code, axis evidence, badge IDs, and numeric
values. Character names, Korean copy, and vector artwork belong to the recap
adapter so visual or localization changes do not break CLI and MCP contracts.
