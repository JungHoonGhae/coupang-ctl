# 계정·멤버십 응답 계약

`coupangctl account benefits`와 MCP `account_benefits`는 같은
`private_local` schema version 6 응답 계약을 사용합니다. 계정 상태를 바꾸지 않는
조회이며 OTP, 쿠키, 계정번호, 카드번호, raw 주문·캐시 거래를 반환하지 않습니다.

전용 Camofox의 로그인 세션으로 멤버십 화면과 캐시 조회를 headless로 읽습니다.
일상용 브라우저를 연결하거나 창을 활성화하지 않습니다. `--cash-pages N`으로
캐시 조회를 1–100페이지로 제한하며 기본값은 50입니다. 원천이 종료를 표시하면
그 전에 멈춥니다. 로그인·접근 제한·구조 오류는 실패로 반환하며 다른 브라우저로
전환하지 않습니다. `account benefits --help`에는 브라우저 설정이 필요 없습니다.

2026-09-11 개발 바이너리에서 CLI 2페이지와 MCP 1페이지 조회를 각각 확인했습니다.
두 실행 모두 회비·혜택 합계·예상 적립금의 관측 여부와 `private_local` 응답을
검증했습니다. 캐시 이력은 둘 다 `partial`이었으며 전체 이력 확보나 실제 납부 회비,
확정 순혜택을 검증한 결과가 아닙니다. 로그에는 상태·페이지 수·필드 관측 여부만
남겼습니다. 릴리스 패키지 검증과도 구분합니다.

v6 변경 후에도 CLI와 MCP에서 각각 1페이지씩 두 차례 재조회해 통과했습니다.
한 차례는 세부 혜택 17개 필드의 존재·정수 타입을 확인했고, 최종 변경 후에는
회원·자동결제 선택 필드의 타입과 페이지 범위를 재검증했습니다. 이 표본에는
세부 혜택 필드 누락이 없었습니다. 누락·`null`·0·false 및 잘못된 값의 처리는
브라우저 reader·파서·CLI·MCP 합성 테스트로 검증했습니다.

`membership.current_monthly_fee_krw`와 `membership.source_fee_change_date`는
멤버십 화면에서 관찰한 현재 요금·원천 변경일 metadata입니다. 변경일은 과거 요금,
실제 청구일, 변경 전후 금액을 뜻하지 않습니다. 따라서 이 필드만으로 역사적 회비를
재구성하지 않습니다.

## 확인된 0과 자료 없음

회비·혜택 합계·예상 적립금은 필드가 있고 값이 유효할 때만 반환합니다.
확인된 0원은 `0`으로 남깁니다. 자료가 없거나 금액의 통화가 불명확하면
그 금액 필드를 생략합니다. 클라이언트는 생략된 값을 0원으로 채우면 안 됩니다.

v6에서는 세부 혜택 금액·이용 횟수, 가입 일수, 회원 여부·유료/체험/중지 여부,
등록 결제수단의 자동결제 여부도 각각 독립된 선택 필드입니다. 원천에서 빠졌거나
`null`이면 응답에서 생략하고 명시된 `0`·`false`는 그대로 반환합니다. 회원 상태
문자열만으로 별도의 회원 여부를 추정하지 않습니다. `is_member`는 원천의
`notMember`가 명시됐을 때만 그 반대값으로 계산합니다. 음수·소수·문자열인
혜택 금액이나 이용 횟수는 유효한 관측으로 받아들이지 않습니다.

따라서 `free_return_count: 0`은 확인된 이용 0회이고, 필드 생략은 이용 횟수
미확인입니다. 세부 혜택을 더해 원천의 합계를 재구성하지 않으며, 합계가 있어도
빠진 항목을 0으로 채우지 않습니다. v5의 모든 세부 필드가 항상 존재한다고
가정한 클라이언트는 필드 존재 여부를 먼저 확인해야 합니다.

등록 결제수단은 카드번호 없이 유형·브랜드·발급사 기준으로 요약합니다.
같은 요약에 묶인 원천 행의 자동결제 여부가 모두 명시되고 일치할 때만
`recurring_registered`를 남깁니다. 값이 충돌하거나 하나라도 미확인이면 생략합니다.
이 요약의 개수는 실제 카드 개수가 아니며 주문 결제수단의 증거도 아닙니다.

- `coverage.current_membership_fee_observed`: 현재 회비의 관측 여부. 명시된
  `currentFee: 0`을 다른 요금 필드로 대체하지 않습니다.
- `coverage.benefit_usage_observed`: 혜택 **합계**의 관측 여부. 가입 일수만
  확인됐다고 합계도 확인된 것으로 보지 않습니다. 세부 혜택 항목 전체의
  수집 여부를 보증하는 표시는 아닙니다.
- `coverage.card_reward_summary_observed`, `card_reward_this_month_observed`,
  `card_reward_next_month_observed`: 예상 적립 총액·이번 달·다음 달의 독립된
  관측 여부. 원화 금액과 통화를 함께 확인합니다.

캐시 이력의 `coverage.cash_transaction_status`는 다음을 구분합니다.

| 값 | 의미 |
| --- | --- |
| `not_read` | 검증된 페이지가 없음. 적립 건수·합계와 `cash_transaction_has_more`를 생략 |
| `partial` | 연속된 페이지를 읽었으나 마지막 페이지에 다음 페이지가 있다고 표시됨 |
| `complete_returned_pages` | 1페이지부터 연속으로 읽고 원천의 종료 표시를 확인함. 원천이 보존하지 않는 과거 자료까지 확보했다는 뜻은 아님 |

페이지 번호·목록·다음 페이지 표시가 없거나, 페이지가 중복·누락되면
구조 오류를 반환합니다. 와우카드로 분류된 거래의 금액·통화·날짜가
검증되지 않아도 오류입니다. 잘못된 행을 빼고 완전한 합계라고 반환하지 않습니다.
월평균은 실제 적립이 관측된 월을 분모로 하며, 관측 월이 없으면 생략합니다.

캐시 거래의 와우카드 분류는 페이지 안에서 원천 문구를 검사한 파생값입니다.
분류된 행의 금액·통화·날짜만 파서로 넘기며, 원문 설명·거래 식별자·계정번호는
전달하지 않습니다. 금액 합계가 확인돼도 모든 세부 혜택 항목의 관측이 보장되지는
않습니다. 세부 항목은 각 필드의 존재 여부로 구분합니다.

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
