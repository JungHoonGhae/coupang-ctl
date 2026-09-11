# 일반 Chrome 연결 확장

> 과거 구현 기록입니다. 현재 개발 소스는 Camofox만 사용하며 이 폴더의 확장 코드와
> ZIP 생성기는 제거했습니다. 아래 설치·연결·테스트 명령은 더 이상 실행할 수 없습니다.
> 현재 설정은 [README](../README.md#camofox-기본-연결-개발-소스)를 참고하세요.

## 최소화 상태의 상품 검색 (실험적)

`coupangctl products search --query '미니 식기' --limit 3 --no-affiliate --ordinary-browser`
명령을 실행하고, 일반 Chrome의 쿠팡 검색 탭에서 확장 팝업의 **이 검색 탭 연결**을
한 번 누릅니다. 이후 Chrome을 최소화해도 같은 탭에서 연속 CLI 검색을 처리합니다.
검색 코드는 창 상태·포커스를 변경하거나 새 창·탭을 만들지 않습니다. 원격 디버깅,
쿠키 읽기, 지문 변경은 사용하지 않습니다. 엄밀한 headless가 아닌 일반 Chrome
확장 경로이며, 사이트의 접근 허용과 백그라운드 탭 실행 상태에 영향을 받습니다.

연결은 최대 30분 유지합니다. 탭을 닫거나 다른 origin으로 이동하거나 Chrome을
종료하면 다시 연결해야 합니다. 연결된 탭의 검색어는 CLI 요청에 따라 바뀌므로
검색용 탭 하나를 사용하세요. 읽는 값은 상품 ID·상품명·관찰 가능한 가격·상품 URL과
결과 순서이며, 후기·배송 배지는 현재 이 경로에서 제공하지 않습니다. category-id,
상품 상세 조회, 장바구니 변경은 지원하지 않습니다. 접근 거부는 실패로 반환합니다.

합성 검증: `node --test tests/search-extension.test.js`. 실제 최소화 상태의
쿠팡 성공 여부는 별도 live 검증이 필요합니다.

이 디렉터리는 `coupangctl orders sync --ordinary-browser`의 실험적
Manifest V3 확장입니다. 사용자가 선택한 정확한 쿠팡 주문목록 탭에서만
한 번 실행되며, 쿠키·브라우저 저장소·원본 응답을 내보내지 않습니다.
전용 프로필로 한 번 로그인한 뒤 headless로 재사용하는 기본 흐름에는 이 확장이
필요하지 않습니다.

## 연결하기

1. `go build -o ./coupangctl ./cmd/coupangctl`로 바이너리를 만듭니다.
2. `./coupangctl browser-bridge install`을 실행합니다.
3. JSON 응답의 `extension_path`를 Chrome `chrome://extensions`에서
   압축해제된 확장으로 한 번 로드합니다.
4. `./coupangctl browser-bridge doctor`의 `ready`가 `true`인지 확인합니다.

설치 명령은 Chrome Native Messaging 호스트
`com.coupangctl.browser_bridge`를 사용자 범위에 등록합니다. 매니페스트의
`path`는 실행한 바이너리의 절대경로이고 허용 origin은 아래 하나뿐입니다.

```json
{
  "name": "com.coupangctl.browser_bridge",
  "description": "Local read-only ordinary-browser bridge for coupangctl",
  "path": "/absolute/path/to/coupangctl",
  "type": "stdio",
  "allowed_origins": [
    "chrome-extension://kdpkegejlalobnlbgpjjibllolajjonf/"
  ]
}
```

사용자 범위 등록 위치는 macOS에서
`~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`,
Linux에서 `$XDG_CONFIG_HOME/google-chrome/NativeMessagingHosts/`이며,
Windows에서는 로컬 상태 디렉터리의 매니페스트와 HKCU 레지스트리 값을
함께 사용합니다. 제거는 `./coupangctl browser-bridge uninstall`이며 동일
설치의 비공개 소유권 기록과 내용이 일치하지 않으면 아무 파일도 지우지 않습니다.
새 바이너리의 번들 또는 실행 경로가 달라진 경우 `install`은 기록된 SHA-256과
현재 파일이 일치할 때만 중단 복구 가능한 업그레이드를 수행합니다. 응답이
`status: "upgraded"`이면 `chrome://extensions`에서 이 확장을 다시 로드한 뒤
동기화합니다.

등록 후 명령을 먼저 실행하고, 이미 로그인된 일반 Chrome 주문목록 탭에서
확장 팝업을 연 뒤 데이터 범위를 확인하고 **이 탭 연결**을 누릅니다. 팝업을
닫으면 읽기를 시작하지 않습니다.

```bash
coupangctl orders sync --max-pages 1 --ordinary-browser
```

확장은 `activeTab`, `nativeMessaging`, `scripting`만 요청합니다. 광범위한
host permission, 쿠키 권한, 외부 메시지, incognito, telemetry는 사용하지
않습니다. 개인정보 처리 범위는 [`PRIVACY.md`](../PRIVACY.md), Web Store
제출 답안과 검증 게이트는 [`STORE_LISTING.md`](STORE_LISTING.md), 프로토콜과 위협 모델은
[`research/ordinary-browser-bridge.md`](../research/ordinary-browser-bridge.md)에
정리되어 있습니다.

## 제출 ZIP

유지관리자는 아래 명령으로 새 경로에 Chrome Web Store 제출 ZIP을 만들고 다시
검증합니다. 일반 사용자의 설치 단계가 아닙니다.

```bash
go run ./cmd/extensionpack --output /new/path/coupangctl-extension.zip
go run ./cmd/extensionpack --verify /new/path/coupangctl-extension.zip
```

생성기는 이 디렉터리의 내장 허용 목록만 ZIP 루트에 넣고 구조화된 파일 목록,
크기, SHA-256을 출력합니다. 기존 ZIP을 덮어쓰지 않으며 누락·추가·중첩·중복·변조
파일은 검증에서 거부합니다.
