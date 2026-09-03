# 리캡 출력 계약

쇼핑 리캡은 동일한 typed insight를 사용하지만 출력 목적에 따라 개인정보
경계가 다릅니다.

## HTML

```bash
coupangctl orders recap --output ./shopping-recap.html
coupangctl orders recap --output ./private-recap.html --include-products
```

기본 HTML은 `public_safe`이며 상품명과 정확한 날짜를 제외합니다.
`--include-products`를 명시한 HTML은 `private_products`이고 실제 상품명·날짜·
금액을 포함하므로 공유하면 안 됩니다. 두 방식 모두 새 `0600` 파일만 쓰고
기존 파일을 덮어쓰지 않습니다.

## PNG 공유 카드

PNG는 반드시 미리보기와 확인의 두 단계로 만듭니다.

```bash
# 1. 파일을 만들지 않고 실제 공유 값 확인
coupangctl orders recap-image

# 2. 같은 public-safe 규칙을 확인한 뒤 새 PNG 생성
coupangctl orders recap-image \
  --output ./shopping-recap.png \
  --confirm-public-safe-image
```

첫 응답은 `RecapSharePreview` schema version 1입니다.

- `visibility`: 항상 `public_safe`
- `format`, `width`, `height`: `png`, `1080`, `1350`
- `ready`: 네 축이 모두 최소 표본을 만족했는지 여부
- `fields`: 이미지에 들어갈 실제 값, provenance, 해당 표본 수와 규칙
- `excluded`: 이미지에 들어가지 않는 필드 그룹
- `confirmation_flag`: 쓰기 단계에 필요한 정확한 확인 flag
- `limitations`: 성격 진단이나 모집단 비교가 아니라는 해석 경계

현재 ready 이미지의 개인화 필드는 다음뿐입니다.

- 월 단위 분석 기간
- 파생된 쇼핑 타입 코드와 장난스러운 캐릭터 이름
- 네 축의 선택값과 화면에 표시되는 분자·분모·표본 설명
- 분석 주문 수, 최장 연속 구매 개월, 24시간 배송률과 배송 표본 수

상품명, 브랜드명, 카테고리명과 수량, 정확한 주문일, 금액, 주문 ID,
결제수단, 계정 식별자, 인증·세션 정보는 제외됩니다. PNG에는 이를 다시
넣는 private 옵션이 없습니다.

확인 단계의 `RecapImageWriteResult`는 `written`, `format`, `visibility`,
`width`, `height`, `bytes`와 동일한 `preview`를 반환합니다. 로컬 Chrome은
임시 `file:` HTML만 headless로 렌더링하며 쿠팡 네트워크를 호출하지 않습니다.
완성 PNG의 크기를 다시 검증한 뒤 새 `0600` 파일로 복사하고, 기존 경로는
거부하며 임시 HTML·브라우저 프로필·중간 PNG를 제거합니다.
