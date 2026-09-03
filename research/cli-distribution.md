# `coupangctl` 단일 바이너리 배포 결정

조사일: 2026-09-03

## 결론

`coupangctl`의 기본 설치물은 **`coupangctl` 단일 Go 바이너리 하나**여야 한다.
Orca, Playwright, Node, Chrome 확장 프로그램은 기본 설치 의존성이 아니다. 사용자의
설치된 Chrome과 `coupangctl` 전용 프로필만 사용하고, 확장은 명시적으로 필요한
호환 경로에만 남긴다.

Orca 같은 별도 제어 CLI를 자동 설치하는 방식은 문제를 해결하지 않는다. 설치해야 할
신뢰 경계, 업데이트 주체, 장애 지점과 바이너리 수를 하나 더 만든다. 필요한 것은
Orca 설치 자동화가 아니라 **`coupangctl` 자체의 검증 가능한 설치 자동화**다.

네이티브 코드 서명이 없는 현재 단계의 권장 순서는 다음과 같다.

1. 공개 태그 전 개발자용 현재 경로는 검증된
   `go install github.com/JungHoonGhae/coupang-ctl/cmd/coupangctl@main`으로 둔다.
   clone은 필요 없지만 움직이는 branch이므로 안정 릴리스라고 부르지 않는다.
2. 첫 공개 베타부터 macOS·Linux에는 서드파티 Homebrew tap을 우선 제공하되,
   네이티브 서명 전 macOS는 tagged source를 로컬에서 빌드하는 formula를 쓴다.
   Windows에는 WinGet portable package를 beta 경로로 제공한다. 설치·업그레이드·
   제거의 소유자는 패키지 관리자로 일관되게 유지한다.
3. 패키지 관리자가 없는 환경에는 GitHub Release 아카이브를 제공하고 SHA-256과
   GitHub artifact attestation을 함께 검증하도록 한다.
4. `curl ... | sh`와 자동 `sudo`, Gatekeeper 격리 속성 삭제, SmartScreen 우회는
   기본 설치 경로로 제공하지 않는다.
5. macOS Developer ID 서명·notarization과 Windows Authenticode 서명이 끝나기
   전에는 이 경로를 “마찰 없는 일반 사용자 설치”라고 홍보하지 않는다.

## 현재 저장소에서 이미 증명된 것

현재 [`.goreleaser.yaml`](../.goreleaser.yaml)은 CGO가 없는 macOS, Linux,
Windows의 amd64·arm64 빌드 여섯 개를 만들고, Unix 계열은 `tar.gz`, Windows는
`zip`으로 묶는다. 각 아카이브에는 실행 파일, `LICENSE`, `README.md`,
`BROWSER_BRIDGE.md`만 들어가며 SHA-256 `checksums.txt`와 아카이브별 SPDX JSON
SBOM도 생성한다.

