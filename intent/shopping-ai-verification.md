# 쇼핑 AI 목표 검증 기록

기준: [INTENT.md](../INTENT.md). 최근 추가 검증: 2026-09-11.
전체 목표는 미완료다. 부분 기능의 합성 테스트나 이전 확장 검색 성공을 전체 접속,
개인화 추천, 전체 주문 이력 확보의 증거로 사용하지 않는다.

## 2026-09-11: 기본 읽기 흐름 실측과 공통 오류 계약

사용자의 “큰 틀에서 동작하도록 진행하고 마무리” 요청에 따라 새 기능 조사 대신
기존 Camofox 경로의 실제 CLI 실행과 MCP 연결을 확인했다.

| 실제 실행 | 관찰 결과 | 검증 범위 |
| --- | --- | --- |
| `auth status` | 약 4.8초, `verified` | `authenticated_session`이며 안정적 계정 식별 검증은 아님 |
| `orders preview` | 약 4.5초, 현재 첫 페이지 5건, 다음 페이지 있음 | `source_entry_page`, `history_complete=false`, `account_identity_verified=false`, `persisted=false` |
| `orders stats` | 약 12ms, 기존 로컬 주문 10건·품목 행 13개 | `retained_local_history`; 현재 로그인 계정의 전체·최신 통계로 해석하지 않음 |
| 서로 다른 CLI 검색 두 건 | `미니 식기`, `스텐 식기` 각각 약 5.9초, 상품 3개·필터 20개 | 상품 식별자·이름·URL과 schema v4 확인. 모든 가격·필터 선택 적용을 이 실측으로 검증한 것은 아님 |
| 검색에서 얻은 정확한 옵션 상세 | 약 4.7초, 리뷰 표본 1개·상세 이미지 URL 2개 | 요청한 상품·item·vendor item과 응답이 일치. 이미지의 시각적 검토는 수행하지 않음 |
| 제한된 추천 조사 | 약 44.2초, 소스 목록 5페이지·후보 71개 발견·2개 상세 확인 | schema v5, `incomplete`, `review_budget_reached`, 후속 행동 3개. 조사 한계를 숨기거나 적합성 확정으로 바꾸지 않음 |
| 실제 추천 결과를 MCP로 렌더링 | 도구 26개 발견, HTML 59,648바이트 | report v2, `private_local`, `incomplete` 유지. 메모리에서만 생성; 저장·공유·화면 QA는 하지 않음 |

추천 검증의 `discovery-target=2`, `search-page-limit=1`, `review-cap=2`, `limit=2`는
실측 비용을 제한한 값이지 최적 추천 개수나 충분한 조사량을 뜻하지 않는다. 목록 페이지
제한은 정렬 축마다 적용된다. 구매 이력을 추천에 결합하는 옵션은 이 실측에서 사용하지 않았다.
원시 응답은 메모리에서만 처리했고 결과에는 상태·개수·범위만 기록했다. 새 주문 동기화,
계정 자동 귀속, 프로필 복사·초기화, 쿠키 삭제, 장바구니·주문·리뷰 변경은 수행하지 않았다.

