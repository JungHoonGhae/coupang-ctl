# README 다이어그램

[`readme.html`](readme.html)을 다운로드해 브라우저로 열면 세 그림을 볼 수 있습니다.
원본 폰트와 라이선스가 내장되어 오프라인에서도 Pretendard로 표시됩니다. GitHub README에는
같은 그림의 2배 해상도 PNG를 사용합니다. GitHub 본문 폰트는 바꾸지 않습니다.

| 그림 | 설명 | README용 이미지 |
| --- | --- | --- |
| Architecture | CLI·MCP → 공통 서비스 → Camofox / SQLite의 호출 경계 | [architecture.png](architecture.png) |
| Recommendation | 필터 발견·검증 → 옵션 조사 → 조건 판정 → 근거 보고서 | [recommendation.png](recommendation.png) |
| Orders | 저장하지 않는 preview와 sync 이후 로컬 분석의 분리 | [orders.png](orders.png) |

실제 고객·주문·상품 자료를 사용하지 않은 기능 설명입니다. 전체 시장 추천, 계정 연결,
전체 주문 수집, 최종 구매·결제의 완료를 주장하지 않습니다. 세부 명령·응답 계약은
[상세 사용 가이드](../advanced-usage.md)에 유지합니다.

## 디자인

`diagram-design`의 minimal 템플릿과 Flowchart·Architecture 규칙을 사용합니다.
처리 역할별 swimlane이 필요한 Data Flow 유형은 사용하지 않았습니다.
각 그림은 9개 이하 노드, 12개 이하 연결, 최대 두 강조 요소로 제한합니다.
따라서 모든 서비스 파일, 필터 옵션, 오류 코드를 그림에 넣지는 않습니다.

- 크기: `fit`, 폭 720px. README와 휴대폰을 위한 세로 배치.
- 색: 기존 보고서의 paper `#faf9f6`, ink `#252724`, accent `#eb6c36`.
- 글꼴: 사용자 지정 Pretendard. 제목·한글·기술 이름 모두 같은 글꼴을 사용합니다.
- 접근성: 각 SVG의 title·desc·고유 ID와 README의 대체 텍스트를 유지합니다.
- 모션·외부 이미지·외부 폰트 요청 없음. 전역 스킬 설정은 수정하지 않습니다.

## 수정과 재생성

1. [`readme-source.html`](readme-source.html)의 문구와 SVG를 수정합니다.
2. 아래 공식 폰트를 임시 경로에 다운로드합니다.
3. 렌더러를 실행합니다. `readme.html`과 세 PNG는 생성물이며 같은 경로에 다시 씁니다.

```bash
curl --fail --location --output /tmp/coupangctl-PretendardVariable.woff2 \
  https://raw.githubusercontent.com/orioncactus/pretendard/v1.3.9/packages/pretendard/dist/web/variable/woff2/PretendardVariable.woff2
node docs/diagrams/render.mjs /tmp/coupangctl-PretendardVariable.woff2
```

기존 개발 의존성 `playwright-core`와 Playwright Chromium이 필요합니다.
렌더러는 브라우저를 자동 설치하지 않으며 계정·Camofox 설정·사용자 프로필을 읽지 않습니다.
임시 headless 브라우저에서 네트워크를 차단하고 실제 폰트 로딩, 노드 글자 잘림,
접근성 ID, 320px·390px 가로 넘침을 검사합니다. PNG는 SVG 영역만 캡처합니다.
`readme-source.html`은 편집용이므로 폰트 자리표시자를 포함합니다. 공유용 완성본은
폰트가 내장된 `readme.html`입니다.

스킬의 기본 `self_check.py`는 폰트의 `data:` URL도 거부합니다. 사용자 지정
Pretendard를 오프라인으로 제공하기 위한 예외이며 검사기를 수정하지 않습니다.
편집 원본은 기본 검사를, 완성본은 위 렌더러의 폰트 해시·리소스 허용 목록·
네트워크 차단·실제 로딩 검사를 사용합니다.

## 폰트 출처

[Pretendard v1.3.9](https://github.com/orioncactus/pretendard/tree/v1.3.9)의 원본
`PretendardVariable.woff2`를 수정 없이 내장합니다. [원문 라이선스](https://github.com/orioncactus/pretendard/blob/v1.3.9/LICENSE)는
SIL Open Font License 1.1이며 저작권 고지와 전문은 [OFL.txt](OFL.txt)에 보존합니다.
폰트 저작자가 이 프로젝트를 보증한다는 뜻은 아닙니다.

```text
SHA-256
9599f12fd42fc0bce1cd50b47a0c022e108d7aa64dd0d1bb0ed44f3282d900b4
```

렌더러는 위 해시와 다르면 중단합니다. 앱의 실행·배포 의존성에는 이 폰트나
문서 렌더러를 추가하지 않습니다.
