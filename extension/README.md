# 일반 Chrome 연결 확장

이 디렉터리는 `coupangctl orders sync --ordinary-browser`의 실험적
Manifest V3 확장입니다. 사용자가 선택한 정확한 쿠팡 주문목록 탭에서만
한 번 실행되며, 쿠키·브라우저 저장소·원본 응답을 내보내지 않습니다.

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

등록 후 명령을 먼저 실행하고, 이미 로그인된 일반 Chrome 주문목록 탭에서
확장 버튼을 한 번 누릅니다.

```bash
coupangctl orders sync --max-pages 1 --ordinary-browser
```

확장은 `activeTab`, `nativeMessaging`, `scripting`만 요청합니다. 광범위한
host permission, 쿠키 권한, 외부 메시지, incognito, telemetry는 사용하지
않습니다. 프로토콜과 위협 모델은
[`research/ordinary-browser-bridge.md`](../research/ordinary-browser-bridge.md)에
정리되어 있습니다.
