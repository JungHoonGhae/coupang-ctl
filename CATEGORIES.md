# 카테고리 데이터 계약

`coupangctl`은 상품명으로 카테고리를 추측하지 않습니다. 동기화된 주문 상품의
쿠팡 상품 페이지에서 JSON-LD `BreadcrumbList`가 확인된 경우에만 숫자 ID,
이름, 위치, 전체 경로를 저장합니다.

## 카탈로그 조회

```bash
coupangctl orders categories --max-products 25
coupangctl orders categories --max-products 25 --recheck
coupangctl orders category-catalog --query '생활용품' --limit 50
coupangctl orders category-stability
coupangctl products search --category-id 123456 --sort sales
```

기본 `orders categories`는 아직 확인하지 않은 주문 상품을 제한된 개수만큼
보강합니다. `orders category-catalog`은 로컬 SQLite만 읽으며 브라우저나
네트워크를 호출하지 않습니다. 반환된 `category_id`는 `products search`의
현재 쿠팡 카테고리 검색에 넘길 수 있습니다.

`--recheck`는 명시적으로 요청한 경우에만 이미 캐시된 상품을 `max-products`
개까지 가장 오래 확인한 순서로 다시 읽습니다. 최신 캐시는 갱신하지만 이전
경로와 확인 시각은 append-only 관측으로 남깁니다. 응답의
`recheck_candidate_count`가 분모이고, 제한 때문에 일부만 처리했으면
`recheck_truncated=true`, `complete=false`입니다.

## 응답 shape

카탈로그 schema version은 `1`, visibility는 `private_local`입니다.

- `source`: `coupang_product_jsonld_breadcrumb_v1`
- `coverage`: 카테고리 확인 대상 distinct 주문 상품 수, 분류/미분류 수와 비율
- `total_category_count`: 현재 원장에서 관찰한 고유 경로 prefix 수
- `matched_category_count`: 선택적 `query`가 이름에 일치한 수
- `returned_category_count`, `truncated`: `limit` 적용 결과
- `categories`: 관찰된 `category_id`, `name`, 원본 `position`, 경로 내 `depth`,
  해당 prefix의 전체 `path`, 로컬 상품 수, leaf/상위 노드로 관찰된 상품 수,
  `role`, `match_kind`
- `provenance`: ID·이름·경로는 `observed`, 로컬 상품 수와 query match는 `derived`

`query`는 관찰된 카테고리 이름에만 대소문자 무시 exact → prefix → substring
순으로 대응합니다. 상품명, 브랜드, 임의 사전은 보지 않습니다.

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "source": "coupang_product_jsonld_breadcrumb_v1",
  "query": "Synthetic broad",
  "match_method": "case_insensitive_label_exact_prefix_substring",
  "coverage": {
    "eligible_product_count": 3,
    "classified_product_count": 2,
    "unclassified_product_count": 1,
    "classified_product_rate": 0.666667
  },
  "total_category_count": 3,
  "matched_category_count": 1,
  "returned_category_count": 1,
  "truncated": false,
  "categories": [
    {
      "category_id": "100",
      "name": "Synthetic broad",
      "position": 2,
      "depth": 1,
      "role": "never_leaf",
      "path": [{"id": "100", "name": "Synthetic broad", "position": 2}],
      "observed_product_count": 2,
      "observed_leaf_product_count": 0,
      "observed_ancestor_product_count": 2,
      "match_kind": "exact_label"
    }
  ]
}
```

`observed_product_count`는 내 로컬 주문 원장에서 해당 breadcrumb prefix가
관찰된 distinct `vendor_item_id` 수입니다. 쿠팡 전체 판매량, 인기도, 공식
taxonomy의 완전성을 뜻하지 않습니다. 쿠팡이 경로나 이름을 바꾸거나 현재 해당
카테고리에 상품이 없을 수 있으므로 최종 가용성과 순서는 `products search`의
현재 응답으로 확인합니다.

## 경로 안정성 조회

`orders category-stability`와 MCP `orders_category_stability`는 네트워크를
호출하지 않고 저장된 관측만 읽습니다. schema version은 `1`, visibility는
`private_local`입니다.

- `eligible_product_count`, `observed_product_count`, `observed_product_rate`:
  로컬 원장과 source-native breadcrumb 관측의 커버리지
- `rechecked_product_count`: 유효한 경로 관측이 두 번 이상인 동일 상품 수
- `multi_day_rechecked_product_count`: 같은 상품을 서로 다른 UTC 날짜에
  관측한 수
- `stable_product_count`, `changed_product_count`: 재관측 상품 중 고유 경로가
  하나인 수와 둘 이상인 수
- `observation_count`, `distinct_observation_day_count`, 최초·최근 시각:
  판정에 사용한 관측 범위
- `assessment`: `unavailable_no_observed_breadcrumbs`,
  `insufficient_rechecks`, `insufficient_distinct_days`, `changes_observed`,
  `stable_within_local_observation_window` 중 하나
- `provenance`: 경로와 시각은 `observed`, 집계와 판정은 `derived`

`stable_within_local_observation_window`는 동일 상품의 multi-day 표본 안에서
변화가 없었다는 뜻일 뿐입니다. 한 계정만으로 다른 계정이나 쿠팡 전체의
taxonomy 안정성을 주장하지 않으며, missing/unavailable 결과도 안정적인
경로로 세지 않습니다.
