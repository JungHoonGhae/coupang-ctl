<h1 align="center">coupangctl</h1>

<p align="center">
  쿠팡 상품 탐색과 내 구매 기록 분석을 AI에게.<br>
  검색부터 비교 근거까지, 내 컴퓨터에서 이어가는 쇼핑 도우미.
</p>

<p align="center">
  <a href="https://github.com/JungHoonGhae/coupang-ctl/actions/workflows/ci.yml"><img src="https://github.com/JungHoonGhae/coupang-ctl/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/CLI_%C2%B7_MCP-local_first-252724?style=flat-square" alt="CLI · MCP · local first">
  <img src="https://img.shields.io/badge/status-early_access-eb6c36?style=flat-square" alt="Early access">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-252724?style=flat-square" alt="MIT license"></a>
</p>

<p align="center">
  <a href="#빠른-시작">빠른 시작</a> ·
  <a href="#추천에-이르는-과정">추천 과정</a> ·
  <a href="#내-주문-확인과-분석">주문 분석</a> ·
  <a href="#전체-구조">전체 구조</a> ·
  <a href="docs/advanced-usage.md">상세 사용 가이드</a>
</p>

> [!IMPORTANT]
> 쿠팡의 공식 제품이 아닌 초기 개발 프로젝트입니다. 최종 주문·결제는 자동화하지 않습니다.
> 아래 설명은 main 개발 소스 기준이며, Camofox 전환을 담은 새 태그 릴리스는 아직 없습니다.

---

## 왜 필요한가요?

“200만 원 안에서 데스크탑을 고르고 싶다”는 요청에는 검색 이상의 일이 필요합니다.
카테고리마다 다른 필터를 확인하고, 판매 옵션의 가격과 사양을 비교하고, 빠진 정보도 찾아야 하죠.
내 소비를 돌아볼 때도 화면에 보이는 몇 건만으로 전체 구매 패턴을 말할 수는 없습니다.

`coupangctl`은 이 과정을 CLI와 MCP 도구로 연결합니다.
상품은 현재 소스에서 읽고, 주문은 사용자가 동기화한 범위 안에서 로컬로 분석합니다.
확인한 사실, 계산한 값, 추론을 구분해 AI가 판단의 근거를 설명하도록 돕습니다.

<a id="한눈에-보기"></a>

## 이런 일을 맡길 수 있어요

| AI에게 하는 요청 예시 | 도구가 제공하는 근거 |
| --- | --- |
| “200만 원 이하 조립 PC를 찾아줘. 필터부터 확인하고 옵션별로 비교해줘.” | 검색 필터, 선택 상태, 판매 옵션의 관찰가·사양, 조건 충족 여부 |
| “저장된 주문에서 자주 산 품목과 지출을 보여줘. 빠진 기간도 알려줘.” | 로컬 주문 집계, 취소·반품 구분, 마지막 수집 상태와 누락 범위 |
| “WOW 혜택이 회비보다 컸는지 보고 싶어.” | 화면에서 확인한 혜택·기간과 회비 근거. 실제 비용과 추정 비용을 구분 |