오류는 core의 공통 분류를 CLI와 MCP가 함께 사용하도록 정리했다. `code`, `message`,
`reason`, `operation`, `retryable`, `next_action`을 보존하며 MCP 실패는 `isError`와
JSON 텍스트를 반환한다. typed 성공 schema는 유지한다. 입력 검증은 의존성 획득 전에
수행하며 원시 오류 문자열로 실패 유형을 추정하지 않는다. 계약과 호환성 변경은
[README 오류 계약](../README.md#climcp-오류-계약)을 참조한다.

합성 검증에서는 실제 CLI bootstrap·MCP stdio 경로에서 인증 필요·접근 거부가 같은
JSON으로 전달됨을 확인했다. 검색은 `result` 안에, 상세는 최상위에 문서를 반환하는
차이를 무시하던 테스트 fixture도 실제 계약에 맞췄다. 생산 코드의 문서 검증은 완화하지 않았다.
마지막 smoke에서 검색·상세·통계 `--help`의 오류 종료를 재현했고, 등록된 옵션을
설정·DB 없이 정상 출력하도록 수정했다. `-h`도 같은 회귀 테스트로 확인한다.

`go test ./...`, 주요 패키지(core/cli/mcpserver/browser/products/orders)의 `go test -race`,
`go vet ./...`, `git diff --check`가 통과했다. `npm run test:camofox`의 브라우저 테스트 140개는
합성 검증이며 위 실측과 구분한다. 개발 바이너리의 `version`, 무인자 도움말,
`--help` 및 주요 읽기 명령 도움말도 모두 종료 코드 0으로 통과했다. 설치된 바이너리는 교체하지 않았으며
다운로드 릴리스의 checksum·SBOM·attestation 수용 검증을 대체하지 않는다.

남은 전체 목표는 안정적 계정 식별에 따른 이력 귀속, 전체 이력·취소/반품·신선도 검증,
실제 구매 근거와 명시한 선호를 결합한 추천이다. 기존 통계와 현재 로그인 세션을 자동으로
같은 계정에 귀속하지 않는다. 기본 읽기 흐름의 통과가 이 조건들의 완료를 뜻하지 않는다.

## 2026-09-11: MCP 공개 탐색과 로컬 저장소 분리

- MCP 상품 검색·상세·추천이 소스 조회 전에 주문 DB를 열던 의존성을 분리했다.
  기존 runtime owner가 가격 기록 또는 명시적인 구매 맥락 조회 시점에 DB를 연다.
  가격 이력 기능은 유지하며 저장 실패는 조회 결과의 경고로 반환한다. 요청한
  구매 이력이 없거나 읽히지 않으면 개인화 완료로 표시하지 않는다.
- 실제 CLI bootstrap과 MCP stdio를 지나는 합성 테스트에서, 정상 검색 응답을
  준비해도 읽을 수 없는 DB 때문에 검색이 실패하던 현상을 먼저 재현했다.
  수정 후 무결과 근거 보존, 정상 가격 기록·조회, 저장 실패 시 상품·경고 보존,
  구매 이력 opt-in, 미수집/읽기 실패 구분, 소스 실패 시 DB 미생성을 확인했다.
- 설치된 Camofox와 새 임시 전용 프로필로 실제 `products_search`를 실행했다.
  조회할 수 없는 합성 DB가 있는 상태에서도 `미니 식기` 상품 3개를 약 6.7초에
  반환했다. schema v4, source `camofox_search_document`였다. 로그인 세션을
  복사하지 않았고 기존 사용자 프로필·주문 DB는 이 검증에 사용하지 않았다.
- 실측 상품 3개의 가격에는 선택 옵션 범위 근거가 없어
  `price.current_amount.scope`가 미확인이었다. 따라서 이 실측은 검색의 DB
  독립성을 입증하지만 가격 기록 성공·옵션 가격 검증을 입증하지 않는다.
  가격 저장 성공·실패 처리는 합성 테스트의 근거다.
- 추가 계정 식별 조사는 내 계정 화면의 새 공개 코드 6개와 그 코드에서 발견한
  대시보드 GET을 확인했다. 응답은 잔액 키 두 개뿐이어서 안정적 계정 식별자는
  채택하지 않았다. 상세 근거는 [endpoint catalog](../research/endpoint-catalog.md)에
  기록했다. 기존 이력의 자동 귀속·병합은 실행하지 않았다.
- 임시 실측 코드는 제거했고 합성 회귀 테스트만 유지했다. 이번 수정은 전체
  계정 이력 수집이나 실제 구매 맥락 기반 추천의 완료를 뜻하지 않는다.
- `go test ./...`, `go test -race ./internal/cli ./internal/products ./internal/mcpserver`,
  `go vet ./...`, `git diff --check`가 통과했다. 설치된 배포 바이너리를 교체하거나
  다운로드 릴리스의 수용 검증을 수행한 것은 아니다.

## 2026-09-07 당시 기록

아래 내용은 당시의 구현과 검증 결과이며 최신 Camofox 지원 상태로 해석하지 않는다.

| 요구 | 현재 근거 | 남은 완료 조건 |
| --- | --- | --- |
| 전용 프로필, 확장 없는 조용한 검색 | 사용자 최초 허용 후 `--apple-events` 별도 CLI 실행 두 건에서 각 실제 상품 3개 반환. 동일 창·탭 최소화 상태 12회 관측 | MCP·상세 연결 및 영속 초기 설정/재시작 재사용 |
| 실제 상품 검색·추천 | 검색·상세 서비스와 추천 서비스 존재. 새 CLI/MCP 추천 호출의 합성 테스트 통과 | 정상 접속 경로에서 실제 검색·상세·추천을 검증하고 범위·미확인 필드를 확인 |
| 내 구매 이력·즉시 통계 | 저장소/주문 서비스 테스트 통과. 실제 로컬 통계 명령 12ms 응답 | 로컬 저장소는 주문 0건, sync-status는 never_run. 승인된 최초 동기화와 범위·신선도 검증 필요 |
| 구매 맥락 기반 추천 | opt-in 구매 집계를 추천 후보에 연결하는 typed CLI/MCP 경로와 SQLite 통합 합성 테스트 통과 | 실제 계정 동기화 후 구매 근거·명시한 선호를 사용한 end-to-end 추천 검증 필요. 자동 취향 점수나 개인화 완료를 주장하지 않음 |
| 식재료 구매 준비·주문·리뷰 비전 | 의도 문서에 보존 | 별도 설계·실제 근거·승인 흐름 필요. 기존 최종 구매/결제 자동화 금지 원칙 유지 |

## 이번 구현·검증

- `products recommend`, `products_recommend`를 기존 typed 추천 서비스에 연결했다.
  기본 읽기는 background 모드이며 확장/Apple Events 연결을 새 명령이 지원한다고
  주장하지 않는다. 기본값은 공개 상품의 근거 조사이며, 구매 이력 결합은 별도 opt-in이다.
  주문·리뷰 게시를 하지 않는다.
- 추천에서 review_limit=40을 생성하지만 상세 입력은 최대 20만 허용하는 불일치를
  회귀 테스트로 확인하고 수정했다. 기존에는 실패를 삼킨 뒤 후보 0개로 complete를 반환했다.
- source 조회 실패는 incomplete, 실제 검색 무결과는 no_matches로 구분한다.
  상세에서 확인된 예산 초과·미확인 가격은 후보에서 제외한다. 성공한 상세 후보도
  category_specific_fit는 추가 검증 대상으로 유지한다.
- 추천 JSON 키를 기존 API의 snake_case 규칙에 맞추고 schema version을 3으로 올렸다.
  affiliate 변환 제외 선택을 CLI/MCP 입력에서 검색·상세까지 전달한다.
- `go test ./internal/core ./internal/store ./internal/orders ./internal/products ./internal/mcpserver`의
  각 대상은 통과했다. 새 CLI 추천 테스트는 production Go files + run_test.go +
  product_recommendation_test.go로 실행해 통과했다. 실제 사이트를 사용한 테스트는 아니다.
- 전체 CLI 테스트는 통과하지 않는다. 기존 current_browser_broker_test.go가 현재 없는
  core 타입과 doctor 필드를 참조해 빌드 실패한다. 그 파일을 제외한 run_test.go에서도
  기존 current-browser 선택/차단 안내 테스트 두 개가 실패한다. 테스트 삭제나 성공 주장 없이
  남겼으며 새 추천 테스트의 통과와 분리한다.
- 임시 경로의 개발 바이너리 빌드가 성공했다. 루트의 기존 실행 파일과 Native Messaging
  설치는 덮어쓰지 않았고 릴리스 검증을 수행한 것은 아니다.
- 새 빌드의 orders sync-status는 never_run/history_complete=false, orders stats는 0건이었다.
  원시 주문 자료·쿠키·계정 식별자는 출력하거나 테스트 fixture로 저장하지 않았다.

## 구매 맥락 연결 추가 검증

- `--use-purchase-history` / `use_purchase_history=true`를 통해 최종 후보에 한정된
  로컬 집계를 받는다. 서비스는 작은 repository interface로 SQLite 또는 합성 adapter를
  받으며 기본 호출은 구매 이력을 읽지 않는다. CLI와 MCP 모두 실제 ledger를 연결한다.
- SQLite snapshot에서 sync 상태와 후보별 상품/판매 옵션 ID 집계를 읽는다.
  전체 취소 주문, 취소·반품 상태 행, 비상품 구매, 잔여 수량 0 행을 제외하고 부분 취소·반품
  수량을 차감한다. 주문 수와 품목 행 수는 구분한다. 상품 단위 근거는 옵션 단위로 표현하지 않는다.
- 응답의 private_local 범위·파생 근거·구매 월·집계 규칙·동기화 상태·누락 한계를 명시한다.
  취향/재구매 점수와 순위 변경은 추가하지 않았다. 이력이 불완전하거나 저장소 읽기 실패 시
  추천은 incomplete이며 공개 후보는 보존한다. 원시 저장소 오류는 응답에서 제외한다.
- 합성 테스트: 동의 없는 조회 0회, 최종 후보만 조회, 옵션/상품 구분, 중복 입력 제거,
  부분 취소/반품·전체 취소·멤버십 제외, never_run/완료/진행 중 coverage, 빈 저장소,
  식별자/후보 수 제한, 오류 비노출, 실제 SQLite→추천 서비스 결합, CLI/MCP 입력 전달.
- 이번 추가 작업에서는 브라우저 창·쿠키·계정 상태·기존 우회 실험을 변경하지 않았다.
  실제 계정 주문 기반 추천은 검증하지 않았으며 최초 동기화가 여전히 남아 있다.
- 재검증: core/store/orders/products/mcpserver 전체 패키지 테스트, 위의 새 CLI 테스트,
  store/products/mcpserver의 추천 관련 `-race` 테스트, 개발 바이너리 빌드가 통과했다.
  전체 CLI 테스트는 위에 기록한 기존 브로커 타입 누락으로 여전히 빌드 실패다.
  새 바이너리의 `products recommend --help`는 사용법에 새 옵션을 표시하지만
  `invalid_command`/exit 1이므로 도움말 성공으로 집계하지 않았다.

## 외부 상태

### 전체 테스트 복구 점검

2026-09-07 `go test ./...`를 재현했을 때 browser, cli, recommendationreport,
중첩 Users 경로의 browserakamai 실험, internal/tools/20260905의 총 5개 패키지가
컴파일에 실패했다. 다음 변경 후 재실행에서는 browser와 cli 2개만 남았다.

- 추천 보고서는 core/실제 source에 없는 ReviewSampling 모델을 참조했다.
  보고서와 합성 fixture를 현재 ProductInspection.Reviews에 맞추고 표본 전략은
  미확인으로 표시했다. 미관찰 가격은 0원 대신 미확인이며, 상품명을 HTML로
  재해석하던 비교 선택기는 textContent를 사용한다. 렌더링/가격 근거/문자열 삽입
  회귀 테스트와 패키지 `-race` 테스트가 통과했다. 실제 화면 시각 QA는 하지 않았다.
- 중첩 `Users/.../browserakamai/bypass.go`의 문자열 키·필드/메서드 충돌·헤더 값
  타입을 수정했다. 기존 헤더 값과 실험 파일은 보존했고 원본 요청을 복제한 뒤
  헤더를 붙이도록 했다. 합성 transport 테스트와 `-race`만 실행했으며 실제 쿠팡
  요청이나 production 경로 통합은 하지 않았다. 이를 우회 성공으로 취급하지 않는다.
- 독립 debug_html 도구의 Go 문법 오류를 수정하고 원문 HTML 출력은 메타데이터로
  제한했다. `go test`로 빌드만 확인했으며 도구의 main/네트워크 요청은 실행하지 않았다.
- 남은 browser 테스트는 미구현 CDP resource/hidden-target/compatibility 기능을,
  CLI 테스트는 미연결 current-browser broker 타입·doctor 상태를 참조한다.
  테스트 삭제·상수 성공 stub·실제 사용자 Chrome 실행으로 통과시키지 않았다.

이 점검은 저장소 검증 가능성의 부분 복구이며, 실제 검색·전체 이력·개인화 추천
완료를 뜻하지 않는다.

이전 점검은 전용 창 0개에서 중단됐으나, 사용자의 초기 준비 자동화 요청 이후 전용
빈 창을 다시 열었다. 추가 창 열기 승인은 대기 상태가 아니다. 자동 클릭 후에도
JavaScript probe는 권한 미허용이었다. Chromium의 보호된 설정 처리와 실제 관측은
[확장 없는 제어의 최초 설정 기록](../research/extensionless-chrome-control.md)을 참조한다.
후속 사용자 직접 허용으로 JavaScript probe가 통과했고, 15:05 KST 실제 두 검색도
성공했다. ID 형식과 조용한 탐색 방식 수정, 표본 최소화 관측은
[최소화 검색 검증 기록](minimized-search.md)에 모았다. 계정 동기화와 전체 목표는 미완료다.

## 다음 검증 순서

1. 실측 통과한 검색 경로의 영속 프로필 설정·재시작 재사용을 검증.
2. 검증된 읽기 경로를 CLI/MCP에 연결하고 기존 테스트 불일치를 원인별로 해결.
3. 승인된 최초 주문 동기화 후 통계의 기간·누락·취소/반품·신선도 검증.
4. 로컬 구매 근거와 추천 근거를 결합하고 개인화 추론을 명시한 end-to-end 사례 검증.
5. 별도 승인 범위에서 구매 준비·리뷰 초안/검토 경험 설계. 최종 주문·결제는 자동화하지 않음.
