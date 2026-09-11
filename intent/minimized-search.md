# 최소화 검색: spec, plan, 검증 기록

입력: [INTENT.md](../INTENT.md) · 2026-09-07
상태: 기존 진행 작업을 문서화한 구현·검증 계획. 별도 문서 승인/릴리스 없음.

## 최신 추가 요구: 전용 프로필, 확장 없는 경로 우선

사용자는 일상용 Chrome을 계속 쓰므로 별도 데이터 디렉터리·실행 인스턴스를
우선한다. 확장 경로의 기존 성공 기록은 보존하되 Apple Events 경로 성공으로
해석하지 않는다. 새 경로는 실험적 `products search --apple-events`까지 연결했으며
CLI 기본값은 그대로다. 2026-09-07 15:05 KST 전용 Chrome에서 실제 연속 두 검색을
확장 없이 검증했다. 영속 프로필·재시작 재연결·상세/MCP 연결은 남아 있다.

- 진단 도구: `research/probes/chrome-apple-events.mjs`와 PID-bound ScriptingBridge helper.
- 최신 관측/후속 검증: [확장 없는 Chrome 제어](../research/extensionless-chrome-control.md).
- 아래 Spec과 연속 검색 성공은 기존 확장 구현의 기록이다.

### Apple Events CLI 구현·검증

- Go 어댑터: `internal/browser/apple_events_search.go`와 내장 PID-bound
  `apple_events_transport.js`. 기존 순수 DTO reader를 바이너리에 포함하므로 실행 시
  Node나 설치된 확장은 사용하지 않는다. Chrome 실행/활성화/최소화/종료는 하지 않는다.
- `--apple-events`는 products search만 지원한다. 실행 환경의
  `COUPANGCTL_APPLE_EVENTS_PID`, `COUPANGCTL_APPLE_EVENTS_PROFILE`로 대상을 명시한다.
  현재 검증 범위에 맞춰 `/tmp/coupangctl-apple-events.SUFFIX` 전용 폴더만 허용한다.
  영속 초기 설정과 재시작 재사용은 미완료이며 이 실험적 입력을 최종 사용자 경험으로
  간주하지 않는다. 기존 일반 브라우저·확장 경로를 대체하지 않는다.
- 프로필 잠금으로 동시 CLI 탐색을 제한한다. 매 Apple Events 호출 전에 프로세스의
  실행 파일·정확한 데이터 폴더·CDP/headless/extension launch 부재를 검증한다.
  창 하나/탭 하나, 동일 ID, 최소화 상태를 확인하고 허용된 검색 URL만 이동한다.
- 소스와 전송 오류는 bounded typed error로 반환한다. 상품 DTO는 Go validator와
  기존 상품 parser를 모두 통과해야 하며 bytes/HTTP status만으로 성공 처리하지 않는다.
- 합성 검증: Go production files + apple_events_search_test.go 일반/`-race` 통과;
  CLI production files + run_test.go + apple_events_test.go + product_recommendation_test.go의
  관련 테스트 통과; 기존 JS probe/extension 테스트 9개와 내장 전송 실행 테스트 3개 통과.
  전체 browser 테스트는 기존 background compatibility/CDP resource 관련 누락 타입으로
  빌드 실패하고 전체 CLI 테스트도 기존 브로커 타입 누락으로 실패한다. 삭제하지 않았다.
- 2026-09-07 실제 개발 바이너리의 `products search --apple-events --query '미니 식기'`
  호출은 `apple_events_unavailable`로 종료했다. 전용 PID는 살아 있으나 창이 0개라는
  OS/Apple Events 관측과 일치한다. 새 창/다른 Chrome/사이트 요청으로 대체하지 않았다.
  이는 실패 경로의 실측이며 상품 검색 성공 증거가 아니다.
- 후속 자동화 요청으로 전용 창을 다시 열었다. 일반 초기 준비에 대한 추가 승인은
  필요하지 않다. JavaScript 권한의 자동 클릭은 적용되지 않았으며, Chromium은 이
  설정의 외부 프로세스/합성 이벤트를 거절한다. 최초 사용자 직접 클릭 이후 probe와
  당시 연속 검색 검증이 남아 있었다. 원인과 최초 설정 기준은 위의 확장 없는 제어
  문서를 참조하며, 후속 성공 실측은 아래에 기록한다.

