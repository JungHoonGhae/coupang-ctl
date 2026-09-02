# 주문 이력 기반 쇼핑 타입의 행동과학 근거와 한계

검토일: 2026-09-02 (Asia/Seoul)

## 결론

쿠팡 주문 이력만으로 방어 가능하게 만들 수 있는 것은 **성격검사**가 아니라
다음 네 가지 **관찰된 구매 행동의 요약**이다.

1. 구매 시점 군집성 (purchase-timing clumpiness)
2. 구매 시간대 집중도 (purchase-time daypart concentration)
3. 반복 선택률 (repeat-choice rate)
4. 주문 바구니 폭 (basket breadth)

네 축은 16개 놀이형 캐릭터를 만드는 재료로 쓸 수 있다. 다만 사용자에게
"행동심리학으로 성격을 분석했다"고 홍보해서는 안 된다. 주문 로그에는 계획성,
충동성, 자기통제, 진짜 충성도, 습관의 자동성, 크로노타입, 구매 의도가 없다.
따라서 제품의 정확한 약속은 **"내 주문 이력으로 만든 놀이형 구매 패턴"**이다.

과학 용어와 산식은 방법론 화면에 두고, 결과 화면에는 짧고 장난스러운 라벨을
쓴다. 각 결과에는 분석 기간, 표본 수, 분모, 제외 규칙, 데이터 출처, 규칙 버전을
함께 표시한다.

## 증거 경계

| 층위 | 이 제품에서 가능한 것 | 불가능하거나 추가 검증이 필요한 것 |
| --- | --- | --- |
| 관찰 (observed) | 주문 시각, 안정적인 상품 식별자, 주문별 상품 구성, 주문 상태처럼 원천 응답에서 의미가 확인된 필드 | 상품명으로 추정한 카테고리, 필드 이름만 보고 짐작한 주문 묶음, 누락된 가입일 |
| 계산 (derived) | 주문 사이 간격, 특정 시간대 주문 비율, 같은 상품의 후속 구매 비율, 주문당 고유 상품 수 | 계산값을 곧바로 심리적 성향으로 바꾸는 것 |
| 추론 (inferred) | 명시적으로 "놀이형 유형"이라고 밝힌 결정론적 캐릭터 | 계획형, 충동형, 야행성 크로노타입, 브랜드 충성, 습관, 절약 성향, 가족 구성 같은 잠재 특성 |

