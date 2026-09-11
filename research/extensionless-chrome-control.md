# macOS 일반 Chrome의 확장 없는 제어

조사일: 2026-09-07. 최초 범위는 공식 Chrome/Chromium·Apple 자료 조사다.
후속 로컬 제어·권한 진단은 아래에 별도로 기록한다. 이후 실제 두 검색 성공은
[최소화 검색 검증 기록](../intent/minimized-search.md)의 Apple Events 실측 절을 참조한다.

## 확인된 API와 권한

- Chromium의 macOS scripting dictionary에는 탭의 쓰기 가능한 `URL`, 읽기 전용
  `loading`, 탭 대상 `execute … javascript …`, 창의 `minimized` 속성이 있다.
  따라서 일반 Chrome의 AppleScript/Apple Events 제어는 실제로 존재하는 경로다.
  이 API 정의는 CDP나 확장 API가 아니다.
  [Chromium scripting.sdef](https://chromium.googlesource.com/chromium/src.git/+/lkgr/chrome/browser/ui/cocoa/applescript/scripting.sdef)
- Chrome 공식 안내는 AppleScript를 통한 JavaScript 주입을 기본 차단하며 사용자가
  `View > Developer > Allow JavaScript from Apple Events`에서 허용할 수 있다고
  설명한다. 확장과 Native Messaging은 같은 문서에서 대안으로 제시된다.
  따라서 **확장이 반드시 필요하다는 근거는 없다**. 이 설정은 JavaScript 실행에
  관한 것이며, URL 변경 자체에 같은 설정이 필요하다고 문서가 말하지는 않는다.
  안내의 “will soon” 문구는 오래된 표현이므로 현재 버전의 기본값을 별도로 실측한
  것으로 취급하지 않는다.
  [Chromium의 Mac 타사 앱 안내](https://www.chromium.org/developers/applescript/)
- macOS는 앱이 다른 앱을 제어할 때 사용자의 Automation 허용을 요구한다.
  사용자는 허용 대화상자와 `System Settings > Privacy & Security > Automation`에서
  권한을 관리한다. Chrome 내부 JavaScript 허용과 OS의 앱 간 제어 권한은 각각
  확인해야 한다. 실제 CLI 실행 호스트의 권한 상태는 이번 조사에서 확인하지 않았다.
  [Apple: Allow apps to automate and control other apps](https://support.apple.com/en-gb/guide/mac-help/mchl108e1718/mac)

## 전용 Chrome 격리

Chromium은 macOS에서 Chrome 실행 파일에 별도 `--user-data-dir`를 전달하는 방식을
공식 문서화한다. 이 디렉터리는 쿠키·방문 기록·북마크와 로컬 상태를 포함하며,
일반 프로필은 그 아래의 하위 디렉터리다. 따라서 일상용 Chrome과 별도 데이터
디렉터리를 사용하는 전용 인스턴스가 적절한 후보이고, 단순히 창만 하나 더 만드는
것과는 다르다. [Chromium: User Data Directory](https://chromium.googlesource.com/chromium/src/+/main/docs/user_data_dir.md)

설계상 주의점(추론): 별도 저장 경로만으로 Apple Events의 대상 프로세스가 자동으로
구분된다고 가정하면 안 된다. 같은 Google Chrome 앱의 인스턴스가 함께 실행될 때
전용 프로세스·창·탭을 정확히 식별하고, 식별 실패 시 일상용 브라우저로 대체하지 않는
검증이 필요하다. 별도 데이터 디렉터리는 저장 상태를 분리하는 수단이며 OS 차원의
별도 보안 경계를 입증하지 않는다. 기존 프로필이나 쿠키를 복사할 필요도 없다.

## 쿠팡 및 최소화 성공과의 경계

API가 있다는 것은 최소화 유지, 쿠팡 차단 회피, 실제 상품·가격 수집이 성공한다는
증거가 아니다. 이 경로에서 서로 다른 검색어 두 개의 실제 상품 응답, 실행 중과
전후의 최소화 상태, 새 창·포커스 이동 부재를 별도로 검증해야 한다. 구조화 상품
자료를 우선하고, 차단·로그인 요구·필드 누락을 그대로 보고해야 한다.

기존 [구현·검증 기록](../intent/minimized-search.md)은 **확장 + Native Messaging**
경로에서 두 번의 실제 검색과 최소화 상태 관측에 성공했다고 기록한다. 그 결과를
Apple Events 경로의 성공 증거로 전용해서는 안 된다. 최초 조사 시점에는 후보 API만
확인했으며, 이후 별도의 실제 Apple Events 두 검색을 검증했다(위 연결 기록 참조).

## 2026-09-07 후속 로컬 진단

사용자의 진행 요청으로 새 빈 데이터 디렉터리에 일반 Chrome을 별도로 실행했다.
기존 프로필·쿠키는 복사하거나 변경하지 않았고, 새 프로필에 확장이나 원격 디버깅을
추가하지 않았다. 이번 최초 설정용 창은 열려 있으며 검색 전 한 번 최소화할 예정이다.

- 기존 Chrome: 창 1개, 탭 6개, 최소화 상태. 전용 Chrome: 별도 PID, 다른 창 ID,
  탭 1개(`about:blank`). 개인정보가 있는 URL/제목은 출력하지 않았다.
- `Application(pid)` JXA 조회는 새 PID를 넘겨도 기존 Chrome의 창 ID/탭 수를
  반환했다. 조회만 실행했고 이 방식으로 URL이나 창 상태를 변경하지 않았다.
- `SBApplication.applicationWithProcessIdentifier(pid)`와 KVC 조회는 전용 Chrome의
  다른 창 ID/탭 1개를 반환했다. 이 경로를 진단 도구에 사용한다.
- 기존 Chrome PID를 진단 도구에 전달하는 음성 검증은 `target_identity_mismatch`로
  제어 전에 거절됐다. 프로세스 명령의 정확한 전용 데이터 폴더를 확인하며 CDP/headless
  실행과 창·탭 식별 변경을 거절한다.
- 전용 빈 탭에서 상수만 계산하는 JavaScript probe가 Apple Events 오류 12를
  반환했다. Chromium 소스의 `kJavaScriptUnsupported` 값과 일치한다. 이는 사이트에
  도달하기 전 Chrome의 JavaScript 실행 권한 문제이지 쿠팡 차단 관측이 아니다.
  [오류 정의](https://raw.githubusercontent.com/chromium/chromium/main/chrome/browser/ui/cocoa/applescript/error_applescript.h),
  [프로필별 실행 권한 검사](https://raw.githubusercontent.com/chromium/chromium/main/chrome/browser/ui/cocoa/applescript/tab_applescript.mm).
- 사용자에게 **전용 빈 Chrome 창**에서 JavaScript from Apple Events 허용을 요청했다.
  권한은 자동으로 켜지 않았으며 새 경로의 상품 검색은 아직 실행하지 않았다.
- `node --test tests/apple-events-probe.test.js`: 합성 테스트 4개 통과.
  프로세스/프로필 거절, URL 인코딩, 실제 상품 필드 검증, 활성화 없는 전송 경로를 확인한다.
  최소화 연속 검색 성공을 대신하는 테스트가 아니다.

진단 도구는 설치된 확장을 사용하지 않고 기존 `extension/search-reader.js`의 순수
DTO 추출 함수를 소스 코드로 재사용한다. 이름·상품 ID·정규화 URL·관측 가격만 반환한다.
프로덕션 경로로 채택할 때는 reader를 공용 어댑터 자산으로 분리할 수 있다.

실행 순서(인수는 새 프로세스와 프로필의 실제 값을 사용):

```text
node research/probes/chrome-apple-events.mjs status PID /tmp/coupangctl-apple-events.SUFFIX
node research/probes/chrome-apple-events.mjs permission PID /tmp/coupangctl-apple-events.SUFFIX
node research/probes/chrome-apple-events.mjs minimize PID /tmp/coupangctl-apple-events.SUFFIX
node research/probes/chrome-apple-events.mjs search PID /tmp/coupangctl-apple-events.SUFFIX '미니 식기'
node research/probes/chrome-apple-events.mjs search PID /tmp/coupangctl-apple-events.SUFFIX '스텐 식기'
```

`search`는 이미 최소화된 창만 허용하고 복원/활성화/최소화 명령을 호출하지 않는다.
각 요청에서 동일 창·탭과 최소화 상태를 표본 관측한다. 연속 이벤트 추적은 아니며,
권한 허용 후 실제 두 검색과 기존 Chrome 불변 상태를 확인해야 검증 완료다.

## 최초 설정 자동화 요청 이후: 자동 클릭이 적용되지 않는 원인

사용자가 초기 준비까지 자동화하도록 요청했다. 전용 창 재개방에 대한 추가 동의는
필요하지 않다. 해당 전용 데이터 폴더의 기존 프로세스에 빈 창 하나를 열었다.
메뉴 접근성 클릭과 합성 마우스 입력은 성공을 반환했지만, 상수 JavaScript probe는
계속 `javascript_permission_required`를 반환했다. 마지막 상태 조회는 같은 전용
창·탭 하나와 `minimized=true`를 확인했다. 쿠키·일상용 프로필은 변경하지 않았다.

Chromium `ToggleJavaScriptFromAppleEventsAllowed`는 다른 프로세스에서 생성한
이벤트와 HID 시스템 기원이 아닌 이벤트를 거절한 뒤에만 프로필 설정을 바꾼다.
이는 관측된 클릭 무효화와 일치하는 소스 근거다. 설치 버전의 소스와 일대일 대조한
결과는 아니지만, 클릭 API 성공만으로 허용을 완료 처리해서는 안 된다.
[Chromium 구현](https://chromium.googlesource.com/chromium/src/+/main/chrome/browser/ui/browser_commands_mac.mm)

후속 작업은 전용 창에서 사용자가 `보기 > 개발자 정보 > Apple Events의 자바스크립트 허용`을
직접 한 번 선택한 뒤 상수 probe로 검증한다. 반복 자동 클릭이나 이벤트 출처 위장,
권한 설정 파일 변경으로 이 확인을 대체하지 않는다. 일반 준비 작업의 승인을 재요청하지
않으며, 권한 확인 이후 최소화·연속 두 검색·상품 DTO 검증을 자동으로 수행한다.
프로필별 최초 허용과 로그인은 매 검색의 연결 버튼을 요구하는 설계와 구분한다.

후속 상태: 사용자의 직접 허용 이후 상수 probe가 통과했다. native 탭 URL setter는
이 환경에서 최소화 창을 복원시켰다. Go 전송과 진단 전송 모두 허용된 탭 내부의
`window.location.assign`으로 변경했다. Chrome의 문자열 ID도 Go 전송에서 숫자로
검증·정규화했다. 그 뒤 Go CLI 실제 두 검색 및 최소화 표본 관측이 성공했으며,
세부 결과의 단일 기록은 위 최소화 검색 문서다. 최초 권한 요청을 다시 반복하지 않는다.
