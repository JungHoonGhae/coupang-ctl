# `coupangctl` 네이티브 배포 서명 로드맵

조사일: 2026-09-03

## 결정

`coupangctl`의 stable 릴리스는 다음 네 실행 파일이 운영체제의 네이티브 신뢰
검증을 모두 통과한 뒤에만 열어야 한다.

- macOS amd64·arm64: Developer ID Application 서명, secure timestamp, hardened
  runtime, Apple notarization `Accepted`, 예상한 서명 주체와 코드 식별자 검증
- Windows amd64·arm64: 공개 신뢰 Authenticode 서명, SHA-256, RFC 3161 timestamp,
  예상한 publisher와 인증서 체인 검증

가장 현실적인 Windows 기본 경로는 **Azure Artifact Signing(이전 명칭 Trusted
Signing)의 Public Trust 프로필 + GitHub OIDC**다. Microsoft는 Artifact Signing을
관리형 end-to-end 서명 서비스로 설명하며, 키 수명주기를 FIPS 140-3 Level 3 HSM
안에서 관리하고 실제 파일 대신 digest를 서명한다.
([Artifact Signing 개요](https://learn.microsoft.com/en-us/azure/artifact-signing/overview))
현재 공식 quickstart에는 대한민국 소재 **조직**도 Public Trust 대상에 포함되어
있다. 다만 대한민국 소재 개인 개발자는 대상이 아니므로, 개인사업자를 포함한
법적 사업체는 Azure 결제 계정 유형과 Organization/DBA 검증 경로가 실제 법적
등록 정보에 맞는지 먼저 확인해야 한다.
([Artifact Signing quickstart](https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart))

macOS는 Apple의 `codesign`과 `notarytool`을 macOS GitHub-hosted runner에서 직접
사용하는 경로를 기준 구현으로 삼는다. 이는 독립 Go CLI에도 적용 가능한 Apple의
custom build workflow이며, 서명·공증 결과를 저장소의 typed verifier가 별도로
검사하기 쉽다.
([distribution-signed code](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac),
[custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow))

현재 `tar.gz` 파일명 계약은 유지할 수 있지만 중요한 제한이 있다. Apple은 ZIP을
notarize할 수 있어도 ZIP 자체에는 ticket을 staple할 수 없고, standalone binary의
ticket도 생성하지만 그 바이너리에 직접 staple할 수는 없다고 명시한다. 따라서
현재 macOS `tar.gz`는 온라인 Gatekeeper ticket 조회를 전제로 한 배포물이다.
오프라인 최초 실행까지 보장해야 한다면 stapling 가능한 서명·공증된 DMG 또는 flat
installer PKG를 **별도 사용자용 산출물**로 추가해야 한다.
([custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow),
[Mac 배포 패키징](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution))

GitHub artifact attestation은 그대로 유지하되 항상 네이티브 서명과 최종 패키징
뒤에 생성한다. GitHub도 attestation이 artifact의 안전성을 보증하지 않고 source와
build instructions를 연결한다고 명시한다. Gatekeeper와 Windows가 신뢰하는
Developer ID/Authenticode publisher identity를 대신하지 않는다.
([GitHub artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations))

## 현재 저장소와 목표 파이프라인의 차이

현재 workflow는 Ubuntu에서 여섯 실행 파일을 한 번에 build·archive하고, SBOM과
checksum을 만든 뒤 attestation하고 draft를 공개한다. 이 순서에서는 운영체제별
서명이 archive보다 앞에 들어갈 자리가 없다. 목표 순서는 다음과 같다.

1. 고정된 tag source에서 여섯 raw executable을 만든다.
2. macOS signing job이 두 Mach-O를 서명하고 notarize한 뒤 자체 검증한다.
3. Windows signing job이 두 PE 파일을 Authenticode 서명하고 자체 검증한다.
4. Linux 두 실행 파일은 변경 없이 다음 단계로 보낸다.
5. **검증이 끝난 이 실행 파일들만** canonical `tar.gz`/ZIP으로 패키징한다.
6. 최종 archive로 SBOM과 `checksums.txt`를 만들고 기존 `releasecheck`를 실행한다.
7. 최종 archive와 checksum manifest에 GitHub attestation을 생성한다.
8. 모든 서명 증거가 같은 artifact digest에 묶였을 때만 draft를 공개한다.

코드 서명은 실행 파일 bytes를 바꾸므로 `archive → sign` 또는 `checksum → sign`은
잘못된 순서다. Apple도 code를 먼저 서명하고 distribution container를 만든 뒤
container를 notarize하도록 안내하며, Windows SignTool도 서명 대상 파일에 embedded
signature를 기록한다.
([Apple 패키징 순서](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution),
[Microsoft SignTool](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool))

GoReleaser 호출 방식은 orchestration detail로 두고, typed core에는 다음 두 좁은
adapter만 추가하는 편이 안전하다.

- `darwin-signing-verifier`: 코드 서명·hardened runtime·timestamp·서명 주체·코드
  식별자·notarization 결과를 구조화한다.
- `windows-signing-verifier`: Authenticode policy·timestamp·publisher·certificate
  chain 결과를 구조화한다.

stable 정책은 환경 변수나 사람이 넘긴 `signed=true`를 받지 않아야 한다. verifier가
현재 실행 파일을 직접 검사해 만든 evidence manifest와 그 실행 파일의 SHA-256이
일치할 때만 통과시킨다. prerelease의 기존 unsigned 표시와 stable fail-closed 정책은
이 실제 adapter가 완성될 때까지 유지한다.

## macOS: Developer ID와 notarization

### 외부 준비물

Apple Developer Program의 Account Holder가 외부 배포용 Developer ID Application
certificate를 발급해야 한다. Developer ID Installer certificate는 PKG 자체를
서명할 때만 별도로 필요하다.
([Developer ID certificates](https://developer.apple.com/help/account/certificates/create-developer-id-certificates))

GitHub-hosted macOS runner에서는 Developer ID certificate와 private key를 PKCS#12로
전달해 job 전용 임시 keychain에 import한다. GitHub의 공식 절차도 certificate를
보호된 secret으로 저장하고 임시 keychain에 import하며, hosted runner는 job 종료
후 폐기된다고 설명한다. plain CLI에는 restricted entitlement가 없는 한 provisioning
profile이 필요하지 않으므로 불필요한 profile을 추가하지 않는다.
([GitHub의 Apple certificate 설치](https://docs.github.com/en/actions/how-tos/deploy/deploy-to-third-party-platforms/sign-xcode-applications),
[Apple의 entitlement와 profile 규칙](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac))

notarization 인증은 App Store Connect **team API key**를 권장한다. `notarytool`은
app-specific password 또는 App Store Connect API key를 지원하지만, Apple은
individual API key가 `notaryTool`을 사용할 수 없다고 명시한다. private key는
저장소, 로그 또는 release artifact에 넣지 않고 보호된 GitHub environment에서만
job에 노출한다.
([notarytool credential migration](https://developer.apple.com/documentation/technotes/tn3147-migrating-to-the-latest-notarization-tool),
[App Store Connect API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api))

필요한 비밀 값의 이름은 이 문서에서 새로 정의하지 않는다. 구현 시 기존 배포
문서와 Doppler/GitHub environment의 단일 계약에서만 확정하고, 출력·fixture·artifact에
값을 기록하지 않는다.

### 서명

독립 실행 파일의 명령 형태는 다음과 같다. angle bracket은 값이 아니라 구현 시
주입할 자리 표시자다.

```sh
codesign --force --timestamp --options runtime \
  --identifier "<stable-code-identifier>" \
  --sign "<Developer-ID-Application-identity>" \
  ./coupangctl
```

Apple은 외부 배포에 Developer ID Application identity를 사용하고, Developer ID
서명에는 `--timestamp`, main executable에는 `-o runtime`, nonbundled code에는
고정 code-signing identifier를 주도록 명시한다. `codesign`을 `sudo`로 실행하거나
서명에 `--deep`을 쓰지 않는다.
([distribution-signed code](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac))

`coupangctl`은 JIT, unsigned executable memory, 외부 plug-in host가 아니므로 첫
구현은 entitlement 없이 시작한다. Hardened Runtime은 보호 기능을 기본으로 켜며,
예외 entitlement는 필요한 기능 하나만 완화하므로 실제 실행 실패가 증명되기 전에는
추가하지 않는다.
([Hardened Runtime](https://developer.apple.com/documentation/security/hardened-runtime))

서명 직후 최소한 다음 검사를 통과시킨다.

```sh
codesign --verify --strict --verbose=2 ./coupangctl
codesign --display --verbose=4 ./coupangctl
```

첫 명령의 성공만으로 충분하지 않다. 두 번째 결과에서 typed verifier가 다음을
allowlist와 비교해야 한다.

- Developer ID Application chain
- expected Team ID와 publisher identity
- stable code identifier
- secure timestamp 존재
- hardened runtime 표시
- 금지된 `get-task-allow` 및 예상하지 않은 entitlement 부재

Apple은 notarization 전에 모든 executable의 유효한 서명, Developer ID, hardened
runtime, secure timestamp가 필요하고 `get-task-allow=true`를 허용하지 않는다.
([notarization requirements](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution),
[common notarization issues](https://developer.apple.com/documentation/security/resolving-common-notarization-issues))

### 제출과 판정

Apple notary service는 `tar.gz`를 받지 않고 ZIP, UDIF disk image, signed flat
installer package를 받는다. 따라서 각 아키텍처의 **서명된 바로 그 실행 파일**로
임시 ZIP을 만들고 제출한다.

```sh
ditto -c -k --keepParent ./coupangctl ./notary-submission.zip
xcrun notarytool submit ./notary-submission.zip \
  --keychain-profile "<temporary-profile>" \
  --wait --output-format json
xcrun notarytool log "<submission-id>" \
  --keychain-profile "<temporary-profile>" \
  ./notary-log.json
```

`altool`은 2023-11-01부터 더 이상 notarization upload를 받지 않으므로 사용하지
않는다. `--wait` 결과가 `Accepted`여야 하며, 성공해도 log에 수정할 warning이 있을
수 있으므로 항상 log를 확인한다. workflow timeout이나 일시 장애가 나면 같은
submission ID를 조회하고, 곧바로 중복 제출하지 않는다.
([custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow))

raw notary log 전체를 공개 release asset으로 올리지 않는다. adapter는 status,
submission ID, warning/error 개수, 제출한 signed binary digest만 포함한 비밀 없는
요약 JSON을 만든다. ZIP 제출 전후 raw binary SHA-256이 같고, 최종 `tar.gz`에서 다시
꺼낸 실행 파일의 SHA-256도 같아야 한다.

### Stapling 경계

현재 형식에서는 다음 상태를 명시적으로 구분한다.

| 배포 형식 | Notarize | Staple | 최초 오프라인 Gatekeeper |
| --- | --- | --- | --- |
| signed binary를 넣은 `tar.gz` | 임시 ZIP 제출로 가능 | standalone binary와 ZIP 모두 불가 | 보장하지 않음 |
| signed `.app`을 넣은 ZIP | ZIP 제출 가능 | `.app`에 staple한 뒤 ZIP 재생성 | 가능 |
| signed DMG | 가능 | DMG에 가능 | 가능 |
| signed flat PKG | 가능 | PKG에 가능 | 가능 |

Apple은 ticket을 온라인에도 게시하므로 standalone binary 사용자는 네트워크가 있으면
Gatekeeper가 ticket을 찾을 수 있다. 반대로 stapling하지 않은 배포물은 사용자가
오프라인일 때 차단될 수 있다.
([custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow),
[Mac 배포 패키징](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution))

따라서 첫 stable에서 기존 `tar.gz`를 유지한다면 release note와 capability에는
`notarized_online_ticket`처럼 정확한 상태를 써야 하며 “stapled”라고 표현하면 안
된다. 나중에 오프라인 설치가 제품 요구가 되면 기존 `tar.gz`를 깨지 않고 DMG 또는
PKG를 추가하고 다음을 수행한다.

```sh
xcrun stapler staple "<distribution-container>"
xcrun stapler validate "<distribution-container>"
```

### 최종 사용자 경로 검증

CI의 `codesign` 성공만으로 Gatekeeper 사용성을 증명하지 않는다. Apple은 실제 배포
형식으로 패키징하고, 웹 다운로드·메일·AirDrop처럼 quarantine이 붙는 경로로 받은
복사본을 별도 Mac에서 테스트하라고 안내한다.
([TN2206 Gatekeeper conformance](https://developer.apple.com/library/archive/technotes/tn2206/),
[Mac 배포물 테스트](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution))

자동 검사는 다음을 함께 사용한다.

```sh
codesign -vvvv -R="notarized" --check-notarization ./coupangctl
```

그 뒤 실제 GitHub Release에서 브라우저로 내려받은 exact archive를 깨끗한 Intel
Mac과 Apple Silicon Mac에서 Finder/Archive Utility로 풀어 quarantine이 실행 파일에
전파된 상태로 `coupangctl version`과 `doctor`를 실행한다. Unix `tar`와 `curl`은
quarantine을 설정·전파하지 않으므로 이 acceptance test의 다운로드/해제 경로로
쓰지 않는다.
quarantine을 지우는 `xattr` 우회는 테스트와 사용자 안내 어디에도 넣지 않는다.
Apple DTS의 현재 지침은 standalone code에는 위 `codesign --check-notarization`을
사용하되, 실제 사용 경로를 재현한 fresh quarantined-copy 검사가 가장 강한 최종
증거라고 설명한다.
([Testing a Notarised Product](https://developer.apple.com/forums/thread/130560),
[Resolving Trusted Execution Problems](https://developer.apple.com/forums/thread/706442))

## Windows: Authenticode

### 권장 경로 — Artifact Signing Public Trust

Public release에는 `Public Trust` certificate profile만 사용한다. `Public Trust Test`는
기본 공개 신뢰가 없고 inner-loop test용이며, `Private Trust`도 Windows의 public root
program에 기본 신뢰되지 않는다.
([Artifact Signing trust models](https://learn.microsoft.com/en-us/azure/artifact-signing/concept-trust-models),
[Artifact Signing resources and roles](https://learn.microsoft.com/en-us/azure/artifact-signing/concept-resources-roles))

외부 onboarding 순서는 다음과 같다.

1. paid Azure subscription과 Microsoft Entra tenant를 준비한다.
2. `Microsoft.CodeSigning` resource provider를 등록한다.
3. Artifact Signing account를 만들고 Organization/DBA Public identity validation을
   완료한다.
4. Public Trust certificate profile을 만든다.
5. GitHub release workflow용 Entra identity와 federated credential을 만든다.
6. 그 identity에 certificate profile 범위의 `Artifact Signing Certificate Profile
   Signer` 역할만 부여한다.

Identity validation은 portal에서만 가능하며, legal name·주소·사업 식별 정보와 Azure
billing account 유형이 일치해야 한다. 공식 문서는 처리에 1~20영업일 이상 걸릴 수
있다고 안내하므로 CI 구현보다 먼저 신청해야 한다.
([Artifact Signing quickstart](https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart),
[Artifact Signing roles](https://learn.microsoft.com/en-us/azure/artifact-signing/tutorial-assign-roles))

GitHub Actions 인증은 long-lived client secret 대신 OIDC를 사용한다. GitHub는 OIDC가
장기 Azure credential을 GitHub secret으로 저장하지 않게 하며, environment를 쓰는
경우 branch/tag 제한과 protection rules를 권장한다.
([GitHub OIDC for Azure](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/configuring-openid-connect-in-azure))

release signing job은 protected tag와 보호된 GitHub environment에서만 실행하고,
`id-token: write`와 `contents: read`만 먼저 부여한다. federated credential의 subject도
그 environment 또는 release tag 경로로 제한한다. 서명과 publish 권한을 같은
identity에 합치지 않는다.

공식 `Azure/artifact-signing-action`은 현재 Windows 2022/2025 hosted runner를
지원하고 Windows Arm runner는 지원하지 않는다. 따라서 action 자체는 지원되는 x64
Windows runner에서 실행한다. amd64·arm64 EXE 두 개를 그 job에서 모두 서명하는
방식은 구현 우선안이지만, action 문서가 cross-architecture signing을 명시적으로
보장하지는 않는다. 실제 두 파일의 Authenticode 검증이 통과해야만 이 구현 추론을
채택하고, ARM64 실행 가능성은 별도의 실제 ARM64 Windows 환경에서 검증한다.
([Artifact Signing action](https://github.com/Azure/artifact-signing-action))

action 앞에는 `azure/login`의 OIDC 교환을 두고, action이 Azure CLI credential만
선택하도록 인증 provider를 제한한다. Microsoft와 action maintainer 모두 OIDC를
권장하며, OIDC를 쓸 수 없을 때의 stored client secret 방식은 덜 안전하다고
명시한다.
([action OIDC guide](https://github.com/Azure/artifact-signing-action/blob/main/docs/OIDC.md),
[alternative authentication warning](https://github.com/Azure/artifact-signing-action/blob/main/docs/AUTHN.md))

두 외부 action은 구현 시 검토한 release의 full-length commit SHA로 고정한다.
GitHub는 full SHA pin이 action을 immutable release로 사용하는 유일한 방법이라고
설명한다.
([GitHub Actions secure use](https://docs.github.com/en/actions/reference/security/secure-use))

Artifact Signing certificate는 수명이 3일이므로 RFC 3161 timestamp가 필수다.
공식 action의 SHA-256 file digest, Microsoft public RFC 3161 timestamp endpoint,
SHA-256 timestamp digest 입력을 사용한다.
([Artifact Signing integrations](https://learn.microsoft.com/en-us/azure/artifact-signing/how-to-signing-integrations),
[Authenticode timestamping](https://learn.microsoft.com/en-us/windows/win32/seccrypto/time-stamping-authenticode-signatures))

Artifact Signing을 썼다고 첫 릴리스부터 SmartScreen 경고가 반드시 사라지는 것은
아니다. Microsoft는 새 publisher/file이 평판을 쌓는 동안 warning이 나타날 수
있다고 명시하므로 “무경고 보장”을 홍보 문구로 쓰지 않는다.
([Windows code-signing options](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options))

### 검증

서명 직후와 최종 ZIP 재추출 후 같은 EXE에 다음 검사를 실행한다.

```powershell
signtool verify /pa /all /tw /v <path-to-executable>
Get-AuthenticodeSignature -LiteralPath <path-to-executable>
```

`/pa`는 기본 Authenticode policy, `/all`은 모든 embedded signature, `/tw`는 timestamp
부재를 warning으로 만든다. SignTool exit code `0`만 성공으로 받고 warning을 뜻하는
`2`도 release 실패로 처리한다. SignTool은 trusted authority, revocation, 선택한
policy를 검증할 수 있다.
([SignTool verify](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool))

PowerShell 결과는 문자열 grep 대신 object로 읽어 다음을 typed evidence로 만든다.

- signature status가 `Valid`
- expected publisher CN/O
- code-signing EKU와 공개 신뢰 chain
- RFC 3161 timestamp certificate 존재
- signed executable SHA-256
- target architecture와 canonical archive name

`Get-AuthenticodeSignature`는 Windows에서 Authenticode signature object를 반환하고
unsigned 파일은 관련 필드가 비어 있다.
([Get-AuthenticodeSignature](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-authenticodesignature))

Artifact Signing의 leaf certificate는 service가 짧은 수명으로 관리하므로 leaf
thumbprint 자체를 장기 allowlist로 고정하지 않는다. 대신 검증된 publisher subject,
신뢰 chain, EKU, timestamp와 현재 file digest를 묶는다. 공개 인증서는 signed binary
안에 포함되고 private key는 서비스 밖으로 제공되지 않는다.
([Artifact Signing FAQ](https://learn.microsoft.com/en-us/azure/artifact-signing/faq),
[certificate management](https://learn.microsoft.com/en-us/azure/artifact-signing/concept-certificate-management))

### 대안 — CA certificate, HSM, PFX

Artifact Signing onboarding이 실패하거나 정책상 사용할 수 없으면 공개 신뢰 CA의
OV code-signing certificate가 다음 선택이다. EV는 더 강한 신원 심사가 필요할 수
있지만 Microsoft는 2024년부터 SmartScreen 즉시 우회 이점이 없어졌다고 안내한다.
Self-signed certificate는 public release용이 아니다.
([Windows code-signing options](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options))

현대의 public code-signing certificate를 “PFX 한 파일을 GitHub secret에 저장”하는
모델로 전제하면 안 된다. CA/Browser Forum 기준은 2023-06-01부터 subscriber private
key가 적합한 hardware crypto module, cloud HSM 또는 준수 signing service 안에서
생성·보관·사용되도록 요구한다.
([CA/Browser Forum Code Signing Baseline Requirements](https://cabforum.org/working-groups/code-signing/requirements/))

따라서 대안 우선순위는 다음과 같다.

1. 발급 CA가 제공하는 cloud HSM/remote signing을 short-lived CI authentication으로
   호출한다.
2. CA가 제공한 hardware token/HSM을 잠금된 self-hosted Windows signing runner에
   연결하고, release tag만 서명하게 한다.
3. 발급 정책상 합법적으로 exportable PKCS#12가 존재하는 예외에만 ephemeral hosted
   runner의 certificate store로 import한다.

3번에서는 PFX와 password를 보호된 environment에서만 읽고, `SecureString`으로
import하며 private key를 non-exportable로 둔다. signing 후 certificate store와
임시 파일을 `always()` cleanup에서 제거한다. PowerShell은 `-Exportable`을 생략하면
import한 private key를 export할 수 없게 한다.
([Import-PfxCertificate](https://learn.microsoft.com/en-us/powershell/module/pki/import-pfxcertificate))

단, non-exportable import는 이미 GitHub에 저장된 PFX 원본의 장기 탈취 위험을 없애지
않는다. 새 public certificate에는 HSM/signing service 경로를 기본으로 하고 PFX는
호환 fallback으로만 유지한다.

PFX fallback에서도 password를 SignTool command line에 직접 넣지 않는다. 먼저
certificate store에 안전하게 import한 뒤 certificate selector로 다음 형태의
서명을 수행하고 동일 verifier를 통과시킨다.

```powershell
signtool sign /sha1 <certificate-thumbprint> /fd SHA256 `
  /tr <CA-RFC3161-timestamp-url> /td SHA256 `
  <path-to-executable>
```

SignTool은 PFX 직접 입력과 certificate store selector를 모두 지원하며, 최신 SDK는
file digest와 timestamp digest algorithm을 명시하도록 요구한다.
([SignTool](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool))

## GitHub attestation과 네이티브 서명의 경계

| 층 | 증명하는 것 | 증명하지 않는 것 |
| --- | --- | --- |
| SHA-256 | 받은 파일과 선택한 digest의 동일성 | digest를 누가 만들었는지 |
| GitHub attestation | artifact digest와 repository·workflow·source ref의 provenance | Apple/Microsoft publisher, 프로그램 안전성 |
| Developer ID | macOS가 인식하는 publisher와 signed bytes의 무결성 | Apple malware scan 완료 |
| Apple notarization | Apple 자동 검사와 code-signing 검사 통과 ticket | App Review, 취약점 부재 |
| Authenticode | Windows가 인식하는 publisher, signed bytes, timestamp | SmartScreen 평판의 즉시 확보, 취약점 부재 |
| SBOM | 선언된 구성요소와 dependency 투명성 | 해당 binary가 그 source로 빌드됐다는 사실 자체 |

GitHub attestation은 Actions workflow가 만든 claim이며, 검증자는 repository뿐 아니라
가능하면 signer workflow와 source ref까지 제한해야 한다. predicate 일부는 workflow
실행 문맥을 장악한 공격자가 거짓으로 채울 수 있으므로 trusted builder와 별도 policy가
필요하다.
([`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify))

반대로 native signature만으로 GitHub source commit을 알 수 없다. 그러므로 최종
사용자 검증은 다음 세 결과를 함께 보여 주는 것이 맞다.

1. archive SHA-256 일치
2. `gh attestation verify`에서 repository, signer workflow, release tag 일치
3. archive 안 실행 파일의 Developer ID/Authenticode 검증 성공

현재 repository의 `checksums.txt`와 artifact attestation은 1·2를 이미 준비한다.
이번 로드맵은 3을 추가하고, attestation subject가 **서명 완료 후의 최종 archive**를
가리키게 순서를 바꾸는 작업이다.

## 구현 단계와 완료 증거

### 0. 자격증명 없이 지금 구현

- 두 verifier의 command runner를 interface로 분리하고 synthetic output fixture로
  성공·서명 없음·publisher 불일치·timestamp 없음·notarization 거부를 테스트한다.
- evidence schema에는 platform, architecture, executable digest, canonical archive,
  native signature status, expected-identity match, timestamp, notarization/stapling mode만
  둔다.
- stable release policy는 네 개 evidence가 모두 현재 digest와 맞을 때만 열리도록
  만들되, 실제 verifier가 없는 동안 계속 fail-closed한다.
- signing job은 PR이나 임의 branch에서 실행되지 않고 protected release tag와
  environment에서만 실행된다는 workflow contract test를 추가한다.
- action SHA pin, 최소 권한, secret 이름/값의 로그·artifact 부재를 actionlint와
  repository policy로 검사한다.

### 1. 외부 등록

- Apple Developer Program, Developer ID Application certificate, App Store Connect
  team API key를 준비한다.
- Azure Organization/DBA Public identity validation, Public Trust certificate profile,
  OIDC federated identity, profile-scoped signer role을 준비한다.
- 법적 publisher 표기를 Apple과 Microsoft 양쪽에서 확인하고 non-secret allowlist로
  code에 고정한다.

### 2. 실제 signed prerelease

- 네 executable을 실제 서명하고 verifier JSON을 생성한다.
- macOS 두 submission이 `Accepted`이고 warning이 검토됐는지 확인한다.
- Windows 두 EXE가 `/pa /all /tw`에서 exit `0`인지 확인한다.
- 그 실행 파일들로 archive를 다시 만들고 release contract, SBOM, checksum,
  attestation을 재생성한다.
- 같은 archive를 다시 추출해 서명과 digest binding을 재검증한다.

### 3. 깨끗한 운영체제 검증

- Intel Mac과 Apple Silicon Mac에서 실제 브라우저 다운로드로 quarantine이 붙은
  release archive를 실행한다.
- Windows x64와 ARM64 clean VM에서 실제 release ZIP을 다운로드해 signature UI,
  SignTool, CLI 실행을 확인한다.
- SmartScreen 경고 발생 여부는 결과로 기록하되 signed 여부와 혼동하지 않는다.
- macOS `tar.gz` 경로는 online ticket 상태를 기록하고, 오프라인 요구가 있으면
  DMG/PKG 추가 전 stable 완료로 과장하지 않는다.

### 4. stable gate 개방

아래 항목이 모두 같은 tag와 digest로 증명될 때만 기존
`stable_native_signing_required`를 signed stable 허용 상태로 바꾼다.

- Darwin amd64·arm64: Developer ID, timestamp, runtime, identity, identifier,
  notarization, fresh quarantined download 통과
- Windows amd64·arm64: Authenticode, SHA-256, RFC 3161 timestamp, Public Trust chain,
  publisher, clean-system 실행 통과
- Linux amd64·arm64: 기존 release contract 통과
- 여섯 archive: canonical name/content, SBOM, checksum 통과
- 최종 archive와 checksum manifest: GitHub attestation 검증 통과
- release note: attestation과 native signing의 차이 및 macOS stapling mode를 정확히
  표시

## 외부 준비물 요약

저장소 코드만으로 만들 수 없는 것은 다음이다.

- Apple Developer Program 활성 membership
- Developer ID Application certificate와 그 private key
- App Store Connect team API key와 발급 주체/issuer 정보
- Apple에서 표시될 정확한 publisher 및 stable code identifier 결정
- paid Azure subscription과 Microsoft Entra tenant
- Azure 결제 계정의 법적 Organization/DBA 정보 정합성
- 완료된 Public identity validation과 Public Trust certificate profile
- GitHub release environment만 신뢰하는 OIDC federation 및 profile-scoped signer role
- Intel/Apple Silicon macOS, x64/ARM64 Windows의 clean-system acceptance 환경

비밀 값의 실제 이름과 값은 이 문서의 산출물이 아니다. 준비가 끝나기 전에는 unsigned
prerelease만 허용하고, stable gate는 닫힌 상태가 정답이다.
