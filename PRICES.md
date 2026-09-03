# 가격 이력과 재구매 비교 계약

`coupangctl`은 성공한 상품 검색·상세 조회에서 명시적으로 관찰한 현재가만
로컬에 기록합니다. 제휴 URL, 쿠키, 계정 정보는 저장하지 않습니다.

## 관찰 규칙

- 상품 응답의 `observed_fields`에 `price.current_amount`가 있고 값이 양수일
  때만 기록합니다.
- `vendor_item_id`가 있으면 옵션별 series, 없으면 `product_id` series를
  사용합니다. 서로 다른 series의 가격을 한 추세로 합치지 않습니다.
- 검색 결과에서는 필터·옵션 축약·limit를 거쳐 실제 반환된 상품만 기록합니다.
- 같은 series, 시각, source의 중복 읽기는 한 번만 저장합니다.
- 이력은 첫 로컬 관찰부터 시작하며 과거 쿠팡 가격을 소급하지 않습니다.

## 가격 이력 응답

아래 값은 모두 합성 예시입니다.

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "product_id": "101",
  "vendor_item_id": "201",
  "observation_count": 2,
  "series_count": 1,
  "first_returned_at": "2026-09-01T00:00:00Z",
  "last_returned_at": "2026-09-03T00:00:00Z",
  "series": [
    {
      "identity": "vendor:201",
      "reference": {"product_id": "101", "vendor_item_id": "201"},
      "latest_name": "Synthetic product",
      "canonical_url": "https://www.coupang.com/vp/products/101",
      "observations": [
        {
          "reference": {"product_id": "101", "vendor_item_id": "201"},
          "name": "Synthetic product",
          "current_amount": 42000,
          "currency": "KRW",
          "observed_at": "2026-09-01T00:00:00Z",
          "source": "coupang_product_search",
          "provenance": "observed"
        },
        {
          "reference": {"product_id": "101", "vendor_item_id": "201"},
          "name": "Synthetic product",
          "current_amount": 39000,
          "currency": "KRW",
          "observed_at": "2026-09-03T00:00:00Z",
          "source": "coupang_product_inspection",
          "provenance": "observed"
        }
      ],
      "trend": {
        "observation_count": 2,
        "first_returned_amount_krw": 42000,
        "latest_amount_krw": 39000,
        "minimum_amount_krw": 39000,
        "maximum_amount_krw": 42000,
        "change_from_first_returned_krw": -3000,
        "change_from_first_returned_percent": -7.14,
        "direction": "lower",
        "provenance": "derived_from_observed_prices_within_one_product_identity"
      }
    }
  ],
  "coverage": {
    "returned_observations": 2,
    "limit": 200,
    "truncated": false
  },
  "warnings": []
}
```

`--vendor-item-id`를 생략하면 관찰된 옵션을 별도 series로 모두 반환합니다.
전체 반환 개수는 `--limit`으로 제한되고, 더 오래된 값이 빠지면
`coverage.truncated`와 warning이 함께 표시됩니다.

## 재구매 가격 비교

`orders reorder`는 정규화 주문의 정확한 상품 identity로 구매 횟수와 보유
수량을 계산합니다. 가격 비교는 다음 두 근거가 모두 있을 때만 제공됩니다.

1. 취소·반품이 없고 결제액과 수량이 양수인 가장 최근 주문 행의 결제 단가
2. 같은 identity에 저장된 가장 최근 상품 가격 관찰

```json
{
  "status": "available",
  "last_paid_unit_amount_krw": 42000,
  "last_paid_at": "2026-08-01",
  "latest_observed_amount_krw": 39000,
  "observed_at": "2026-09-03T00:00:00Z",
  "observation_age_hours": 3,
  "freshness": "recent_under_24h",
  "difference_krw": -3000,
  "difference_percent": -7.14,
  "direction": "lower",
  "provenance": "derived_from_normalized_paid_unit_and_latest_observed_exact_identity_price",
  "limitations": [
    "the latest observation is not a guaranteed checkout price; verify options and promotions on the final product page"
  ]
}
```

가격 관찰이 없으면 `unavailable_no_local_price_observation`, 안전한 마지막
결제 단가가 없으면 `unavailable_missing_paid_unit_evidence`입니다. 상품명
유사도나 다른 옵션 가격으로 빈 값을 채우지 않습니다. `orders reorder`는
네트워크를 호출하지 않으므로 오래된 관찰일 수 있습니다.
24시간 미만은 `recent_under_24h`, 그 이상은 `stale_24h_or_more`로
표시하며 오래된 관찰에는 새 상세 조회가 필요하다는 limitation을 추가합니다.

## Watchlist와 반복 갱신

이미 관찰한 정확한 identity만 watchlist에 등록할 수 있습니다. 이름 검색으로
대상을 추측하지 않습니다.

```bash
coupangctl products watch-add --product-id ID --vendor-item-id ID
coupangctl products watch-list
coupangctl products watch-refresh --limit 20 --stale-hours 24
coupangctl products watch-remove --product-id ID --vendor-item-id ID
```

`watch-add`와 `watch-remove`는 `changed`와 대상 `entry`를 반환합니다.
`watch-list`는 다음과 같은 bounded local 목록입니다.

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "count": 1,
  "items": [
    {
      "identity": "vendor:201",
      "reference": {"product_id": "101", "vendor_item_id": "201"},
      "name": "Synthetic product",
      "canonical_url": "https://www.coupang.com/vp/products/101",
      "added_at": "2026-09-03T00:00:00Z",
      "last_status": "pending"
    }
  ]
}
```

`watch-refresh`는 `last_checked_at`이 없거나 기준 시간보다 오래된 항목을 최대
`limit`개 조회합니다. 각 시도는 `observed`, `unavailable`, `failed` 중 하나로
기록되어 같은 실패를 즉시 반복하지 않습니다. 이 명령은 운영체제의 cron,
systemd timer, CI scheduler에서 실행할 수 있는 구조화 JSON 명령입니다.

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "attempted": 2,
  "observed": 1,
  "unavailable": 1,
  "failed": 0,
  "remaining_due": 0,
  "items": [
    {
      "reference": {"product_id": "101", "vendor_item_id": "201"},
      "status": "observed",
      "checked_at": "2026-09-03T00:00:00Z",
      "provenance": "observed"
    }
  ]
}
```

갱신은 공개 상세 읽기와 로컬 기록만 수행합니다. 제휴 링크 변환, 장바구니,
주문, 결제는 호출하지 않습니다. `product_watch_add`,
`product_watch_remove`, `product_watch_refresh`는 MCP에서도 로컬 변경 여부가
annotation에 표시됩니다.

## 삭제

`products price-history-purge --confirm purge-product-price-history`는 가격
관찰만 삭제하고 watchlist는 유지합니다. watchlist 전체 삭제는
`products watch-clear --confirm clear-product-watchlist`를 사용합니다. 두
명령 모두 주문 원장이나 인증 세션은 건드리지 않으며 MCP에는 일괄 삭제
도구를 노출하지 않습니다.
