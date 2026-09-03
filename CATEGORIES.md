# 카테고리 데이터 계약

`coupangctl`은 상품명으로 카테고리를 추측하지 않습니다. 동기화된 주문 상품의
쿠팡 상품 페이지에서 JSON-LD `BreadcrumbList`가 확인된 경우에만 숫자 ID,
이름, 위치, 전체 경로를 저장합니다.

## 카탈로그 조회

```bash
coupangctl orders categories --max-products 25
coupangctl orders category-catalog --query '생활용품' --limit 50
coupangctl products search --category-id 123456 --sort sales
```

첫 명령은 아직 확인하지 않은 주문 상품을 제한된 개수만큼 보강합니다. 두 번째
명령은 로컬 SQLite만 읽으며 브라우저나 네트워크를 호출하지 않습니다. 반환된
`category_id`는 세 번째 명령의 현재 쿠팡 카테고리 검색에 넘길 수 있습니다.

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
