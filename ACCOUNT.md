# 계정·멤버십 응답 계약

`coupangctl account benefits`와 MCP `account_benefits`는 같은
`private_local` schema version 3 응답을 사용합니다. 계정 상태를 바꾸지 않는
조회이며 OTP, 쿠키, 계정번호, 카드번호, raw 주문·캐시 거래를 반환하지 않습니다.

`membership.current_monthly_fee_krw`와 `membership.source_fee_change_date`는
멤버십 화면에서 관찰한 현재 요금·원천 변경일 metadata입니다. 변경일은 과거 요금,
실제 청구일, 변경 전후 금액을 뜻하지 않습니다. 따라서 이 필드만으로 역사적 회비를
재구성하지 않습니다.

## 멤버십 비용

`membership_costs`의 첫 번째 후보 출처는 로컬 normalized order ledger입니다. 한 주문에
항목이 하나 이상 있고 모든 항목의 원천 metadata가 `membership_fee`로 명시된
경우만 포함합니다. 상품명, 금액, 결제 주기로 멤버십을 추정하지 않습니다.

```json
{
  "status": "complete_available_history",
  "source": "normalized_order_ledger_explicit_membership_metadata",
  "provenance": "derived",
  "observed_payment_count": 13,
  "observed_gross_amount_krw": 102570,
  "observed_non_canceled_payment_count": 12,
  "observed_paid_amount_krw": 94680,
  "first_observed_payment_date": "2025-08-01",
  "last_observed_payment_date": "2026-07-01",
  "last_complete_history_sync_at": "2026-09-03T01:00:00Z",
  "complete_history_sync": true,
  "limitations": ["membership charges absent from the source order history cannot be recovered"]
}
```

- `observed_gross_amount_krw`: 취소 여부와 관계없이 관찰된 멤버십 주문 합계
- `observed_paid_amount_krw`: 완전 취소된 멤버십 주문을 제외한 합계
- `complete_history_sync`: 최신 sync run이 끝까지 도달했고 이어받기 checkpoint가
  남아 있지 않을 때만 `true`
- import한 데이터나 page budget 중간에 멈춘 sync는 `partial_history`

완전 동기화는 “현재 쿠팡 주문 원천이 제공하는 모든 페이지를 읽었다”는 뜻입니다.
원천 자체에 보존되지 않은 과거 결제나 별도 환불 정산까지 복원한다는 뜻은 아닙니다.
실계정 전체 동기화에서는 멤버십을 구별할 source enum이 없었습니다. 현재 유료
회원인데 명시적 멤버십 행이 0개라면 status는
`unavailable_no_explicit_membership_order_metadata`이며, 0원 지불로 해석하면 안
됩니다.

## 혜택 대비 비용

`benefit_usage.total_observed_savings_krw`는 쿠팡 멤버십 관리 화면의 관찰값입니다.
headed metadata-only 검증에서 화면의 `최근 3개월` 문구를 확인했습니다. 이 경우
`window_status: observed`, `window_kind: rolling_recent_months`,
`window_months: 3`을 반환합니다.

같은 기간의 실제 회비 영수증이 아직 없으므로 비교 비용은
`current_monthly_fee_krw * window_months`입니다. 결과는 각각
`estimated_membership_fee_krw`, `estimated_net_value_krw`에 들어가고 status는
`estimated_current_fee_window`, provenance는 `inferred`입니다. 중지·환불·무료기간·
결제 실패·기간 중 요금 변경을 반영한 실제 납부액이 아닙니다. 그래서
`confirmed_net_value_krw`는 채우지 않고 `missing_evidence`에
`actual_membership_payments_for_benefit_window`를 남깁니다.
`source_fee_change_date`가 존재하면 응답의 `definitions.membership_fee`와
`net_value.limitations`도 이 관찰값과 실제 결제 증거의 차이를 명시합니다.

와우카드 적립, 카드 연회비, 등록 결제수단은 각각 별도 관찰 기간과 의미를 가지므로
이 멤버십-only 계산에 합치지 않습니다. 등록된 카드는 실제 주문 결제수단의 증거도
아닙니다.

관련 원천은 쿠팡의 [와우 멤버십 FAQ](https://news.coupang.com/archives/64216/)와
[월회비 변경 안내](https://news.coupang.com/archives/44584/)입니다. FAQ는 월회비
현금영수증을 PC의 `마이쿠팡 → MY쇼핑 → 영수증 조회/출력`에서 확인하도록
안내합니다. endpoint와 필드의 실제 채택 여부는 이 공개 안내가 아니라 별도의
redacted live metadata 검증으로 결정합니다.
