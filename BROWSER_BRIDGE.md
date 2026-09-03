# 일반 Chrome 브리지 계약

일반 Chrome 브리지는 사용자가 이미 로그인한 주문목록 탭을 한 번 선택해
로컬 원장을 동기화하는 실험적 경로입니다. 쿠키·브라우저 저장소·원본 응답을
복사하지 않으며 Orca, Playwright, 별도 AI 브라우저를 런타임 의존성으로
사용하지 않습니다.

## 설치와 진단

```bash
coupangctl browser-bridge install
coupangctl browser-bridge doctor
```

`install`은 실행 중인 바이너리 안에 포함된 검토된 MV3 번들을 사용자 로컬
상태 경로에 풀고, 현재 사용자 범위에 Native Messaging 호스트를 등록합니다.
macOS와 Linux는 Chrome의 사용자 NativeMessagingHosts 디렉터리를 사용하고,
Windows는 상태 디렉터리의 매니페스트를 HKCU Chrome 키에 연결합니다.

설치 응답은 다음 형태입니다. 아래 경로는 예시이며 실제 응답은 현재 시스템의
절대경로를 반환합니다.

```json
{
  "schema_version": 1,
  "status": "installed",
  "browser": "chrome",
  "extension_id": "kdpkegejlalobnlbgpjjibllolajjonf",
  "extension_path": "/absolute/local/state/ordinary-browser-bridge/extension",
  "native_host_name": "com.coupangctl.browser_bridge",
  "native_host_manifest_path": "/absolute/user/path/com.coupangctl.browser_bridge.json",
  "installation_record_path": "/absolute/local/state/ordinary-browser-bridge/installation.json",
  "next_action": "load_unpacked_extension"
}
```

Chrome Web Store 배포 전에는 `extension_path`를 `chrome://extensions`에서
압축해제된 확장으로 한 번 로드해야 합니다. 설치 명령은 Chrome 보안 정책을
우회해 확장을 몰래 설치하지 않습니다.

`doctor`는 실행 파일, 소유권 기록, 내장 번들과 설치된 번들의 일치, Native
Messaging 매니페스트의 정확한 origin·바이너리 경로, Windows의 HKCU 등록을
검사합니다. `status`는 `ready`, `not_installed`, `repair_required` 중 하나이고
모든 로컬 검사가 통과할 때만 `ready: true`입니다. Chrome 프로필을 읽어 확장
설치 여부를 추측하지 않으므로 `extension_load_status`는 `not_checked`,
`next_action`은 `load_or_verify_extension_in_chrome`입니다. 검사 메시지는 원본
파일 내용이나 브라우저 데이터를 출력하지 않습니다.

## 동기화

CLI:

```bash
coupangctl orders sync --max-pages 1 --ordinary-browser
```

MCP 도구는 `orders_sync_ordinary_browser`이며 입력과 응답은 기존
`orders_sync`와 같은 `SyncRequest`/`SyncResult` typed core 계약을 사용합니다.
호출하기 전에 AI는 사용자가 로그인된 쿠팡 주문목록 탭을 선택하고 대기 중에
확장 버튼을 한 번 누르도록 안내해야 합니다.

확장은 `activeTab`, `nativeMessaging`, `scripting`만 요청합니다. 클릭한 정확한
top frame에서 패키지 안의 isolated-world reader를 한 번 실행하고, 최대 다섯
주문의 정규화된 페이지를 로컬 네이티브 호스트로 전달합니다. 브리지는 2분짜리
단일 사용 인증 rendezvous를 사용하며 URL, 임의 명령, 쿠키, 원본 문서를 받지
않습니다.

## 제거

```bash
coupangctl browser-bridge uninstall
```

제거 전에는 설치 기록, 네이티브 매니페스트·등록, 네 개의 내장 번들 파일이
현재 바이너리가 기대하는 내용과 모두 일치해야 합니다. 하나라도 바뀌면 제거를
거부합니다. 성공 시 `native_host_registration`, `extension_bundle`,
`installation_record`만 제거하며 Chrome 프로필, 쿠키, Chrome이 관리하는 확장
데이터, `coupangctl.sqlite3`는 삭제하지 않습니다.

## 현재 검증 상태

합성 계약 테스트는 설치 충돌의 사전 차단, Unix 비공개 파일 권한, 정확한 확장
origin, 변조 진단, 소유권 기반 제거, MCP typed provider 분리를 검증합니다.
Linux, macOS, Windows 바이너리는 CGO 없이 교차 빌드합니다. 실제 macOS
관리형 호스트 설치는 doctor를 통과했고, 일반 Chrome에서
CLI→Native Messaging→typed core→SQLite 한 페이지 읽기가 네 번 연속
성공했습니다. 마지막 실행 전 Chrome 세부정보에서 관리형 `extension_path`가
실제 로드 위치임을 확인했습니다. 깨끗한 Chrome 프로필, Linux·Windows 실설치,
Chrome Web Store 심사는 아직 완료되지 않았으므로 상태는 `experimental`입니다.