### Apple Events 실제 연속 검색 — 2026-09-07 15:05 KST

사용자가 전용 Chrome의 설정을 직접 허용한 뒤 상수 JavaScript 실행이 성공했다.
실제 CLI에서 발견한 두 전송 문제를 수정했다. Chrome의 문자열 창·탭 ID를 검증된
숫자 ID로 변환하고, 창을 복원시키던 native URL setter 대신 허용된 탭 내부의
`window.location.assign`으로 탐색한다. 순수 reader와 기존 Go parser로 상품을 검증했다.

임시 개발 바이너리를 빌드하고 `products search --apple-events --limit 3 --no-affiliate`
명령을 검색어별로 별도 프로세스에서 실행했다. 기존 설치 바이너리는 덮어쓰지 않았다.

| 검색어 | 응답 시각 (KST) | 종료 코드 | 상품 수 | 확인 필드 |
| --- | --- | --- | --- | --- |
| 미니 식기 | 15:05:16 | 0 | 3 | 상품/옵션 ID, 이름, 가격, 정규화 URL |
| 스텐 식기 | 15:05:18 | 0 | 3 | 상품/옵션 ID, 이름, 가격, 정규화 URL |

- 두 응답 모두 schema version 3, source `apple_events_search_document`였다.
- 전후 및 실행 중 각 6회, 총 12회 OS 상태 조회에서 같은 창·탭 하나와
  `minimized=true`를 확인했다. 실행 중 관측 간격은 약 0.6초이며 연속 이벤트 추적은 아니다.
  성공한 두 실행 사이 창 복원·재최소화·연결 버튼·프로세스 재시작을 하지 않았다.
- 이 경로는 설치된 확장/Node/CDP에 의존하지 않는다. Node는 외부 검증용 상태 관측에만
  사용했다. 일상용 Chrome의 프로필·쿠키·탭과 기존 우회 실험은 변경하지 않았다.
- ID 문자열/잘못된 ID 회귀 테스트를 실패 상태에서 추가한 뒤 수정하여 통과했다.
  JS 관련 테스트 14개, 선택한 Go Apple Events 어댑터/CLI·추천 회귀 테스트 및 개발 빌드가
  통과했다. 기존 전체 테스트의 다른 빌드 문제까지 해결됐다는 뜻은 아니다.
- 평점·리뷰·배송 배지는 미수집이며 기본 false 값을 실제 부재로 해석하지 않는다.
  재시작 후 재사용, 임시 폴더를 대체할 영속 설정, 상세/MCP·계정 이력 연결은 미검증이다.

## Spec

- `--ordinary-browser`는 명시적 opt-in이다. 기본 브라우저 경로는 유지한다.
- 확장 popup에서 선택한 쿠팡 검색 탭에 연결한다. Native Messaging 세션을
  유지하여 연속 CLI 요청을 처리하며 `tabs.update(id, {url})`만으로 이동한다.
- URL은 HTTPS `www.coupang.com/np/search`에 한정한다. 확장이 읽는 결과는
  상품 식별자/URL/이름/확인된 가격/출처·위치로 제한하고 Go 경계에서 검증한다.
- 구조화 상품 자료를 우선한다. DOM fallback은 좁은 reader에 격리한다.
- 쿠키 접근 권한, CDP, 창 활성화 호출은 이 경로에 필요하지 않다.
- 연결은 최대 30분이다. 차단이나 권한 상실 시 자동 창 복원 대신 오류를 반환한다.

## Plan / 진행 상태

1. 구현: `extension/search-{action,reader}.js`, popup/service worker,
   `internal/browser/ordinary_{protocol,search,session}.go`, CLI opt-in 연결 — 작성됨.