현재 [release workflow](../.github/workflows/release.yml)는 태그를 확인하고 테스트와
릴리스 계약 검증을 먼저 통과시킨 뒤, `actions/attest`의 `subject-path`로
`checksums.txt` 자체를, `subject-checksums`로 체크섬 파일에 열거된 산출물을 각각
attest하고 draft 릴리스를 공개한다. 이 사용법은
GoReleaser의 [attestation 공식 예시](https://goreleaser.com/customization/publish/attestations/)
및 `actions/attest`의
[`subject-checksums` 계약](https://github.com/actions/attest#identify-subjects-with-checksums-file)과
일치한다. 로컬 snapshot도 동일한 여섯 아카이브 이름을 실제로 생성한다.

현재 로컬과 원격 저장소에는 공개 태그가 없다. 대신 공개 원격의 정확한 commit을 새
임시 `GOBIN`에 `go install`하고, 설치된 바이너리가 그 source의 pseudo-version을
구조화된 `version` 응답으로 출력하는 흐름은 실제로 검증됐다. Go 공식 module
계약도 `go install package@version`이 현재 디렉터리의 `go.mod`와 독립된 module-aware
설치이며, branch나 revision을 canonical pseudo-version으로 해석한다고 명시한다.
([Go modules의 `go install`](https://go.dev/ref/mod#go-install))

따라서 [README의 개발 스냅샷 경로](../README.md)는 현재 사실과 일치한다. 다만
Homebrew/WinGet manifest에 들어갈 불변 릴리스 URL과 최종 SHA-256은 아직 만들 수
없다. 태그와 릴리스 생성은 이 조사 범위에서 수행하지 않았다.

## 선행 사례에서 가져올 원칙

GitHub CLI는 macOS에서 Homebrew, Windows에서 WinGet, Linux에서 관리되는 패키지
저장소를 공식 경로로 두면서 precompiled binary도 제공한다. 특히 Windows 문서는
설치와 업그레이드를 모두 WinGet에 맡긴다.
([GitHub CLI 설치 개요](https://github.com/cli/cli#installation),
[Windows 설치](https://github.com/cli/cli/blob/trunk/docs/install_windows.md),
[Linux 설치](https://github.com/cli/cli/blob/trunk/docs/install_linux.md))

`age` 역시 Homebrew와 WinGet을 첫 줄에 두고 직접 다운로드를 함께 제공하지만,
prebuilt archive에는 별도의 Sigsum transparency proof를 제공한다.
([age 설치](https://github.com/FiloSottile/age#installation),
[Sigsum 검증](https://github.com/FiloSottile/age/blob/main/SIGSUM.md))

`restic`은 여러 OS 패키지 관리자와 공식 바이너리를 함께 제공한다. 직접 다운로드의
SHA-256 목록은 PGP 서명으로 인증하고, 자체 업데이트도 서명된 체크섬을 먼저 검증한
뒤에만 실행한다. 즉 “self-update가 편하다”가 아니라 **업데이트 메타데이터의
진위를 바이너리 안에서 검증할 수 있을 때만 self-update를 제공한다**는 사례다.
([restic 설치 및 self-update 계약](https://github.com/restic/restic/blob/master/doc/020_installation.rst))

이 세 사례가 지지하는 공통 구조는 다음과 같다.

- 런타임은 독립 실행형 CLI다.
- 운영체제별 패키지 관리자를 우선 사용한다.
- 직접 다운로드는 보조 경로다.
- 해시만 게시하지 않고, 해시 또는 산출물에 별도의 인증 가능한 출처 증명을 붙인다.
- 설치 방법을 섞지 않고 처음 설치한 관리자가 업그레이드와 제거도 맡는다.

## 권장 사용자 경로

### macOS와 Linux: source-build Homebrew tap

첫 공개 베타의 대표 명령은 완전 수식 이름 하나여야 한다.

```bash
brew install JungHoonGhae/tap/coupangctl
```

Homebrew는 서드파티 tap의 완전 수식 이름을 직접 설치하면 해당 항목만 신뢰하고 tap을
자동 추가한다. 이후 `brew update`, `brew upgrade coupangctl`,
`brew uninstall coupangctl`로 같은 관리자가 생명주기를 맡는다.
([tap 설치와 업데이트](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap),
[tap trust 모델](https://docs.brew.sh/Tap-Trust),
[upgrade/uninstall 명령](https://docs.brew.sh/Manpage))

서명 전에는 immutable tagged source와 SHA-256을 고정한 작은 수동 formula가 Go를
build dependency로 사용해 `cmd/coupangctl`을 로컬에서 빌드하게 한다. 사용 시
런타임 산출물은 여전히 `coupangctl` 하나지만 설치 시에는 Homebrew와 Go가 필요하다.
Homebrew의 공식 formula 정책도 open-source command-line software에는 stable tagged
source build를 기본으로 두며, platform-specific prebuilt binary는 core formula의
정상 경로로 보지 않는다.
([Homebrew Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae),
[Formula Cookbook](https://docs.brew.sh/Formula-Cookbook))

서명 전 macOS prebuilt cask는 일반 사용자 완성 경로가 아니다.
Apple은 알려진 개발자로 등록되지 않은 프로그램에 경고를 표시하며, 사용자가
보안 설정을 수동으로 무시하는 것을 권장하지 않는다.
([Apple의 unknown developer 안내](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unknown-developer-mh40616/mac))
공식 Homebrew cask도 Gatekeeper 통과를 요구한다.
([Homebrew Acceptable Casks](https://docs.brew.sh/Acceptable-Casks))
따라서 다음을 하지 않는다.

- cask의 `caveats`나 설치 스크립트로 `xattr -d com.apple.quarantine` 실행 유도
- Gatekeeper 비활성화
- 서드파티 인증서를 신뢰 저장소에 자동 추가
- Homebrew가 OS 코드 서명을 대체한다고 주장

GitHub CLI도 현재 Homebrew 빌드가 x86_64에서는
unsigned, Apple Silicon에서는 ad-hoc signed이며 안정된 signing identity가 없다고
명시한다. 이는 beta 편의 경로일 뿐 Developer ID의 대체물이 아니다.
([GitHub CLI macOS keyring 분석](https://github.com/cli/cli/blob/trunk/docs/macos-keyring.md))

Developer ID 서명과 notarization을 추가한 뒤에는 prebuilt archive로 전환한다.
GoReleaser v2.10+ 기준 deprecated `brews`가 아니라 `homebrew_casks`를 사용한다.
Cask는 archive 안의 CLI를 `binary` artifact로 연결하고 SHA-256을 검증할 수 있으며,
태그 전에는 `skip_upload: true`로 생성 결과만 검사할 수 있다.
([GoReleaser Homebrew Casks](https://goreleaser.com/customization/publish/homebrew_casks/),
[Homebrew Cask `binary`·SHA-256 계약](https://docs.brew.sh/Cask-Cookbook))

### Windows: WinGet ZIP + nested portable

현재 Windows ZIP을 그대로 WinGet의 archive installer로 사용할 수 있다. manifest는
다음 정보를 고정해야 한다.

- `InstallerType: zip`
- `NestedInstallerType: portable`
- `NestedInstallerFiles`의 `coupangctl.exe`와 `PortableCommandAlias: coupangctl`
- amd64는 WinGet `x64`, arm64는 `arm64`
- 각 GitHub Release URL과 그 ZIP의 `InstallerSha256`
- 기본 설치 범위는 `user`

이 구조는 Microsoft의 community repository 지침과 고정된 WinGet 1.12 installer
schema가 지원한다. community repository는 multi-file manifest를 요구하고 ZIP
installer에는 `NestedInstallerType`이 필요하다. 개발 중인 `latest` schema 대신
현재 저장소가 받는 `1.12.0`을 생성 결과에 고정한다.
([community manifest 지침](https://github.com/microsoft/winget-pkgs/blob/master/.github/instructions/manifests.instructions.md),
[WinGet 1.12 installer schema](https://github.com/microsoft/winget-pkgs/blob/master/doc/manifest/schema/1.12.0/installer.md))

첫 릴리스 뒤 `wingetcreate new`로 최초 PR을 만들고, 다음 릴리스부터
`wingetcreate update`를 사용한다. Community Repository는 manifest PR을 자동
검증하고 알려진 악성 패키지를 검사하지만, 실제 publish에는 그 외부 저장소의
검토가 필요하다.
([manifest 제출 절차](https://learn.microsoft.com/windows/package-manager/package/repository),
[WinGetCreate](https://github.com/microsoft/winget-create))

WinGet은 설치, 업그레이드, 제거와 PATH alias를 일관되게 처리하는 장점이 있다.
([WinGet 개요](https://learn.microsoft.com/windows/package-manager/winget/),
[upgrade](https://learn.microsoft.com/windows/package-manager/winget/upgrade))
하지만 WinGet의 `InstallerSha256`이나 GitHub attestation이 Authenticode를
대체하지는 않는다. Microsoft에 따르면 unsigned binary는 버전마다 SmartScreen
평판을 새로 쌓고 “Windows protected your PC” 경고가 나타날 수 있으며, 일부
enterprise policy에서는 실행 자체가 막힌다. Smart App Control도 알려지지 않은
unsigned code를 차단할 수 있다.
([SmartScreen reputation](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation),
[Smart App Control](https://learn.microsoft.com/windows/apps/develop/smart-app-control/overview))

### 패키지 관리자가 없는 환경: 직접 다운로드

직접 다운로드는 “archive, checksum, provenance” 세 개를 한 세트로 다룬다.

```bash
gh release download v0.1.0 \
  -R JungHoonGhae/coupang-ctl \
  -p 'coupangctl_0.1.0_darwin_arm64.tar.gz' \
  -p checksums.txt

archive=coupangctl_0.1.0_darwin_arm64.tar.gz
expected=$(awk -v name="$archive" '$2 == name { print $1 }' checksums.txt)
test -n "$expected"
actual=$(shasum -a 256 "$archive" | awk '{ print $1 }')
test "$actual" = "$expected"

gh attestation verify coupangctl_0.1.0_darwin_arm64.tar.gz \
  -R JungHoonGhae/coupang-ctl \
  --signer-workflow JungHoonGhae/coupang-ctl/.github/workflows/release.yml \
  --source-ref refs/tags/v0.1.0
```

Linux에서는 `sha256sum -c checksums.txt`, Windows PowerShell에서는
`Get-FileHash -Algorithm SHA256`을 사용한다. `gh attestation verify`는 파일 digest,
repository owner와 signer workflow identity를 검증하며, `--source-ref`로 예상 태그도
제한할 수 있다. GitHub는 attestation이 산출물의 안전성을 보증하는 것이 아니라
어떤 source와 workflow가 만들었는지를 연결하는 증명이라고 명시한다.
([GitHub artifact attestation 개념](https://docs.github.com/actions/concepts/security/artifact-attestations),
[`gh attestation verify` 계약](https://cli.github.com/manual/gh_attestation_verify))

SHA-256만 통과한 경우는 다운로드 손상 또는 게시된 해시와의 불일치만 찾는다.
공격자가 archive와 `checksums.txt`를 함께 바꿀 수 있는 경로에서는 publisher
authenticity 증명이 아니다. 반대로 attestation은 OS 실행 신뢰를 대체하지 않는다.
두 검증 결과와 플랫폼 코드 서명은 서로 다른 층이다.

| 검증 | 증명하는 것 | 증명하지 않는 것 |
| --- | --- | --- |
| SHA-256 | 받은 파일이 선택한 체크섬 목록과 같은지 | 그 목록을 누가 만들었는지 |
| GitHub attestation | artifact digest를 만든 repository·workflow provenance | 코드 안전성, Apple/Microsoft publisher identity |
| Homebrew/WinGet hash | package manifest가 가리킨 고정 artifact와 같은지 | 네이티브 코드 서명 |
| Developer ID/Authenticode | OS가 인식하는 publisher와 변경 여부 | 소스 코드가 안전한지 |

## 릴리스 파일명 계약

현재 파일명은 바꾸지 않는 것이 좋다.

```text
coupangctl_<version>_darwin_amd64.tar.gz
coupangctl_<version>_darwin_arm64.tar.gz
coupangctl_<version>_linux_amd64.tar.gz
coupangctl_<version>_linux_arm64.tar.gz
coupangctl_<version>_windows_amd64.zip
coupangctl_<version>_windows_arm64.zip
checksums.txt
<archive>.sbom.json
```

이름은 project, version, OS, architecture가 기계적으로 파싱 가능하고 Windows만
ZIP이라는 규칙이 명확하다. GitHub CLI, age, restic도 같은 정보 순서에 약간의
구분자 차이만 둔 안정된 이름을 사용한다.
([GitHub CLI releases](https://github.com/cli/cli/releases),
[age releases](https://github.com/FiloSottile/age/releases),
[restic releases](https://github.com/restic/restic/releases))

첫 공개 릴리스 뒤에는 이 이름을 호환 API로 취급한다. installer, tap, WinGet manifest
및 사용자 스크립트가 여기에 의존하므로 `darwin`을 `macOS`로 바꾸거나 version의
`v` 포함 여부를 바꾸지 않는다. raw executable을 중복 release asset으로 추가하지
않는다. 동일 대상 후보가 두 개이면 자동 선택과 보안 검증이 불필요하게 복잡해진다.

현재 `checksums.txt`는 archive와 SBOM을 열거하고 `subject-checksums`가 각 열거
artifact를 attest한다. 체크섬 파일 자체는 자기 자신을 열거하지 못하므로 별도의
`actions/attest` 호출이 `subject-path: dist/checksums.txt`를 attest한다.

## 설치·업그레이드·제거 계약

### 설치

패키지 관리자 경로는 사용자 범위 설치를 기본으로 하며 자동 관리자 권한 상승을 하지
않는다. 직접 설치기는 다음 조건을 모두 만족할 때만 보조 경로로 추가한다.

- `--version` 또는 명시한 immutable SemVer tag를 요구하고, `latest` 리디렉션을
  최종 provenance policy로 사용하지 않는다.
- OS/architecture를 allowlist로 매핑하고 알 수 없는 조합은 거부한다.
- 새 임시 디렉터리에 archive와 검증 메타데이터를 내려받는다.
- 예상 archive 한 개의 SHA-256을 확인한 뒤 path traversal 없는 allowlist로 푼다.
- 새 파일에서 `coupangctl version`을 실행한 뒤 동일 파일시스템에서 원자적으로
  교체한다.
- root가 아닌 사용자 디렉터리를 기본값으로 하고, PATH 변경은 고지한다.
- 실패하면 기존 바이너리를 보존하고 임시 파일을 지운다.
- 쿠키, OTP, Chrome profile, 주문 DB를 읽거나 이동하지 않는다.

설치 스크립트를 추가하더라도 README의 한 줄 명령은 `curl | sh`가 아니라
“파일로 다운로드 → 내용 확인 → 실행”을 기본으로 둔다. 스크립트 자체도 versioned
release asset으로 만들고 checksum/attestation 대상으로 포함한다. 스크립트에
GoReleaser 산출물 선택 규칙을 중복 구현한다면 동일 fixture로 여섯 대상 전체를
Linux·macOS·Windows CI에서 검사한다.

### 업그레이드

설치 주체를 섞지 않는다.

- Homebrew 설치: `brew upgrade coupangctl`
- WinGet 설치: `winget upgrade --id JungHoonGhae.coupangctl --exact`
- 직접 설치: 같은 version-pinned installer를 다시 실행
- 소스 설치: 사용자가 선택한 source workflow로 다시 빌드

현재 단계에서는 바이너리가 자기 자신을 바꾸는 `coupangctl self-update`를 넣지
않는다. 관리자 권한, 실행 중 파일 교체, rollback, package-manager 충돌뿐 아니라
서명되거나 내장된 trust root로 update metadata를 검증하는 계약이 먼저 필요하다.
원한다면 read-only `coupangctl update check --json`만 먼저 구현할 수 있지만,
다운로드나 교체는 하지 않아야 한다.

### 제거와 사용자 데이터

패키지 관리자의 uninstall은 자신이 설치한 실행 파일과 alias만 제거해야 한다.
현재 애플리케이션 상태는 플랫폼별로 다음 위치에 별도로 저장된다.

```text
macOS:   ~/Library/Application Support/coupangctl
Windows: %LOCALAPPDATA%\coupangctl
Linux:   ${XDG_STATE_HOME:-~/.local/state}/coupangctl
```

여기에는 전용 브라우저 프로필과 로컬 SQLite가 있으므로 Homebrew `zap`이나 WinGet
uninstall hook으로 묵시적으로 삭제하면 안 된다. 업그레이드에서도 그대로 보존한다.
완전 삭제는 바이너리를 제거하기 전에 별도의 명시적 확인 명령으로 제공해야 한다.
현재 `orders purge`는 정규화 주문 범위만 지우며 전용 브라우저 프로필까지 포함하는
전체 제품 데이터 삭제 계약은 아니다. 따라서 전체 삭제 명령을 구현하기 전까지는
위 경로와 잔존 범위를 uninstall 문서에 정확히 고지한다.

## 코드 서명 이후 전환

macOS는 Developer ID로 모든 executable을 서명하고 hardened runtime과 secure
timestamp를 사용한 뒤 Apple notary service에 제출한다. Apple은 notarization이
악성 코드 검사와 code-signing 문제 검사를 수행하고 Gatekeeper가 확인할 ticket을
발급한다고 설명한다. 현재의 `tar.gz`를 유지하려면 실제 배포 archive와 내부
executable을 대상으로 깨끗한 VM Gatekeeper 테스트를 추가해야 한다.
([Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution),
[custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow))

Windows는 일관된 trusted publisher certificate로 `coupangctl.exe`를 Authenticode
서명하고 timestamp한 뒤 ZIP을 만든다. Microsoft는 Store 밖 배포에는 Artifact
Signing 또는 신뢰된 CA의 인증서를 권장하며, EV 인증서도 즉시 SmartScreen 평판을
보장하지 않는다고 명시한다.
([Windows code-signing options](https://learn.microsoft.com/windows/apps/package-and-deploy/code-signing-options),
[SmartScreen reputation](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation))

서명 순서는 반드시 `build → sign executable → verify signature → archive → checksum
→ attest → publish`다. 서명 후 binary를 수정하면 서명이 깨진다. GitHub provenance는
서명된 최종 archive와 SBOM을 가리키도록 유지한다.

## 태그 없이 지금 구현 가능한 순서

다음 작업은 tag, release, credential 또는 외부 package repository mutation 없이
현재 저장소에서 완전히 구현·검증할 수 있다.

1. **완료 — 설치 계약 fixture**: 여섯 canonical asset name, OS/architecture
   mapping, archive format과 내부 executable name은 하나의 typed release-contract
   module에 있으며 release checker와 installer E2E가 같은 표를 사용한다.
2. **완료 — 안전한 직접 설치기**: POSIX shell과 PowerShell installer는
   version-pinned, user-scope, checksum-first, allowlist, atomic-replace,
   no-auto-sudo 계약을 구현한다. 로컬 HTTP fixture가 실제 사용자 홈과 외부
   릴리스 없이 두 스크립트를 E2E 검사한다.
3. **완료 — 체크섬 파일 attestation**: 기존 12개 checksummed artifact
   attestation과 별도로 `checksums.txt` 자체를 attest하고, 검증 명령은
   `--signer-workflow`와 `--source-ref`를 고정한다.
4. **완료 — uninstall/data 경계**: package manager와 직접 설치기의 수명주기는
   binary만 소유하며 브라우저 프로필과 SQLite를 보존한다고 문서화했다. 현재
   `orders purge`보다 넓은 전체 로컬 데이터 삭제는 제공하지 않으며, 향후 필요할
   때에만 별도 confirmation-gated typed command와 symlink/path ownership 테스트를
   먼저 만든다.
5. **완료 — Homebrew source formula dry run**: typed generator가 tagged source와
   SHA-256, Go build dependency, `std_go_args`, `cmd/coupangctl`만 사용하는 formula를
   만든다. 새 출력 디렉터리만 허용하며 macOS CI가 실제 Ruby parser로 검사한다.
   실제 tap repository와 write token은 첫 release 직전에 별도로 준비한다.
6. **완료 — WinGet template dry run**: 같은 generator가 x64·arm64 ZIP을 가리키는
   `1.12.0` multi-file version, installer, ko-KR default-locale manifest를 만든다.
   Go contract test와 Windows CI가 portable alias·archive mapping을 확인하고 macOS
   CI가 YAML을 파싱한다. 실제 URL/hash 생성과 Community Repository validation/PR은
   첫 release 이후에 수행한다.
7. **P1 — signed-build seams**: macOS codesign/notary와 Windows signing 단계를 아직
   실행하지 않더라도, unsigned release를 명시적으로 표시하고 signing credential이
   없는 stable release는 실패하도록 opt-in gate를 설계한다. 서명 후 전환할
   `homebrew_casks` dry run도 이 단계에서 최종 서명 archive를 대상으로 추가한다.
   secret 이름만 참조하고 값은 fixture, log, artifact에 넣지 않는다.

첫 공개 태그 이후에만 가능한 외부 작업은 tap repository publish, WinGet Community
Repository PR, 실제 GitHub attestation 조회, macOS notarization, Windows publisher
identity 검증이다.

## 최종 판단

사용자가 설치해야 할 것은 Orca도 확장 프로그램도 아닌 `coupangctl` 하나다. 태그
전에는 검증된 `go install ...@main`이 clone 없는 개발 스냅샷 경로이고, 현재
GoReleaser, release-contract, 사용자 범위 직접 설치기, Homebrew/WinGet metadata
generator까지 하나의 배포 계약으로 연결되어 있다. 첫 공개 tag 뒤 남는 일은 실제
hash로 metadata를 생성하고 외부 package repository의 검증·심사를 통과하는 것이다.

다만 “서명 전에도 자동 설치되니 일반 사용자가 아무 경고 없이 쓸 수 있다”는 결론은
근거가 없다. 패키지 관리자와 GitHub provenance는 공급망 검증과 생명주기를 크게
개선하지만 Gatekeeper·SmartScreen의 publisher trust는 대신하지 못한다. 따라서
서명 전은 투명한 beta 경로, 서명 후가 기본 일반 사용자 경로라는 단계 구분을 제품
문구와 릴리스 gate에 그대로 유지해야 한다.