위 문장은 사용 예시이며 실제 조회 결과가 아닙니다.
[MCP를 연결한 AI](#mcp-연결)가 요청을 조건으로 바꿉니다. CLI 자체가 자연어를 해석하는 것은 아닙니다.
구매 이력을 추천에 쓰려면 `--use-purchase-history`로 따로 요청해야 합니다.

<a id="자연어로-상품-찾기"></a>
<a id="추천-조사-실험적"></a>

## 추천에 이르는 과정

추천의 출발점은 “몇 개를 보여줄까?”가 아니라 “어떤 조건을 확인해야 할까?”입니다.

<p align="center">
  <img src="docs/diagrams/recommendation.png" width="720" alt="요청 조건을 정리하고 검색에서 필터를 발견합니다. 선택 상태를 검증한 뒤 정확한 판매 옵션을 조사합니다. 조건 충족 후보, 제외 이유, 미확인 근거를 보고서에 함께 남깁니다.">
</p>

1. **필터를 발견합니다.** 기본 검색의 `coverage.facets`에서 현재 카테고리의 선택지를 읽습니다.
2. **범위를 좁힙니다.** 실제 제공된 필터만 적용하고 최종 선택 상태를 확인합니다.
3. **판매 옵션을 조사합니다.** 같은 상품이라도 옵션별 가격·사양을 구분합니다. 검색 제목을 확정 사양으로 쓰지 않습니다.
4. **근거를 남깁니다.** 조건부 후보뿐 아니라 제외 이유, 미확인 정보, 다음 확인 항목도 보고서에 담습니다.

`--limit`은 표시 개수일 뿐, 조사 깊이나 적절한 추천 개수의 근거가 아닙니다.
`complete`도 정해진 범위의 조사 완료를 뜻하며 “시장 전체에서 최고”라는 판정은 아닙니다.
이 그림은 기능 흐름이지 특정 상품을 실제 조사한 기록은 아닙니다.

<details>
<summary>CLI로 조사하고 HTML 보고서 만들기</summary>

```bash
./coupangctl products recommend \
  --query '조립PC' --max-price 2000000 --proceed --no-affiliate
```

`--proceed`를 생략하면 관찰된 필터를 바탕으로 선택 질문 후보를 반환합니다.
조사 결과는 `needs_input`, `no_matches`, `incomplete`, `complete`로 구분합니다.
가격이나 필수 조건의 근거가 없으면 조건을 충족했다고 처리하지 않습니다.

기존 결과를 `recommendation.json`으로 저장했다면 다음처럼 보고서를 만듭니다.
아래 변환에는 별도 도구인 `jq`가 필요합니다.

```bash
jq '{schema_version: 2, title: "상품 비교 근거", recommendation: .}' recommendation.json |
  ./coupangctl products report --input - --output comparison.html
```

보고서 생성은 상품을 다시 조회하지 않습니다. 기존 출력 파일도 덮어쓰지 않습니다.
결과는 개인용 `private_local`이며, HTML을 열면 외부 상품 이미지가 로드될 수 있습니다.
[조건 입력·보고서 응답 계약](docs/advanced-usage.md#추천-조사-실험적)

</details>

<a id="개발-소스-빠른-시작"></a>

## 빠른 시작

현재는 소스에서 빌드해 사용합니다. 빌드에는 Go 1.26 이상이 필요합니다.
사이트 조회에는 Node 22 이상과 **검증해 설치한 Camofox 서버·Camoufox 엔진**이 필요합니다.
`camofox setup`은 설치된 런타임을 등록하며 다운로드나 업데이트를 대신하지 않습니다.

### 1. CLI 빌드

```bash
git clone https://github.com/JungHoonGhae/coupang-ctl.git
cd coupang-ctl
go build -o ./coupangctl ./cmd/coupangctl
```

이 안내는 macOS에서 실측한 경로를 기준으로 합니다.
Windows·Linux의 빌드 및 합성 테스트 통과는 실제 브라우저 동작 검증과 다릅니다.
다운로드한 릴리스를 사용할 때는 [산출물 검증 절차](RELEASING.md)를 따르세요.

<a id="camofox-기본-연결-개발-소스"></a>

### 2. 전용 브라우저 등록

아래 경로를 설치된 Camofox 서버 폴더와 Camoufox 엔진 폴더로 바꿔 실행하세요.

```bash
./coupangctl camofox setup \
  --runtime /absolute/path/to/camofox \
  --engine-dir /absolute/path/to/camoufox
./coupangctl doctor
```

조회는 창 없이 전용 세션에서 실행합니다. 일상용 Chrome·Aside와 프로필을 공유하지 않으며,
확장 프로그램·Swift·Orca는 필요하지 않습니다. 설정이나 접근에 문제가 생겨도 다른 브라우저로 자동 전환하지 않습니다.
`doctor`는 로컬 설치 점검입니다. 로그인 확인은 다음 단계에서 합니다.
[런타임·저장소·재연결 상세](docs/advanced-usage.md#camofox-기본-연결-개발-소스)

<a id="로그인-방식"></a>

### 3. 로그인하고 첫 검색

직접 인증할 때만 전용 창을 엽니다. 이미 인증된 세션이면 새 로그인 창을 열지 않습니다.

```bash
./coupangctl login --manual
./coupangctl auth status
./coupangctl products search --query '미니 식기' --limit 3 --no-affiliate
```

휴대폰 앱에서 승인하려면 `login --qr --link`를 선택하세요.
일회성 링크와 확인 숫자는 공유하거나 저장하지 마세요.
앱에서 링크가 열리지 않으면 `--manual` 화면에서 직접 로그인할 수 있습니다.
SMS 발송·OTP 자동 입력은 지원하지 않습니다.

`auth status`의 성공은 사이트의 로그인 상태 확인입니다.
현재 계정과 로컬 DB의 연결, 주문 조회, 전체 이력 확보까지 보증하지는 않습니다.
[로그인 방식과 제한](docs/advanced-usage.md#로그인-방식)

<a id="mcp-연결"></a>

### 4. AI에 연결

사용하는 AI 앱의 MCP 설정에 다음 내용을 넣으세요. `command`는 빌드한 바이너리의 절대경로로 바꿉니다.

```json
{
  "mcpServers": {
    "coupangctl": {
      "command": "/absolute/path/to/coupangctl",
      "args": ["mcp"]
    }
  }
}
```

연결한 뒤 “미니 식기를 찾아줘. 사용 가능한 필터부터 확인해줘”처럼 요청하면 됩니다.
MCP 연결과 도구 목록 확인은 브라우저나 주문 DB 없이 가능합니다. 사이트 조회 시에는 위의 런타임 설정이 필요합니다.
[도구 목록·연결 계약](docs/advanced-usage.md#mcp-연결)

<a id="무엇을-할-수-있나요"></a>
<a id="주문-분석과-리캡"></a>

## 내 주문 확인과 분석

지금 주문만 볼 때는 `preview`, 이력을 모아 분석할 때는 `sync`를 사용합니다.

<p align="center">
  <img src="docs/diagrams/orders.png" width="720" alt="orders preview는 주문 첫 페이지만 개인용으로 반환하고 주문 DB를 사용하지 않습니다. orders sync는 정규화한 주문과 커서를 SQLite에 저장합니다. 저장한 범위에서 개인 분석과 공유용 리캡을 만듭니다.">
</p>

```bash
./coupangctl orders preview
```

`preview`는 첫 페이지를 읽고 주문 DB를 열거나 저장하지 않습니다.
다음 페이지 유무는 `has_next_page`로 확인하며 결과는 개인용 `private_local`입니다.
빈 결과를 “구매한 적 없음”으로, 첫 페이지를 전체 이력으로 해석하지 않습니다.

> [!WARNING]
> 현재 로그인 계정과 기존 주문 DB를 자동으로 연결해 검증하지 않습니다.
> 계정을 바꾼 뒤 같은 DB에 동기화하면 이력이 섞일 수 있으므로, 계정 연결을 확인하기 전에는 `preview`만 사용하세요.

<details>
<summary>같은 계정의 주문을 로컬에 모아 분석하기</summary>

```bash
./coupangctl orders sync --max-pages 1
./coupangctl orders sync-status
./coupangctl orders stats
```

첫 명령은 한 페이지만 저장합니다. 수집을 이어가려면 `orders sync`를 다시 실행하세요.
`sync-status`와 통계는 로컬 DB만 읽으며 브라우저를 시작하지 않습니다.
마지막 실행과 누적 수집 범위를 구분하고, 커서가 끝나도 전체 계정 이력 확보로 단정하지 않습니다.

분석·리캡은 저장된 자료의 범위와 누락을 함께 표시합니다.
쇼핑 유형은 규칙 기반 해석이며 성격이나 심리 진단이 아닙니다.
공유용 `public_safe` 리캡과 실제 상품·금액·날짜가 있는 개인용 보고서를 구분하세요.

[주문 확인·수집 계약](docs/advanced-usage.md#무엇을-할-수-있나요) ·
[분석·리캡 명령](docs/advanced-usage.md#주문-분석과-리캡) ·
[유형의 근거](TYPE_SYSTEM.md)

</details>

## 어디까지 확인됐나요?

| 영역 | 현재 범위 |
| --- | --- |
| 검색·필터·상세 | 전용 Camofox 조회와 정확한 판매 옵션 연결. 확인하지 못한 값은 누락으로 표시 |
| 로그인·주문 | 세션 확인, 첫 페이지 미리보기, 로컬 동기화·분석. 현재 계정과 DB의 연결은 미검증 |
| 추천·보고서 | 실험적 조사와 근거 HTML. 조사 중단·필수 조건 미확인·제외 이유를 보존 |
| WOW·가격 이력 | 관찰된 혜택과 옵션별 가격 기록. 혜택 추정과 실제 금액을 구분 |
| 미지원 | 장바구니 변경, 영수증 조회, SMS·OTP 자동 입력, 리뷰 게시 |
| 자동화하지 않는 작업 | 최종 주문 확정과 결제 |

2026-09-11 실측에서는 서로 다른 두 검색, 상품 상세, 로그인 상태, 주문 미리보기를 확인했습니다.
제한된 추천 조사는 `review_budget_reached`로 `incomplete`를 반환했습니다.
이 기록을 전체 기능이나 모든 계정의 성공으로 확대하지 않습니다.
[실측 결과와 남은 검증](intent/shopping-ai-verification.md)

<a id="구조"></a>

## 전체 구조

<p align="center">
  <img src="docs/diagrams/architecture.png" width="720" alt="CLI와 MCP adapter가 공통 서비스와 typed core를 호출합니다. 쿠팡 소스 읽기는 Camofox adapter, 로컬 주문·가격 저장은 SQLite repository, 보고서 생성은 별도 렌더러가 맡습니다.">
</p>

CLI와 MCP는 같은 typed core와 서비스를 사용합니다.
불안정한 사이트 응답은 좁은 소스 adapter에 격리하고, 로컬 분석·보고서 생성은 브라우저와 분리합니다.
[구현 위치](docs/advanced-usage.md#구조) · [Pretendard 다이어그램 원본](docs/diagrams/README.md)

<a id="데이터-저장과-개인정보"></a>

## 내 데이터는 어떻게 다루나요?

- 쿠키·세션은 전용 로컬 상태 디렉터리에 보관하며 로그나 도구 응답으로 내보내지 않습니다.
- 원시 주문 응답은 저장하지 않습니다. `sync`로 정규화한 주문 DB와 개인 보고서는 내 컴퓨터에 남깁니다.
- 일반 CLI 검색은 DB 없이 실행합니다. 상세 조회와 MCP의 가격 기록은 확인된 옵션·가격 근거가 있을 때 시도합니다.
- 원본 쿠팡 URL을 보존합니다. 제휴 링크는 `--no-affiliate` 또는 `COUPANGCTL_AFFILIATE_DISABLED=true`로 끌 수 있습니다.

POSIX에서는 비공개 파일에 `0600` 권한을 적용합니다. Windows는 상속된 ACL을 사용하며 소유자 전용 접근을 보장하지 않습니다.
AI 앱에 전달한 결과는 그 앱의 데이터 처리 정책도 확인하세요.
[저장 경로·공유 범위](docs/advanced-usage.md#데이터-저장과-개인정보) · [개인정보 안내](PRIVACY.md)

<a id="climcp-오류-계약"></a>

설정 필요·인증 필요·접근 거부·불완전한 응답은 구분해 반환합니다.
실패를 성공으로 바꾸거나 임의로 로그인 창을 열지 않습니다.
[CLI·MCP 오류 계약](docs/advanced-usage.md#climcp-오류-계약)

## 더 알아보기

- [상세 사용 가이드](docs/advanced-usage.md) — 명령, JSON 응답, 검색 필터, 추천, 주문 분석
- [제품 의도](INTENT.md) · [제품 원칙](PRODUCT_PRINCIPLES.md) — 만들려는 경험과 사실·계산·추론의 경계
- [가격 이력](PRICES.md) · [WOW 혜택](ACCOUNT.md) · [쇼핑 유형](TYPE_SYSTEM.md) — 지표의 의미와 제한
- [검증 기록](intent/shopping-ai-verification.md) · [GitHub Issues](https://github.com/JungHoonGhae/coupang-ctl/issues) — 확인한 동작과 남은 작업
- [릴리스 검증](RELEASING.md) · [보안 제보](SECURITY.md)

<a id="개발"></a>
<a id="기여"></a>

<details>
<summary>개발과 기여</summary>

```bash
go test ./...
go vet ./...
npm ci
npm run typecheck
npm run test:camofox
go build ./cmd/coupangctl
```

새 지표에는 출처, 분모, 표본 수, 누락 처리와 합성 테스트가 필요합니다.
자세한 기준은 [제품 원칙](PRODUCT_PRINCIPLES.md)을 따릅니다.
버그 제보와 PR에는 실제 주문·쿠키·인증 정보 대신 합성 자료나 가린 메타데이터를 사용해 주세요.

</details>

<a id="쿠팡-파트너스-고지"></a>
<a id="파트너스-링크-비활성화"></a>

## 쿠팡 파트너스 고지

[쿠팡 홈 열기](https://link.coupang.com/a/gIEGRL0z7c)

이 링크를 통해 구매하면 쿠팡 파트너스 활동의 일환으로 일정액의 수수료를 제공받습니다.
제휴 링크 자체로 구매자에게 별도 수수료가 부과되지는 않습니다. 최종 가격과 혜택은 쿠팡 화면에서 확인하세요.
프로젝트 운영자의 본인 구매는 수익 인정 대상이 아닙니다.
[제휴 링크 비활성화와 설정](docs/advanced-usage.md#파트너스-링크-비활성화)

## 라이선스

[MIT](LICENSE). 쿠팡 및 관련 상표는 각 권리자에게 귀속됩니다.
