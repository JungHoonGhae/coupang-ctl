# 일반 Chrome 브리지 계약

일반 Chrome 브리지는 기본 설치 요구 사항이 아니라, 전용 프로필과 Chrome
144+의 사용자 승인 `--current-browser` 연결이 적합하지 않을 때 선택하는 호환
경로입니다. 사용자가 이미 로그인한 주문목록 탭을 한 번 선택해
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

소유권 기록 schema v3는 일곱 개 확장 파일과 네이티브 호스트 매니페스트의
SHA-256을 저장합니다. 이후 바이너리의 내장 번들이 바뀌면 현재 파일이 기록된
이전 digest와 정확히 일치할 때만 `upgrading` 전환 기록을 먼저 쓰고 파일을
추가·교체·제거합니다. 중간에 프로세스가 끝나도 다음 `install`이 기록된 이전·목표
파일 집합과 digest에 속하는 파일만 이어서 복구합니다. 기존 schema v2 digest
기록도 같은 검증을 거쳐 v3로 올립니다. 기록에 없는 변경이나 예상 밖 확장 파일은
덮어쓰지 않습니다. 바이너리 경로가 바뀐 정상 업데이트도 같은 계약으로
네이티브 호스트 매니페스트를 갱신합니다.

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

최초 설치와 동일 버전 재실행은 `status: "installed"`를 반환합니다. 검증된
이전 번들 또는 실행 경로를 갱신한 경우에는 `status: "upgraded"`와
`next_action: "reload_extension_in_chrome"`를 반환합니다. Web Store 배포 전
압축해제 확장은 Chrome에서 다시 로드해야 새 코드가 확실히 반영됩니다.

Chrome Web Store 배포 전에는 `extension_path`를 `chrome://extensions`에서
압축해제된 확장으로 한 번 로드해야 합니다. 설치 명령은 Chrome 보안 정책을
우회해 확장을 몰래 설치하지 않습니다.

`doctor`는 실행 파일, 소유권 기록, 내장 번들과 설치된 번들의 일치, Native
Messaging 매니페스트의 정확한 origin·바이너리 경로, Windows의 HKCU 등록을
검사한 뒤 합성 native-host ping을 수행합니다. ping은 비공개 일회성 rendezvous,
정확한 확장 origin, Native Messaging framing, 빈 typed 주문 페이지 왕복을 실제
코드 경로로 검증하고 즉시 정리하며 Chrome·쿠팡·프로필·쿠키·주문 데이터는 열지
않습니다. `status`는 `ready`, `not_installed`, `repair_required` 중 하나이고
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
성공·부분 실패 결과의 `source`는 `ordinary_browser_selected_tab`,
`provenance`는 `observed_source_native_structured_order_document`로 표시됩니다.
호출하기 전에 AI는 사용자가 로그인된 쿠팡 주문목록 탭을 선택하도록 안내해야
합니다. 사용자는 확장 팝업에서 읽을 필드와 로컬 전송 범위를 확인한 뒤
**이 탭 연결**을 눌러야 합니다. 툴바 아이콘만 누르거나 팝업을 닫으면 읽기를
시작하지 않습니다.

확장은 `activeTab`, `nativeMessaging`, `scripting`만 요청합니다. 클릭한 정확한
top frame에서 패키지 안의 isolated-world reader를 한 번 실행하고, 최대 다섯
주문의 정규화된 페이지를 로컬 네이티브 호스트로 전달합니다. 브리지는 2분짜리
단일 사용 인증 rendezvous를 사용하며 URL, 임의 명령, 쿠키, 원본 문서를 받지
않습니다.

## 제거

```bash
coupangctl browser-bridge uninstall
```

제거 전에는 활성 설치 기록, 네이티브 매니페스트·등록, 일곱 개의 소유 번들 파일이
현재 바이너리가 기대하는 내용과 모두 일치해야 합니다. 진행 중인 업그레이드나
기록되지 않은 변경이 있으면 먼저 `install`로 복구해야 하며 제거는 거부됩니다.
성공 시 `native_host_registration`, `extension_bundle`,
`installation_record`만 제거하며 Chrome 프로필, 쿠키, Chrome이 관리하는 확장
데이터, `coupangctl.sqlite3`는 삭제하지 않습니다.

## 현재 검증 상태

합성 계약 테스트는 설치 충돌의 사전 차단, Unix 비공개 파일 권한, 정확한 확장
origin, digest 기반 정상 업그레이드, 중단 복구, 실행 경로 이동, 기록되지 않은
변조와 예상 밖 파일 거부, 소유권 기반 제거, MCP typed provider 분리를 검증합니다.
Linux, macOS, Windows 바이너리는 CGO 없이 교차 빌드합니다. 실제 macOS
관리형 호스트 설치는 일곱 doctor 검사를 통과했고, 동의 팝업을 추가하기 전의
일반 Chrome에서
CLI→Native Messaging→typed core→SQLite 한 페이지 읽기가 네 번 연속
성공했습니다. 마지막 실행 전 Chrome 세부정보에서 관리형 `extension_path`가
실제 로드 위치임을 확인했습니다. Ubuntu CI는 사용자 파일 등록 계약을 실행하고,
Windows CI는 격리된 실제 HKCU 키에서 install→doctor/ping→uninstall 및 충돌
거부를 실행합니다. 실제 Chrome이 설치된 깨끗한 Linux·Windows 데스크톱 검증과
Chrome Web Store 심사는 아직 완료되지 않았으므로 상태는 `experimental`입니다.

배포 snapshot은 CGO를 끈 macOS·Linux·Windows의 amd64·arm64 아카이브 여섯
개를 생성했고, 파일 허용 목록·SHA-256·플랫폼 조합과 여섯 SBOM을
`releasecheck`로 검증했습니다. GitHub 태그 워크플로는 이 검증과 provenance
attestation이 모두 성공해야 draft를 공개합니다. macOS·Windows 네이티브 코드
서명은 별도 미완료 게이트이며 자세한 계약은 [`RELEASING.md`](RELEASING.md)에
있습니다.