2. 합성 검증: JS 확장 테스트, Go 프로토콜/반복 rendezvous 테스트 — 통과 기록 있음.
3. 로컬 설치: CLI 빌드 및 `browser-bridge install` — 설치 완료.
4. 실제 Chrome에 확장 0.2.0 로드 및 사용자 최초 연결 — 확인됨.
5. 최초 연결 후 최소화하여 서로 다른 검색어 두 개 실행; OS 상태를 수동 복원 없이
   관측하고 상품 결과를 검증 — 2026-09-07 10:30 KST 두 검색 모두 확인됨.
6. 사용 설명/개인정보 범위 갱신 및 관련 테스트 재실행 — 완료, 스토어 제출 없음.

## 검증 판정

모의 테스트 통과는 실제 쿠팡 접속 성공의 증거가 아니다. 이전 headless 시도의
실패는 [관측 기록](../research/headless-search-observations.md)을 참조한다.
현재 작업의 실측 성공/실패는 아래에 시각·명령·상품 수·창 상태만 남긴다.
쿠키, 토큰, 원시 주문 자료나 고객 fixture는 기록하지 않는다.

2026-09-07 로컬 점검:

- 확장 합성 테스트 12개, 선택한 Go ordinary bridge 프로토콜/세션 테스트 통과.
- `go test ./internal/browserbridge ./internal/extensionpack ./internal/coupang/products ./internal/products` 통과.
- `go build -o ./coupangctl ./cmd/coupangctl` 통과. 전체 `go test ./...` 통과 주장은 하지 않음.
- 실제 CLI는 확장 최초 연결을 기다리다 `product_source_unavailable`로 종료했다.
  OS 입력이 팝업을 대상으로 유지되지 않아 연결 버튼 실행을 확인하지 못했다.
  사이트 검색 요청이나 최소화 연속 검색 성공의 증거가 아니다.

### 최초 연결 후 실제 연속 검색

2026-09-07 10:30 KST, 사용자가 검색 연결 버튼을 눌렀고 Chrome의 자식
Native Messaging 프로세스가 실행 중인 것을 확인했다. 최초 한 번 창을
최소화한 다음 아래 두 명령을 별도 CLI 프로세스로 실행했다.

```bash
./coupangctl products search --query '미니 식기' --limit 3 --no-affiliate --ordinary-browser
./coupangctl products search --query '스텐 식기' --limit 3 --no-affiliate --ordinary-browser
```

| 검색어 | 응답 시각 (KST) | 종료 코드 | 상품 수 | 확인 필드 |
| --- | --- | --- | --- | --- |
| 미니 식기 | 10:30:31 | 0 | 3 | 상품 ID, 이름, 가격, 상품 URL |
| 스텐 식기 | 10:30:56 | 0 | 3 | 상품 ID, 이름, 가격, 상품 URL |

- 둘 다 schema version 3, source `ordinary_browser_search_document`인 JSON을 반환했다.
- 같은 Native Messaging 프로세스가 두 실행 이후에도 유지됐다. 재연결 버튼이나
  창 복원/활성화 동작 없이 연속 요청을 처리했다.
- 첫 검색 전, 첫 실행 이후 6회, 두 번째 실행 이후 8회, 마지막 확인까지
  Chrome의 native `minimized of windows` 조회는 모두 단일 `true`였다.
  중간 관측 간격은 약 0.8초 이상이며 이벤트 단위 무중단 추적은 아니다.
  관측 과정은 읽기 전용이고 최소화된 창을 복원하지 않았다.
- Orca는 최소화 뒤 on-screen 창을 나열하지 못하므로, 이후 관측에는 Chrome의
  읽기 전용 OS 속성을 사용했다. CDP나 원격 디버깅 연결은 사용하지 않았다.
- 가격은 반환됐지만 평점·리뷰·배송 배지는 이 어댑터에서 미수집이다.
  해당 필드의 기본값을 실제 미제공/비해당 사실로 해석하면 안 된다.

판정: 현재 환경에서 요청한 최소화 연속 검색은 실측 통과. 엄격한 headless,
30분 만료 후 재연결, 모든 정렬/필터 조합의 성공을 입증한 결과는 아니다.

## 남은 위험

쿠팡 문서 구조 변경, background tab 로딩 지연, Chrome activeTab 권한 만료,
서비스 워커/Native Messaging 단절. 이 경우 실패를 명시하고 조용히 종료해야 한다.
