# 영수증 계약

`coupangctl`의 영수증 기능은 쿠팡 영수증 화면의 구조화된 읽기 응답을
typed core로 정규화합니다. 모든 결과는 `private_local`이며 원문 응답,
카드 식별자·번호, 계정 식별자, 다운로드 URL을 포함하지 않습니다.

## 지원 범위

| 작업 | CLI | MCP | 외부 상태 변경 |
| --- | --- | --- | --- |
| 현금·카드 요청 상태 | `receipts status` | `receipts_status` | 없음 |
| 요청 이력 | `receipts list` | `receipts_list` | 없음 |
| 기간·결제수단 합계 | `receipts summary` | `receipts_summary` | 없음 |
| 주문별 판매자 영수증 | `receipts vendor` | `receipts_vendor` | 없음 |
| 완료 파일 저장 | `receipts download` | 지원하지 않음 | 없음 |
| 새 영수증 묶음 생성 요청 | 지원하지 않음 | 지원하지 않음 | 구현 제외 |

기간 합계는 한 번에 최대 366일, 이력은 한 페이지 최대 50개입니다. 카드
합계 조회에 필요한 비공개 카드 키는 좁은 Coupang adapter 안에서만 사용한
뒤 폐기합니다.

판매자 영수증은 `orders list`의 SHA-256 `source_ref`를 입력으로 받습니다.
원주문 ID는 브라우저 adapter 안에서만 찾아 GET 경로에 사용하고 즉시
폐기합니다. 기본 `max_pages=1000`은 검색 안전 상한이지 이력을 1000페이지로
잘라 저장한다는 뜻이 아닙니다.

## 응답 형태

아래 예시는 모두 합성 데이터입니다. 실제 값이나 계정 fixture가 아닙니다.

### 상태

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "fetched_at": "2026-09-03T00:00:00Z",
  "statuses": [
    {
      "kind": "cash",
      "request_in_progress": false,
      "can_request_new": true,
      "provenance": "observed"
    }
  ],
  "definitions": {
    "source": "coupang_payment_receipt_read_endpoints",
    "provenance": "observed",
    "payment_privacy": "card identifiers and card numbers are discarded before the typed response",
    "download_privacy": "download URLs are used in memory and never returned or logged"
  }
}
```

### 이력

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "kind": "card",
  "page_index": 0,
  "page_size": 5,
  "has_next": false,
  "items": [
    {
      "history_index": 0,
      "requested_at": "2026.09.01",
      "from": "2026.08.01",
      "to": "2026.08.31",
      "total_count": 3,
      "total_amount_krw": 42000,
      "payment_method_display": "합성 카드",
      "status": "COMPLETED",
      "download_count": 1,
      "provenance": "observed"
    }
  ]
}
```

`history_index`와 `download_count`는 사용자가 CLI 다운로드 대상을 명시하기
위한 안전한 위치 정보입니다. 원본 URL은 응답에 없습니다.

### 기간 합계

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "kind": "card",
  "from": "2026.08.01",
  "to": "2026.08.31",
  "total_count": 3,
  "total_amount_krw": 42000,
  "payment_methods": [
    {
      "display_name": "합성 카드",
      "total_count": 3,
      "total_amount_krw": 42000,
      "provenance": "derived_from_observed_receipt_summaries"
    }
  ],
  "installments": {
    "status": "unavailable",
    "limitations": [
      "the verified sales-slip summary exposes payment-method totals but no installment-month field"
    ]
  },
  "warnings": []
}
```

동일한 표시명이 여러 비공개 카드 항목에 쓰이면 표시명별 건수와 금액을
합산합니다. 그래서 결제수단 행의 provenance는 `derived_from_observed_receipt_summaries`입니다. 서로 같은 실제 카드라고 추론하지 않습니다. 할부 여부는 명시적인 원천 필드가 발견되기
전까지 항상 `unavailable`입니다.

응답의 `from`과 `to`는 호출자가 요청한 ISO 날짜입니다. 원천 응답이 다른
기간을 되돌려주면 합계 자체를 숨기지 않고 `warnings`에 불일치를 표시합니다.

### 주문별 판매자 영수증

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "source_ref": "<synthetic-sha256>",
  "pages_scanned": 2,
  "vendor_count": 1,
  "vendors": [
    {
      "vendor_index": 0,
      "vendor_name": "합성 판매자",
      "main_payment_type_name": "합성 카드",
      "main_payment_amount_krw": 12000,
      "payment": {
        "original_payment_amount_krw": 12000,
        "original_payment_canceled_amount_krw": 3000,
        "coupon_discount_amount_krw": 3000,
        "coupon_discount_canceled_amount_krw": 1000
      },
      "products": [
        {
          "product_index": 0,
          "name": "합성 상품",
          "quantity": 2,
          "canceled_quantity": 1,
          "unit_price_krw": 7500
        }
      ]
    }
  ],
  "installments": {"status": "unavailable"},
  "settlement": {"status": "source_components_observed"}
}
```

`original_payment_canceled_amount_krw` 같은 필드는 쿠팡 응답의 취소 결제
구성요소를 이름 그대로 보존한 관찰값입니다. 할인 배분, 포인트, 배송비,
후속 조정과 환불 완료 상태까지 합의된 계약이 아니므로 현재는 “실제 환불액”
또는 “정확한 순지출”로 재명명하거나 자동 합산하지 않습니다.

### 다운로드 결과

```json
{
  "schema_version": 1,
  "visibility": "private_local",
  "kind": "card",
  "history_index": 0,
  "download_index": 0,
  "filename": "synthetic-receipt.pdf",
  "content_type": "application/pdf",
  "bytes": 1024,
  "output_path": "/private/local/path/synthetic-receipt.pdf"
}
```

파일은 사용자가 지정한 새 경로에 권한 `0600`으로 생성합니다. 기존 파일은
덮어쓰지 않고, 실패한 부분 파일은 제거합니다. 브라우저 adapter는 선택한
이력 행을 다시 읽고 HTTPS Coupang 호스트와 최종 리디렉션 호스트, 콘텐츠
종류, 최대 크기를 확인합니다.

## 증거와 제한

- `provenance: observed`는 영수증 읽기 응답에서 직접 확인한 값입니다.
- 표시명별 합계처럼 관찰값을 더한 결과는 규칙을 위에 공개합니다.
- 새 archive 요청 POST는 지원하지 않습니다. 판매자 영수증은 검증된 단건 GET만 지원합니다.
- 완료 파일 다운로드는 합성 계약 테스트를 통과했지만, 라이브 계정의 현재
  이력이 비어 있어 실제 완료 파일 검증은 아직 남아 있습니다.
- 비공개 endpoint는 불안정할 수 있으므로 모두 `internal/coupang/receipts`
  adapter 뒤에 격리합니다.
