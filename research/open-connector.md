# Open Connector에서 `coupangctl`이 가져올 것과 버릴 것

검토일: 2026-09-04 (Asia/Seoul)

검토 기준: `oomol-lab/open-connector` 기본 브랜치의
[`d11f0e5ff87a3c1096eb337a33b1ec539ef4098b`](https://github.com/oomol-lab/open-connector/commit/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b)
(커밋 시각 2026-09-04 08:34:02 +08:00, `package.json` 버전 1.4.1). 이 문서는 해당
커밋의 저장소 코드와 저장소 안의 1차 문서만 근거로 삼는다. 인증된 트래픽이나 실제
사용자 데이터는 사용하지 않았다.

## 결론

Open Connector를 `coupangctl`의 브라우저 엔진으로 가져오면 안 된다. 이 프로젝트의
핵심은 로그인된 소비자 웹 세션을 자동화하는 브라우저가 아니라, 사용자가 API key나
OAuth로 명시적으로 연결한 SaaS 계정을 JSON Schema 기반 Action으로 실행하는 인증
게이트웨이다. 런타임 의존성에도 Playwright·Puppeteer·Chromium이 없고, 저장소에서
보이는 브라우저 세션 기능은 Anchor Browser·Kernel 같은 외부 서비스의 API를 표현한
provider Action이다. 이는 `coupangctl`의 로컬 Chrome 프로필과 headed 로그인 →
headless 읽기 문제를 해결하지 않는다. 이 판단은
[`package.json`](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/package.json)과
[`anchor_browser` Action 계약](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/anchor_browser/actions.ts),
[`kernel` Action 계약](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/kernel/actions.ts)을
함께 읽은 결과다.

다만 다음 세 가지는 강하게 참고할 가치가 있다.

1. **Action 계약을 단일 진실 원천으로 삼는 구조**: 타입, 입력·출력 JSON Schema,
   요구 권한, 실행 가능 상태를 한 계약에 두고 CLI·HTTP·MCP·문서를 같은 계약에서
   투영한다.
2. **자격 증명과 실행의 경계**: 에이전트에는 원시 credential이 아니라 안전한 계정
   표지와 실행 결과만 주고, 연결 선택과 정책 검사를 credential 조회보다 먼저 한다.
3. **설명의 리듬**: 한 문장 약속 → 제공 가치 → 적합한 사용자 → 도구 표 → 작동
   흐름 → 빠른 시작 → 상세 문서 순으로, 독자가 지금 읽을 층위를 쉽게 선택하게 한다.

반대로 외부 CAPTCHA solver·stealth·proxy 옵션, 범용 provider proxy, `raw` upstream
응답 노출은 도입하지 않는다. Open Connector에 존재한다는 사실은 이 기능을
`coupangctl`의 인증 경로에 넣어도 된다는 근거가 아니다.

## 1. 구조와 프로토콜

### 참고할 패턴

Open Connector의 `ActionDefinition`은 id, 설명, required scopes, provider permissions,
입력 Schema, 출력 Schema를 포함하는 공개 계약이며, 정의 객체는 credential이나
network call, executor code에 의존하면 안 된다고 명시한다
([타입 계약](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/core/types.ts#L197-L226)).
Provider 정의는 catalog의 진실 원천이고 executor는 별도 파일에 있으며 실행 시점에만
동적으로 읽힌다
([catalog 형식](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/catalog-format.md),
[provider loader](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/provider-loader.ts#L17-L59)).

이 분리는 현재 `coupangctl`의 typed core / CLI adapter / MCP adapter 원칙과 잘 맞는다.
가져올 핵심은 TypeScript 구현이 아니라 **계약의 소유권**이다.

- `internal/core`가 명령의 public request/result와 stable error code를 소유한다.
- Coupang의 가변 endpoint와 응답 해석은 `internal/browser` 및
  `internal/coupang/*`의 좁은 adapter가 소유한다.
- CLI와 MCP는 core 계약을 재정의하지 않고 직렬화·전달만 한다.
- 향후 Scrapling 같은 acquisition backend를 쓰더라도 그 IPC는 범용 브라우저 API가
  아니라 `ReadOrderDocument`, `ReadProductSearchDocument` 같은 제한된 작업 계약만
  가져야 한다.

Open Connector는 HTTP에 공통 JSON envelope를 사용하고, MCP는 provider별 수천 개
tool을 직접 펼치지 않고 `list_apps`, `list_connections`, `search_actions`,
`get_action_guide`, `execute_action`의 작은 발견형 도구 집합을 노출한다
([Runtime API](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/runtime-api.md#L78-L131)).
MCP 결과는 같은 payload를 사람이 읽는 JSON text와 `structuredContent`에 동시에 싣는다
([MCP 직렬화](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/mcp.ts#L467-L482)).
HTTP와 MCP는 공통 `ActionRunner`를 호출하므로 정책·연결·감사 로그의 동작이 adapter마다
갈라지지 않는다
([공통 실행 경계](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/server/actions/action-runner.ts)).

`coupangctl`에 적용할 때는 다음 정도가 적절하다.

- MCP 도구 수가 계속 늘면 `capabilities → search/inspect → execute`의 발견 흐름을
  추가하되, 기존의 명확한 도메인 도구까지 하나의 범용 `execute`로 숨기지는 않는다.
- CLI JSON과 MCP `structuredContent`가 동일한 core DTO 및 schema version을 사용하게
  회귀 테스트한다.
- capability 상태를 `implemented`, `locally_executable`, `live_verified`처럼 분리한다.
  Open Connector도 catalog-only, locally executable, externally verified를 별개의 상태로
  문서화한다
  ([검증 언어](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/verification.md)).
  `coupangctl`에서는 여기에 `observed` / `derived` / `inferred` provenance를 그대로
  유지해야 한다.

### 그대로 가져오지 않을 패턴

Open Connector의 일부 provider output은 정규화 필드와 함께 외부 서비스의 `raw`
객체를 반환한다
([Anchor Browser output](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/anchor_browser/actions.ts#L142-L177)).
이는 일반 connector에는 디버깅 편의가 있지만 `coupangctl`에는 맞지 않는다. CLI·MCP에
원시 주문 payload, raw HTML, headers, cookies를 내보내면 개인정보 원칙과 endpoint
격리 원칙을 동시에 깨뜨린다. source adapter에서 크기·형태를 검증한 뒤 typed DTO로
즉시 축소하고 원문은 버려야 한다.

또한 `/v1/proxy/:service`는 provider별 executor가 임의의 상대 endpoint를 호출할 수
있도록 만든 별도 표면이다. 경로 traversal을 막고 정책을 별도로 검사하지만, curated
Action보다 넓은 API를 열기 때문에 보안 문서도 이를 별도 위험으로 취급한다
([proxy runner](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/server/proxy/proxy-runner.ts#L183-L261),
[hardening](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/SECURITY.md#L120-L157)).
`coupangctl`에는 범용 Coupang proxy를 만들지 말고, reverse-engineered endpoint마다
method·host·path·query·응답 shape를 고정한 좁은 adapter를 유지한다.

## 2. 인증, 로그인 handoff, 세션 갱신

Open Connector의 OAuth handoff는 브라우저를 원격 조작하지 않는다. 런타임이
`authorizationUrl`과 state를 만들고, 사용자가 브라우저에서 동의한 뒤 callback이 code를
교환해 credential을 저장한다. pending state는 기본 15분 제한이며 callback에서 한 번
꺼내 쓰고, provider가 선언하면 PKCE를 사용한다
([OAuth flow](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/oauth/oauth-flow-service.ts#L76-L154),
[사용자 흐름](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/credentials.md#L128-L184)).
만료된 access token은 refresh token이 있으면 자동 갱신하고 저장한다
([refresh 문서](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/credentials.md#L303-L312)).
동일 connection의 동시 refresh는 한 Promise를 공유해 중복 refresh를 막는다
([connection refresh 조정](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/connection-service.ts#L602-L660)).

여기서 참고할 것은 “사람이 해야 하는 단계만 typed handoff로 분리하고, 이후 갱신은
하나의 장기 세션 소유자가 처리한다”는 흐름이다. `coupangctl`에 맞게 바꾸면 다음과 같다.

1. background session을 먼저 검증한다.
2. 명백한 로그인 만료 또는 challenge일 때만 local headed Chrome을 연다.
3. QR·OTP·CAPTCHA 값은 CLI/MCP 결과나 로그로 전달하지 않고 로컬 화면에서만 처리한다.
4. 성공 후 같은 product-owned profile을 잠근 장기 session이 headless 읽기를 담당한다.
5. 동시에 여러 명령이 세션 갱신을 요구하면 single-flight로 하나만 수행한다.
6. 실패는 `authentication_required`, `human_challenge_required`,
   `access_denied`, `source_unavailable`처럼 원인이 다른 stable code로 나눈다.

원시 credential은 agent에 주지 않고 안전한 account profile만 보여 주며, named
connection을 지정했는데 없으면 다른 계정으로 조용히 fallback하지 않는다는 원칙도
유효하다
([safe connection 결과](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/runtime-api.md#L108-L121),
[connection identity](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/credentials.md#L67-L78)).
단, `coupangctl`의 단일 계정 UX에서는 account id나 사용자 PII를 굳이 출력하지 말고
`authenticated`, source kind, profile health, last verified time 같은 비식별 상태만
제공하면 된다.

Open Connector의 OAuth URL handoff를 Coupang QR URL에 기계적으로 적용하면 안 된다.
일부 QR·앱 링크는 사실상 단기 bearer secret일 수 있으므로, 현재의 “credential·OTP를
출력하지 않는다” 원칙 아래서는 화면 표시와 typed 상태만 허용하는 것이 안전하다.

## 3. 안전 경계

### 참고할 패턴

Open Connector는 로그 단계와 저장용 run summary 단계에서 별도로 민감정보를 줄인다.
logger는 authorization, cookie, token, password, `values.*` 같은 경로를 censor하고
([logger](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/server/logger.ts)),
run summary는 key·context·credential-like value·민감 query를 redaction하면서 깊이,
node 수, 문자열, 배열, object key, 총 byte를 제한한다
([bounded summary](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/server/actions/run-log-summary.ts)).

`coupangctl`도 이중 방어가 필요하다.

- 구조적으로 secret/raw payload가 logger까지 오지 않게 한다.
- 그래도 error나 metadata에 섞일 경우를 대비해 sink 직전 redaction과 크기 제한을 둔다.
- URL은 전체 query가 아니라 allowlisted host/path와 redacted key/type shape만 기록한다.
- live fixture가 아니라 synthetic fixture만 저장한다.

Provider egress는 최초 URL뿐 아니라 redirect와 DNS resolved address도 검사하고,
cross-origin redirect에서는 credential-bearing header를 제거한다
([guarded fetch](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/core/guarded-fetch.ts#L168-L328)).
Scrapling worker나 다른 sidecar가 추가되더라도 동일한 규칙을 Go 경계와 worker 경계 양쪽에
적용할 가치가 있다. worker가 임의 URL을 받지 못하게 operation별 allowlist를 두고,
redirect의 최종 host·path도 다시 검증해야 한다.

### 도입하지 않을 패턴

Open Connector catalog에는 외부 Anchor Browser의 `captcha_solver`, `extra_stealth`와
Kernel의 `proxy_id` 같은 설정이 schema로 존재한다
([Anchor 설정](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/anchor_browser/actions.ts#L84-L126),
[Kernel 설정](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/providers/kernel/actions.ts#L89-L144)).
이는 Open Connector가 해당 외부 API를 기술한 것이지, 그 기능이 모든 대상 사이트에서
허용되거나 `coupangctl`에 적합하다고 보증한 것이 아니다.

`coupangctl`에는 다음을 넣지 않는다.

- CAPTCHA 자동 해결 또는 challenge 우회
- 차단을 피하기 위한 proxy/IP 회전
- fingerprint·identity 위장
- 원격 CDP/live-view URL을 agent 결과로 노출
- 사용자 승인 없이 side effect를 실행하는 범용 Action

Open Connector의 agent instruction은 create/update/delete/send 같은 외부 변경 전에
사용자 의도를 확인하라고 안내하지만, 이는 문장 기반 지침이고 hard transaction
boundary는 아니다
([MCP 지침](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/mcp.ts#L38-L48)).
`coupangctl`의 final purchase/payment 금지는 코드 수준 capability 부재로 유지해야 하며,
장바구니도 정확한 상품 식별자와 명시적 확인 없이는 실행하지 않는다.

## 4. 의존성과 단일 바이너리

Open Connector는 Bun executable 안에 서버 runtime, catalog, migration, web console을
실제로 포함한다. 실행 시 Node 설치, checkout, `node_modules`, disk extraction이 필요
없다고 명시하며, 여섯 OS/architecture artifact는 약 145–170 MiB다
([single binary](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/single-binary.md#L1-L40),
[build script](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/scripts/build-binary.ts)).
여기서 중요한 기준은 “사용자에게 명령 하나만 보인다”가 아니라 **런타임 의존성까지
artifact에 실제로 들어 있다**는 점이다.

따라서 `coupangctl`이 실행 중 `uv`로 Python과 Scrapling을 설치해야 한다면 그것은 아직
진정한 단일 바이너리가 아니다. 그런 실험은 backend adapter 뒤에서 할 수 있지만,
release 문구에는 외부 runtime과 최초 download를 정확히 밝혀야 한다. single-binary로
출시하려면 플랫폼별 Python/browser 구성요소도 서명·SBOM·checksum·attestation·native
smoke 범위에 포함하거나, Go native backend가 외부 runtime 없는 기본 경로로 남아야 한다.

Open Connector의 CI에서 가져올 만한 것은 build와 smoke를 분리하고, build job이 올린
artifact를 각 실제 OS/architecture runner가 다시 내려받아 실행하는 방식이다. checkout
credential persistence를 끄고 toolchain action과 Bun version도 고정한다
([binary workflow](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/.github/workflows/build-binary.yml#L47-L114),
[native artifact smoke](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/.github/workflows/build-binary.yml#L197-L255)).
smoke는 fresh data dir에서 health, embedded console/assets, catalog/schema, DB 생성, 종료를
확인한다
([smoke script](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/scripts/smoke-binary.ts)).

다만 이 저장소의 binary workflow는 공개 release의 checksum·SBOM·attestation 검증
모범 사례는 아니다. 문서도 cross-compile runtime download가 Bun에 의해 별도 integrity
check되지 않고 TLS에만 의존한다고 명시한다
([알려진 공급망 경계](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/single-binary.md#L41-L46)).
`coupangctl`은 현재 AGENTS.md의 더 강한 계약을 유지한다. 즉 downloaded artifact의
checksum, SBOM, attestation을 먼저 검증하고, 그 artifact 자체로 `version`, 인자 없는
help, `--help`를 smoke-test해야 한다.

## 5. README 설명 방식과 리듬

Open Connector README의 정보 구조는 다음 순서다.

1. 두 문장으로 대상과 효용을 선언한다.
2. hosted / Cloudflare / self-hosted 선택지를 한 표에서 비교한다.
3. `What It Provides`에서 기능, `Where It Fits`에서 대상 사용자를 분리한다.
4. SDK / CLI / MCP / HTTP 표로 진입점을 고른다.
5. 그림 한 장으로 credential boundary와 실행 흐름을 설명한다.
6. quick start는 실제로 성공하는 no-auth Action까지 포함한다.
7. endpoint·response envelope·MCP 예시는 상세 문서로 옮긴다.

이 구조는
[`README`](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/README.md#L18-L181)와,
“endpoint 목록과 protocol 예제를 README 밖에 둔다”고 목적을 명시한
[`runtime-api.md`](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/runtime-api.md#L1-L17)에서
확인할 수 있다. Action별 agent guide도 `Execute → Input Parameters → Required Scopes →
Provider Permissions → Policy → Current Connection → Notes For Agents` 순으로 생성해,
“무엇을 할 수 있나”와 “어떤 계정·권한으로 실행하나”를 붙여 보여준다
([Action guide renderer](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/src/server/api/action-markdown.ts)).

`coupangctl`에는 이 리듬을 다음처럼 변형하는 것이 좋다.

> 내 쿠팡 기록을 한 번 연결하고, 이후에는 화면 없이 검색·분석합니다.
>
> 결제는 하지 않으며 원시 세션과 주문 payload는 도구 밖으로 내보내지 않습니다.

그 다음 순서는 `무엇을 얻나 → 3분 시작 → 로그인은 언제 화면이 필요한가 → 가능한
기능/검증 상태 → AI·CLI 사용 예 → 개인정보와 구매 경계 → 심화 문서`가 적합하다.
Open Connector처럼 독자가 기능과 사용 대상을 먼저 이해하게 하되, 개인용 CLI에서는
quick start가 dashboard나 배포 선택지보다 뒤로 밀리면 안 된다.

문장 리듬도 참고할 수 있다.

- 한 문단에는 하나의 결정만 둔다.
- 기능 설명은 “명사 나열”보다 “검색한다 → 검사한다 → 실행한다” 순서의 동사를 쓴다.
- 장점 바로 옆에 비용이나 경계를 둔다. 예: “한 번 로그인” 바로 뒤에 “challenge일
  때만 화면을 연다.”
- status와 marketing claim을 섞지 않는다. “구현됨”과 “현재 live 검증됨”은 다른 말이다.
- 한국어 본문에서는 `credential`, `provider`, `runtime` 같은 영어 명사를 과도하게
  섞지 않고, 사용자가 보는 용어인 “로그인 상태”, “쿠팡 연결”, “백그라운드 읽기”를
  우선한다. Open Connector의
  [한국어 README](https://github.com/oomol-lab/open-connector/blob/d11f0e5ff87a3c1096eb337a33b1ec539ef4098b/docs/README.ko.md)는
  구조는 잘 보존하지만 영어 명사가 많은 편이므로 문장 자체를 복제할 필요는 없다.

## 6. 우선순위별 적용안

| 우선순위 | 적용 | 완료 기준 |
| --- | --- | --- |
| P0 | acquisition backend를 operation allowlist가 있는 좁은 typed interface 뒤에 둔다. | CLI/MCP가 브라우저·Scrapling 원문이나 범용 URL을 받을 수 없고, synthetic contract test가 같은 core DTO를 검증한다. |
| P0 | 로그인/challenge/접근 거부/source failure를 다른 stable error로 유지하고, headed handoff 뒤에는 장기 profile/session을 재사용한다. | 정상 세션의 연속 명령에서 추가 browser approval이나 로그인이 없고, 동시 갱신이 single-flight다. |
| P0 | worker와 Go 양쪽에 host/path allowlist, redirect 최종 URL 검사, body/message 크기 제한, sink redaction을 둔다. | 임의 URL, oversized body, credential-shaped error, cross-origin redirect가 synthetic test에서 fail closed한다. |
| P1 | CLI JSON과 MCP `structuredContent`를 같은 schema-versioned core contract에서 생성한다. | 동일 synthetic 입력에 의미상 동일한 JSON 결과가 나오고 adapter drift test가 있다. |
| P1 | capability 상태에 구현·실행 가능·live 검증을 분리한다. | README, `capabilities`, ROADMAP가 같은 용어와 `last_verified` 근거를 쓴다. |
| P1 | README를 사용자 약속 → 3분 시작 → 로그인 흐름 → 기능/상태 → 경계 순으로 다듬는다. | 첫 화면에서 설치, 로그인 때만 headed, 이후 headless, 결제 미지원이 함께 이해된다. |
| P2 | release smoke를 build job이 아닌 downloaded artifact 중심으로 유지·확장한다. | checksum·SBOM·attestation 검증 후 각 artifact의 `version`, no-arg help, `--help`, 최소 synthetic flow가 native runner에서 통과한다. |

## 최종 채택표

| Open Connector 패턴 | 판단 | 이유 |
| --- | --- | --- |
| 정의/Schema와 executor 분리 | 채택 | typed core와 불안정 endpoint adapter 분리를 강화한다. |
| 하나의 core runner를 CLI/HTTP/MCP가 공유 | 채택 | adapter별 정책·오류·provenance drift를 막는다. |
| 작은 발견형 MCP tool set | 조건부 채택 | tool 폭주를 막되 명확한 도메인 도구를 전부 범용 execute로 숨기면 안 된다. |
| safe connection profile, 명시적 connection 선택 | 채택 | credential/PII 노출과 잘못된 계정 fallback을 막는다. |
| OAuth typed browser handoff와 자동 refresh | 개념 채택 | Coupang에는 OAuth가 아니므로 headed local handoff + persistent profile + single-flight renewal로 번역한다. |
| bounded/redacted run summaries | 채택 | raw payload 금지와 운영 진단을 동시에 만족한다. |
| redirect/DNS/SSRF guard | 채택 | sidecar와 reverse endpoint가 임의 네트워크 클라이언트로 변하는 것을 막는다. |
| Bun single executable 구현 | 원칙만 채택 | Go 제품에 Bun을 넣지 않는다. 외부 runtime까지 실제 artifact에 포함해야만 single-binary라 부른다. |
| downloaded artifact의 native smoke | 채택·강화 | 현재 checksum/SBOM/attestation 및 CLI smoke 계약과 결합한다. |
| 범용 provider proxy | 미채택 | reverse-engineered Coupang endpoint는 operation별 좁은 adapter여야 한다. |
| upstream `raw` 결과 노출 | 미채택 | 개인정보·세션·불안정 shape가 core/CLI/MCP로 새어 나온다. |
| 외부 CAPTCHA solver·stealth·proxy 설정 | 미채택 | catalog에 존재할 뿐, 정당한 인증 handoff나 호환성 보장이 아니다. |
| 범용 side-effect Action | 미채택 | final purchase/payment는 capability 자체가 없어야 한다. |

## 2026-09-04 적용 결과

검토 직후 다음 항목을 제품에 반영했다.

- `auth.Service`의 로그인·복구 전환을 single-flight로 직렬화했다. 동시 복구
  요청은 첫 로그인 뒤 상태를 다시 확인하며, 합성 동시성 테스트가 visible QR
  전환이 하나뿐임을 검증한다.
- MCP에 로컬 정적 `capabilities` 도구를 추가했다. 기존 도메인 도구를 범용
  `execute`로 숨기지 않으면서 schema version, 구현 상태, 검증 상태, 다음 작업을
  AI가 사용자 데이터나 브라우저 접근 없이 발견할 수 있다.
- `mcp --current-browser`에서 현재 Chrome은 주문·상품 acquisition만 소유하고,
  `auth_status`와 확인 기반 QR 복구는 전용 영구 프로필 adapter가 맡도록 composition
  root를 분리했다.
- README 첫 화면을 사용자 약속 → 가치 → 3분 시작 순서로 정리하고, 제휴 고지는
  삭제하지 않은 채 실제 파트너스 링크 설명 위치로 옮겼다.
- 빠른 시작을 `설치 확인 → 최초 연결 → 평소 사용`의 세 박자로 나눴다. 내부
  구현 명사보다 사용자가 보게 될 동작과 결과를 먼저 설명하고, 구현 상태와
  실환경 검증 상태도 한 문장에 섞지 않도록 문단을 분리했다.

원시 upstream 결과, 범용 proxy, CAPTCHA solver, stealth, proxy rotation은 도입하지
않았다.