반복 구매와 충성 또는 습관은 특히 구분해야 한다. 실제 연구에서도 반복 구매는
호의적인 태도와 상황 단서에 의한 자동적 습관 등 서로 다른 원인으로 생길 수
있다. 거래 기록과 태도 자료를 함께 쓴 연구가 이 둘을 별도로 다룬다는 점은,
단순 반복률만으로 원인을 확정하면 안 된다는 좋은 경계다
([Liu-Thompkins & Tam, 2013](https://doi.org/10.1509/jm.11.0508)).

## 1차 문헌에서 얻을 수 있는 근거

### 구매 간격과 몰아치기

Zhang, Bradlow, Small은 거래가 관찰 기간에 고르게 퍼지지 않고 뭉쳐 있는 정도를
경계 공백까지 포함한 엔트로피 기반 `clumpiness`로 측정했다. 북미 소매사와 서로
다른 여섯 회사의 자료에서 clumpiness는 기존 recency, frequency, monetary value와
기업 마케팅 활동을 통제한 뒤에도 이탈, 구매 발생, 금액 예측에 추가 정보를 줬다
([Zhang, Bradlow & Small, 2015](https://doi.org/10.1287/mksc.2014.0873)).
별도의 구매 간격 모형 연구도 지수분포 대신 규칙성을 허용한 Erlang-k 모형이
시뮬레이션과 여섯 데이터셋에서 예측력을 높였다고 보고한다
([Reutterer, Platzer & Schroder, 2021](https://doi.org/10.1016/j.ijresmar.2020.09.002)).

Goh와 Barabasi는 사건 사이 간격의 평균 `mu`와 표준편차 `sigma`를 이용한
burstiness 지수 `B = (sigma - mu) / (sigma + mu)`를 제안했다. `B`는 규칙적인
사건열에서 -1에 가깝고, 평균과 표준편차가 같은 무기억 기준에서 0, 극단적인
몰아치기에서 1에 가까워진다. 이는 사건열의 모양을 설명하는 지표이지 사람의
성격 척도가 아니다
([Goh & Barabasi, 2008](https://doi.org/10.1209/0295-5075/81/48002)).

짧은 사건열에서는 이 지수가 표본 수의 영향을 크게 받는다. Kim과 Jo는 유한한
사건열을 위한 보정 지표를 제안했고, 적은 사건 수에서 원래 지수를 그대로 비교할
때 생기는 문제를 보였다
([Kim & Jo, 2016](https://doi.org/10.1103/PhysRevE.94.032311)).
구매 간격 자체도 단순한 한 분포로 충분히 설명되지 않을 수 있고 마케팅 변수와
관찰되지 않은 가구 차이의 영향을 받는다는 구매 패널 연구가 있다
([Jain & Vilcassim, 1991](https://doi.org/10.1287/mksc.10.1.1)).

따라서 `coupangctl`은 "계획적인 사람"을 판정하지 말고 **이 기간의 주문 간격이
고른지, 몰려 있는지**만 말해야 한다.

### 시간대

Gullo 등은 네 연구와 수백만 건의 구매 분석에서 시간대에 따른 다양성 선택의
차이를 보고했고, 생리적 각성과 일주기 선호가 그 관계에 관여함을 실험했다
([Gullo et al., 2019](https://doi.org/10.1093/jcr/ucy061)). 그러나 이 결과가 한
사람의 주문 시각만으로 크로노타입이나 각성 수준을 판정할 수 있다는 뜻은 아니다.
다른 초기 연구에서는 광고 반응의 시간대 차이는 있었지만 구매 의도에는 시간대
효과가 없었다
([Hornik, 1988](https://doi.org/10.1086/209139)).
고전적인 morningness-eveningness 연구도 크로노타입을 주문 시각이 아니라 별도
자기평가 설문과 체온 리듬으로 측정했다
([Horne & Ostberg, 1976](https://pubmed.ncbi.nlm.nih.gov/1027738/)).

따라서 이 축은 네 축 중 근거가 가장 약한 **상황적 행동 요약**으로 취급한다.
"야행성", "충동 구매"가 아니라 **밤 주문 비중**이라고 해야 한다.

### 반복 선택과 다양성

Kahn, Kalwani, Morrison은 5개 상품군의 16개 브랜드 패널 자료를 이용해
다양성 추구와 강화 행동을 구분하는 여러 검증 가능한 모형을 제안했다. 핵심은
시간 순서가 있는 선택 패널에서 전환과 반복을 모델링했다는 점이다
([Kahn, Kalwani & Morrison, 1986](https://doi.org/10.1177/002224378602300201)).

상품 전환이 곧 "진짜 다양성 추구"인 것도 아니다. van Trijp, Hoyer, Inman은
진짜 다양성 추구를 다른 전환 원인과 구분하고 상품군 수준에서 검토했다
([van Trijp, Hoyer & Inman, 1996](https://doi.org/10.1177/002224379603300303)).
즉, 상품군과 선택 집합을 무시한 전체 SKU 반복률에는 재고 보충 주기, 상품군
구성, 할인, 단종, 추천 노출 같은 설명이 섞인다.

따라서 원천에서 검증된 쿠팡 카테고리가 있을 때만 카테고리 안에서 분석한다.
그 전에는 **정확히 같은 상품 식별자의 반복 선택률**만 제공하고, 이를 "충성도"나
"다양성 추구 성향"이라고 부르지 않는다. 상품명 텍스트로 임의 카테고리를 만들지
않는다. Gullo 등의 실제 구매 분석도 소매사가 제공한 다단계 상품 taxonomy를
사용하고 분류 깊이에 대한 강건성을 확인했다. 이는 자체 키워드 분류를 쿠팡의
공식 카테고리처럼 취급하지 말아야 한다는 설계 근거다
([Gullo et al., 2019](https://doi.org/10.1093/jcr/ucy061)).

### 주문 바구니

다상품군 바구니 연구는 한 주문에 여러 상품군이 함께 나타나는 현상에 보완성,
구매 주기의 우연한 일치, 관찰되지 않은 가구 선호가 모두 관여할 수 있음을
명시한다
([Manchanda, Ansari & Gupta, 1999](https://doi.org/10.1287/mksc.18.2.95)).
쇼핑 여행 연구에서도 큰 정기 구매와 작은 보충 구매를 관찰적으로 구분하지만,
그 목적을 확정하려면 주문 크기 이외의 맥락이 필요하다
([Kahn & Schmittlein, 1989](https://doi.org/10.1007/BF00436149)).
대규모 소매 바구니에서 쇼핑 미션을 식별한 연구도 단순 품목 수가 아니라
카테고리 동시 구매와 구성 균형을 사용했다. 여기서 미션은 원천 필드가 아니라
검증이 필요한 분석 결과다
([Sarantopoulos et al., 2016](https://doi.org/10.1016/j.jbusres.2015.08.017)).

따라서 한 품목 주문은 **집중형 바구니**라고 묘사할 수 있지만 "계획 구매" 또는
"단일 미션"이었다고 단정할 수 없다. 여러 품목 주문도 의도적인 묶음 구매인지,
플랫폼이 합친 것인지, 단순 동시 구매인지 로그만으로는 알 수 없다.

## 권장 축

과학 용어는 내부 스키마와 방법론에, 놀이형 문구는 리캡 화면에 쓴다.

| 우선순위 | 내부 축과 영문 | 관찰 가능한 양극 | 추천 놀이형 라벨 예시 | 금지할 심리 표현 |
| --- | --- | --- | --- | --- |
| 1 | 구매 시점 군집성 / purchase-timing clumpiness | 관찰 기간에 비교적 고르게 퍼짐 / 특정 기간에 주문이 뭉침 | `차곡차곡` / `몰아서팡` | 계획형 / 충동형 |
| 2 | 반복 선택률 / repeat-choice rate | 처음 산 상품 비중이 큼 / 이전에 산 정확한 상품 비중이 큼 | `처음봄` / `또삼` | 탐험가 성격 / 충성 고객 / 습관 중독 |
| 3 | 주문 바구니 폭 / basket breadth | 한 주문 한 상품이 많음 / 여러 상품 주문이 많음 | `하나만요` / `같이 와요` | 목적 구매 / 쟁임 본능 |
| 4, 선택적 | 구매 시간대 집중도 / purchase-time daypart concentration | 주간 주문이 많음 / 야간 주문이 많음 | `해 떠 있을 때` / `다 잘 때` | 아침형 인간 / 야행성 / 자기통제 부족 |

첫 세 축은 로그가 직접 담는 구조와 비교적 가깝다. 시간대 축은 재미와 식별력이
있지만 상황 의존성이 커서 방법론상 가장 낮은 신뢰 등급을 준다. 네 글자 16유형을
유지해야 할 때만 네 번째 축으로 넣고, UI에는 "성격"이 아니라 "주문 시간대"라고
설명한다.

네 축이 심리측정학적으로 서로 독립적이라고 주장해서도 안 된다. 시간대와 다양성
선택은 같은 연구에서 연관됐고, 구매 빈도와 바구니 규모도 큰 장보기와 보충 구매
구조에서 함께 움직일 수 있다. 16유형은 MBTI처럼 검증된 직교 척도가 아니라,
서로 관련될 수 있는 네 관찰 지표를 이해하기 쉽게 조합한 캐릭터 체계다.

## 산식과 분모

### A. 구매 시점 군집성

1. 취소된 주문과 의미가 확인되지 않은 레코드를 제외한다.
2. 같은 결제 또는 주문 묶음임이 검증된 행은 하나의 구매 사건으로 묶는다.
3. 기본 리듬 사건은 현지 시간의 **고유 주문일**로 정의한다. 같은 날 여러 주문은
   별도 재미 통계로 남기되 리듬 축에서는 하루 한 번으로 센다.
4. 완전성이 확인된 동일한 선택 기간을 이산 일 단위 `1..T`로 두고 `n`개 주문일을
   정렬한다. 최초 데이터가 계정 가입일이라는 증거가 없으면 가입 이후 전체로
   부르지 않는다.
5. 첫 주문일 전과 마지막 주문일 후를 포함한 `n + 1`개 공백을 구하고 관찰 구간으로
   나눈 비율을 `p_i`라 한다.
6. 정규화 clumpiness를 계산한다.

```text
C = 1 + sum(p_i * log(p_i)) / log(n + 1)
```

`0 * log(0)`은 0으로 처리한다. 공백이 같으면 `C = 0`에 가깝고 주문이 한 기간에
몰리면 1에 가까워진다. 시작과 끝 공백을 포함하므로 최초·마지막 주문만 잘라낸
간격 CV보다 관찰 창의 구조를 더 잘 보존한다. 비교할 때는 모든 대상에 같은 기간
경계를 써야 한다.

보조 진단으로 `cv = sd(delta) / mean(delta)`와
`B = (cv - 1) / (cv + 1)`도 반환할 수 있다.

`B = 0`은 다른 사람들의 평균이 아니라 **표준편차와 평균이 같은 무기억 사건열
기준**이다. `B > 0`을 "남들보다 몰아 산다"고 쓰면 안 된다. 현재의 "활성 월
최장 연속 / 전체 월 범위"는 캘린더 연속성을 재는 별도 지표이며 구매 간격
burstiness와 같지 않다.

`C`에는 사람을 가르는 자연스러운 보편 문턱값이 없다. 권장 분류는 같은 관찰
기간과 같은 주문 수를 가진 균등 무작위 사건열을 합성해 `C`의 귀무 분포를 만들고,
실제 `C`의 위치를 보여주는 방식이다. 예를 들어 상·하위 25% 밴드를 `몰림`과
`고름`으로 쓰더라도 이것은 **사람 집단의 분위수**가 아니라 동일 조건의 균등
사건열에 대한 위치임을 밝혀야 한다. 중간은 `혼합`으로 둔다. 25% 밴드 자체도
제품 기본값이므로 규칙 버전에 포함한다. leave-one-out에서 판정이 자주 바뀌면
`판정 보류`로 표시한다. 유한 표본 보정은 경계 조건까지 구현해 합성 사건열로
검증한 뒤 도입한다.

### B. 구매 시간대 집중도

미리 문서화한 현지 시간 구간을 사용한다. 기존 제품 구간을 유지한다면:

```text
night_share = orders_at_20_00_to_05_59 / timestamped_retained_orders
```

`20:00-05:59`는 행동과학이 정한 보편적 야간 경계가 아니라 제품 규칙이다. 반드시
시간대와 함께 버전을 고정한다. 50%는 "주문의 과반이 이 구간"이라는 문자 그대로의
경계여서 설명 가능하다. 현재 40% 경계는 비교 집단이나 외부 규준이 없는 한
"야간형"을 뜻하는 근거가 없다. 구간별 막대 또는 24시간 방사형 차트를 함께 보여
이분법의 정보 손실을 보완한다.

### C. 반복 선택률

상품 수량이 아니라 **주문별 고유 상품 선택**을 센다. 한 주문에서 같은 상품을
3개 산 것을 재구매 2회로 세지 않는다.

```text
N = stable_product_id가 있는 (order, product) 고유 쌍 수
repeat_events = 이전 retained order에 같은 product_id가 있었던 쌍 수
repeat_choice_rate = repeat_events / N
```

식별자가 완전하면 `repeat_choice_rate = 1 - distinct_products / N`과 같다. 전체
SKU 지표는 "반복 선택"으로만 부른다. 검증된 원천 카테고리가 생기면 카테고리별
반복률과 분모를 먼저 보여주고, 전체값은 유효 선택 수로 가중한다. 카테고리별
구매 빈도와 보충 주기가 다르므로 카테고리 구성이 바뀌면 전체 점수도 바뀔 수
있음을 명시한다.

### D. 주문 바구니 폭

```text
k_j = retained order j의 고유 stable_product_id 수
single_order_share = count(k_j == 1) / valid_orders
multi_order_share = 1 - single_order_share
```

중앙 주문당 상품 수와 분포도 함께 보인다. 현재처럼 **단일 품목 주문이 차지한
금액 비중**을 쓰면 비싼 단품 하나가 축 전체를 뒤집을 수 있어 바구니 구조와 가격을
혼합한다. 타입 축에는 주문 수 기준을 쓰고 금액 구성은 별도 통계로 둔다.

## 문턱값과 최소 표본 전략

아래 수치는 논문에서 검증된 보편 규준이 아니라, 과도한 판정을 막기 위한
**초기 제품 기본값**이다. 합성 데이터와 실제로 허용된 익명 집계에서 안정성을
검증하고 규칙 버전과 함께 변경해야 한다.

| 축 | 권장 최소 데이터 | 설명 가능한 중립 경계 | 표본 부족 처리 |
| --- | --- | --- | --- |
| 구매 간격 | 고유 retained order day 20일, 관찰기간 180일 | 같은 `n`과 기간의 균등 사건열 분포. 상·하위 밴드는 제품 규칙으로 버전 관리 | 중앙 간격과 범위만 표시, 타입 축 제외 |
| 시간대 | 유효 시각 주문 20건, 3개월 이상 | `night_share = 0.5`는 야간 구간 과반 | 시간대 히스토그램만 표시, 타입 축 낮은 신뢰 표시 |
| 반복 선택 | 유효 상품 선택 20건, retained order 10건, 180일, ID coverage 70% 이상 | `repeat_choice_rate = 0.5`는 선택의 과반이 반복 | 수치와 분모만 표시하거나 축 제외 |
| 바구니 폭 | 상품 구성이 유효한 retained order 10건 | `single_order_share = 0.5`는 주문의 과반이 단품 | 중앙 상품 수만 표시하거나 축 제외 |

비율 축에는 Wilson 구간처럼 작은 표본에서 정규 근사보다 적합한 구간을 함께
계산할 수 있다
([Wilson, 1927](https://doi.org/10.1080/01621459.1927.10502953)). 구간이 0.5를
가로지르면 엄밀한 결과는 `균형`이다. 16유형을 위해 반드시 한쪽을 골라야 한다면
점추정치로 캐릭터는 배정하되 `놀이용 분류`, 표본 수, 축 점수를 보여주고 강한
해석을 금지한다.

### 재구매 30%는 방어 가능한가

- **가능:** "식별 가능한 상품 선택 120건 중 36건이 이전 주문과 같은 상품이었다"
  같은 개인 내 기술 통계.
- **불가능:** "30% 이상이므로 재구매를 엄청 많이 하는 편", "상위 재구매형",
  "충성도가 높다" 같은 상대적 또는 심리적 주장.
- **이유:** 현재 대표성 있는 `coupangctl` 사용자 분포, 상품군별 규준, 관찰기간이
  같은 비교 집단이 없다. 첫 구매가 반드시 포함되는 산식 특성상 기간과 표본 수도
  비율을 바꾼다.
- **권장:** 30%는 축의 과학적 문턱값으로 사용하지 않는다. 50%는 적어도 "선택의
  과반이 반복"이라는 의미론적 경계가 있다. 더 나은 방법은 연속 점수를 그대로
  보여주고, 향후 동의받은 대표 표본이 생겼을 때만 사전 등록한 분위수 기준으로
  "이 비교 집단 안에서"라는 문구와 함께 상대 등급을 제공하는 것이다.

## 사용자 화면의 표현 원칙

학술 용어를 메인 카드에 올리는 대신 다음 두 층으로 나눈다.

### 메인 카드

```text
몰아서팡 · 다 잘 때 · 또삼 · 하나만요

내 주문 기록에서는
조용하다가 한꺼번에 사고,
밤에 주문하고,
전에 산 상품을 다시 고르고,
한 주문엔 한 상품을 담는 날이 많았어요.
```

이 문구는 관찰된 행동을 일상어로 옮기지만 원인을 주장하지 않는다. 캐릭터 이름은
재미있게 만들 수 있어도 "충동형", "중독형", "절약형", "충성형"은 피한다.

### 근거 영수증

```text
분석: 2021-03 ~ 2026-08
자료: 내 기기에 저장된 쿠팡 주문 이력
표본: retained order 000건, 식별 가능한 상품 선택 000건
제외: 전액 취소, 시각 누락, 상품 식별자 누락
계산: 밤 주문 00% · 반복 선택 00% · 단품 주문 00%
규칙: shopping-type/v4
안내: 성격검사가 아닌 주문 기록 기반 놀이형 분류
```

"쿠팡 API 분석" 또는 "쿠팡 공식 유형"이라고 쓰지 않는다. 사실에 맞는 출처는
"브라우저 세션으로 동기화해 로컬에 정규화한 내 쿠팡 주문 이력"이다. 공개용
결과에서는 분석 기간을 월 단위로 낮추고 정확한 상품명, 주문번호, 금액, 날짜는
기본적으로 숨긴다.

## 시각화 권장

- 구매 간격: 시간순 점 또는 막대와 중앙 간격. 평균만 쓰지 않는다.
- 시간대: 24시간 히스토그램 또는 원형 시간 차트. 야간 영역과 시간대를 명시한다.
- 반복 선택: `처음 산 선택`과 `이전에 산 선택`의 100% 누적 막대. 카테고리가
  검증되면 카테고리별 소형 막대를 추가한다.
- 바구니 폭: 주문당 고유 상품 수의 `1`, `2`, `3`, `4+` 분포 막대. 단순 파이
  차트보다 분포를 더 잘 보존한다.
- 네 축 요약: 레이더 차트는 서로 다른 단위와 임의 정규화를 숨길 수 있으므로
  장식용으로만 쓰지 않는다. 네 개의 동일 길이 양극 막대가 산식과 분모를 더
  정직하게 보여준다.

모든 차트에는 `n`, 기간, 누락 상태를 붙인다. 표본이 부족한 축은 0으로 그리지
말고 `데이터 부족` 상태로 남긴다.

## 구현 전 완료 조건

각 축은 다음이 모두 맞을 때만 제품 지표로 완료된다.

1. typed core 응답에 원시 점수, 분자, 분모, 기간, timezone, eligibility,
   provenance, rule version이 있다.
2. 취소, 반품, 누락 시각, 상품 ID 누락, 동일 주문 중복의 처리 규칙이 문서와
   합성 테스트에 동일하게 반영된다.
3. 카테고리는 원천 구조에서 의미가 확인되기 전에는 타입 계산에 사용하지 않는다.
4. CLI와 MCP는 같은 typed core 결과를 표현만 다르게 렌더링한다.
5. 리캡 문구가 계산값보다 강한 심리 또는 인구집단 주장을 하지 않는다.
6. 공개 카드에 기간, 표본, 산식 요약, "놀이형 분류" 안내가 있다.

## 참고한 1차 문헌

- Zhang, Y., Bradlow, E. T., & Small, D. S. (2015). Predicting customer
  value using clumpiness: From RFM to RFMC. *Marketing Science, 34*(2),
  195-208. <https://doi.org/10.1287/mksc.2014.0873>
- Reutterer, T., Platzer, M., & Schroder, N. (2021). Leveraging purchase
  regularity for predicting customer behavior the easy way. *International
  Journal of Research in Marketing, 38*(1), 194-215.
  <https://doi.org/10.1016/j.ijresmar.2020.09.002>
- Goh, K.-I., & Barabasi, A.-L. (2008). Burstiness and memory in complex
  systems. *EPL, 81*, 48002. <https://doi.org/10.1209/0295-5075/81/48002>
- Kim, E.-K., & Jo, H.-H. (2016). Measuring burstiness for finite event
  sequences. *Physical Review E, 94*, 032311.
  <https://doi.org/10.1103/PhysRevE.94.032311>
- Jain, D. C., & Vilcassim, N. J. (1991). Investigating household purchase
  timing decisions: A conditional hazard function approach. *Marketing
  Science, 10*(1), 1-23. <https://doi.org/10.1287/mksc.10.1.1>
- Gullo, K., Berger, J., Etkin, J., & Bollinger, B. (2019). Does time of day
  affect variety-seeking? *Journal of Consumer Research, 46*(1), 20-35.
  <https://doi.org/10.1093/jcr/ucy061>
- Hornik, J. (1988). Diurnal variation in consumer response. *Journal of
  Consumer Research, 14*(4), 588-591. <https://doi.org/10.1086/209139>
- Horne, J. A., & Ostberg, O. (1976). A self-assessment questionnaire to
  determine morningness-eveningness in human circadian rhythms.
  *International Journal of Chronobiology, 4*(2), 97-110.
  <https://pubmed.ncbi.nlm.nih.gov/1027738/>
- Kahn, B. E., Kalwani, M. U., & Morrison, D. G. (1986). Measuring
  variety-seeking and reinforcement behaviors using panel data. *Journal of
  Marketing Research, 23*(2), 89-100.
  <https://doi.org/10.1177/002224378602300201>
- van Trijp, H. C. M., Hoyer, W. D., & Inman, J. J. (1996). Why switch?
  Product category-level explanations for true variety-seeking behavior.
  *Journal of Marketing Research, 33*(3), 281-292.
  <https://doi.org/10.1177/002224379603300303>
- Liu-Thompkins, Y., & Tam, L. (2013). Not all repeat customers are the same:
  Designing effective cross-selling promotion on the basis of attitudinal
  loyalty and habit. *Journal of Marketing, 77*(5), 21-36.
  <https://doi.org/10.1509/jm.11.0508>
- Manchanda, P., Ansari, A., & Gupta, S. (1999). The shopping basket: A model
  for multicategory purchase incidence decisions. *Marketing Science, 18*(2),
  95-114. <https://doi.org/10.1287/mksc.18.2.95>
- Kahn, B. E., & Schmittlein, D. C. (1989). Shopping trip behavior: An
  empirical investigation. *Marketing Letters, 1*(1), 55-69.
  <https://doi.org/10.1007/BF00436149>
- Sarantopoulos, P., Theotokis, A., Pramatari, K., & Doukidis, G. (2016).
  Shopping missions: An analytical method for the identification of shopper
  need states. *Journal of Business Research, 69*(3), 1043-1052.
  <https://doi.org/10.1016/j.jbusres.2015.08.017>
- Wilson, E. B. (1927). Probable inference, the law of succession, and
  statistical inference. *Journal of the American Statistical Association,
  22*(158), 209-212. <https://doi.org/10.1080/01621459.1927.10502953>
