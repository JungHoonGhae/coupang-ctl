# 릴리스 계약

현재 공개 태그 릴리스는 없습니다. README의 `go install …@main`은 clone 없는
개발 스냅샷 설치 경로이며 안정 릴리스로 취급하지 않습니다.
이 문서는 첫 태그부터 동일하게 적용할 배포 계약을 설명합니다.

## 자동화된 태그 릴리스

`v0.1.0-rc.1` 같은 SemVer prerelease 태그가 원격 저장소에 push되면 `Release`
워크플로가 다음 순서로 실행됩니다. 현재 네이티브 서명 단계가 아직 없으므로
`v0.1.0` 같은 stable 태그는 첫 빌드 전에 정책 검사에서 거부됩니다. credential의
존재나 수동 boolean은 서명 증거로 인정하지 않으며, stable 공개는 macOS와 Windows
산출물의 암호학적 서명 검증 adapter가 구현된 뒤에만 열 수 있습니다.

1. 타입화된 release policy로 malformed tag와 unsigned stable publication 거부
2. Go 테스트·vet, TypeScript 연구 probe typecheck, MV3 확장 계약 테스트
3. Linux·macOS·Windows의 실제 설치 Chrome을 깨끗한 임시 전용 프로필로 두 번
   headless 실행·종료해 발견·프로필 재사용·잠금 해제·정상 종료를 검증
4. CGO를 끈 macOS·Linux·Windows의 amd64·arm64 바이너리 여섯 개 빌드
5. macOS·Linux는 `tar.gz`, Windows는 `zip`으로 패키징
6. 각 아카이브의 SPDX JSON SBOM과 SHA-256 `checksums.txt` 생성
7. 저장소의 `releasecheck`로 대상 조합, 파일 목록, SBOM 완전성, 모든
   체크섬을 재검증
8. `checksums.txt` 자체와 그 파일에 열거된 열두 산출물 전체에 각각 GitHub
   Actions provenance attestation 생성
9. 앞 단계가 모두 성공했을 때만 unsigned 표시가 붙은 GitHub prerelease 공개

릴리스 아카이브의 허용 목록은 다음 네 파일뿐입니다.

- 현재 플랫폼의 `coupangctl` 또는 `coupangctl.exe`
- `LICENSE`
- `README.md`
- `BROWSER_BRIDGE.md`

연구 probe, Node 런타임, 브라우저, 프로필, 쿠키, 자격 증명, 원본 주문
payload, 테스트 fixture는 배포물에 들어갈 수 없습니다. 아카이브에 파일이 하나라도
더 있거나 여섯 대상 중 하나가 빠지면 draft 공개 전에 실패합니다.

## 태그 전 확인

유지관리자는 깨끗한 `main`에서 다음을 확인합니다.

```bash
go test ./...
go vet ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color=false
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
npm ci
npm run typecheck
npm run test:extension
goreleaser check
goreleaser release --snapshot --clean
go run ./cmd/releasecheck --require-sbom ./dist
go run ./cmd/releasepolicy --tag v0.1.0-rc.1
sh -n ./installers/install.sh
```

로컬 snapshot에는 Syft가 필요합니다. CI는 GoReleaser `v2.18.0`, Syft
`v1.42.3` 및 사용한 GitHub Actions를 고정된 버전·commit으로 실행합니다.
실제 태그 생성과 push는 자동화하지 않습니다.

직접 설치기는 태그를 반드시 요구하고, 지원 플랫폼·아카이브명·실행 파일명을
`internal/releasecontract`의 타입화된 계약과 대조하는 synthetic E2E를 통과해야
합니다. Linux와 macOS는 실제 POSIX 스크립트를, Windows는 실제 PowerShell
스크립트를 로컬 HTTP fixture에 연결하며 사용자 홈이나 외부 릴리스를 건드리지
않습니다. 세부 설치·업그레이드·데이터 보존 규칙은
[`installers/README.md`](installers/README.md)에 있습니다.

## 패키지 관리자 메타데이터

첫 태그와 릴리스 아카이브가 준비되면 유지관리자는 source archive와 Windows ZIP의
실제 SHA-256으로 다음 명령을 실행합니다. 출력 경로는 존재하지 않는 새 디렉터리여야
하며, 이 도구는 로컬 파일만 만들고 tap이나 WinGet 저장소에는 게시하지 않습니다.

```bash
go run ./cmd/packagemanifests \
  --tag v0.1.0 \
  --source-sha256 SOURCE_ARCHIVE_SHA256 \
  --windows-amd64-sha256 WINDOWS_AMD64_ZIP_SHA256 \
  --windows-arm64-sha256 WINDOWS_ARM64_ZIP_SHA256 \
  --output /new/path/coupangctl-package-manifests
```

결과는 tagged source를 로컬에서 빌드하는 Homebrew formula 하나와 WinGet community
repository용 version, installer, `ko-KR` default-locale manifest 세 개입니다. WinGet
metadata는 현재 community repository의 고정된 multi-file schema `1.12.0`을
사용하고, Windows ZIP 안의 portable 실행 파일과 사용자 범위 command alias를
명시합니다. 생성기는 release-contract의 canonical URL·archive 이름을 사용하며
기존 디렉터리를 덮어쓰거나 병합하지 않습니다.

CI는 합성 tag와 hash만으로 같은 생성기를 실행해 Homebrew Ruby 문법, WinGet YAML
파싱, x64·arm64 portable 계약을 확인합니다. 실제 tag URL과 hash의 원격 접근성,
Homebrew tap 반영, WinGet community validation/PR은 릴리스 이후의 외부 단계입니다.

선택적 일반 Chrome 호환 확장의 스토어 ZIP은 CLI 릴리스 아카이브와 분리해 새
경로에 만들고 검증합니다. 이는 일반 사용자의 기본 설치물이 아닙니다.

```bash
go run ./cmd/extensionpack --output /new/path/coupangctl-extension.zip
go run ./cmd/extensionpack --verify /new/path/coupangctl-extension.zip
```

CI도 같은 생성·재검증 계약을 실행합니다. 실제 Web Store 업로드 전에는
[`extension/STORE_LISTING.md`](extension/STORE_LISTING.md)의 UI 미디어, public
key/extension ID, 개인정보 답안, 심사 게이트를 별도로 완료해야 합니다.

## 다운로드 검증

릴리스가 생긴 뒤 사용자는 원하는 태그의 산출물을 모두 받은 디렉터리에서
다음처럼 무결성과 빌드 출처를 각각 확인할 수 있습니다.

```bash
gh attestation verify checksums.txt \
  -R JungHoonGhae/coupang-ctl \
  --signer-workflow JungHoonGhae/coupang-ctl/.github/workflows/release.yml \
  --source-ref refs/tags/vVERSION

shasum -a 256 -c checksums.txt
gh attestation verify coupangctl_VERSION_OS_ARCH.tar.gz \
  -R JungHoonGhae/coupang-ctl \
  --signer-workflow JungHoonGhae/coupang-ctl/.github/workflows/release.yml \
  --source-ref refs/tags/vVERSION
```

Windows 아카이브는 `.zip` 파일명을 사용합니다. SHA-256은 다운로드 파일의
무결성을 확인하고, attestation은 해당 digest를 이 저장소의 GitHub Actions
빌드와 연결합니다. 어느 쪽도 프로그램 자체의 안전성을 보증하지는 않습니다.

설치·업그레이드·제거를 포함한 단일 바이너리 배포 결정과 패키지 관리자별
단계는 [`research/cli-distribution.md`](research/cli-distribution.md)에 기록되어
있습니다. Orca와 선택 탭 확장은 기본 설치 의존성이 아닙니다.

## 아직 남은 배포 게이트

- macOS Developer ID 서명·notarization과 Windows Authenticode 서명은 아직
  구성되지 않았습니다. 따라서 stable tag는 `stable_native_signing_required`로
  fail-closed합니다. GitHub provenance를 운영체제 코드 서명으로 표현하면 안
  됩니다.
- 향후 Apple adapter가 사용할 secret 이름은 `MACOS_SIGN_P12`,
  `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_KEY`, `MACOS_NOTARY_KEY_ID`,
  `MACOS_NOTARY_ISSUER_ID`로 예약합니다. 값은 문서, fixture, 로그, artifact에
  넣지 않습니다. Windows는 Artifact Signing과 certificate/PFX 중 방식을 결정한
  뒤에만 secret 이름을 확정합니다.
- 언어 기준은 Go 1.26이지만 저장소의 `toolchain` 지시문과 CI 빌드는 보안 수정이
  반영된 Go 1.26.8을 사용합니다. 새 보안 패치가 나오면 Dependabot과
  `govulncheck` 결과를 확인해 같은 minor 계열에서 올립니다.
- 일반 Chrome 확장의 자동 설치는 Chrome Web Store 검토와 배포가 끝나기
  전까지 지원하지 않습니다. 현재 바이너리는 검토된 번들을 풀어 주지만 사용자가
  Chrome에서 그 경로를 직접 로드해야 합니다. 이 확장은 기본 설치나 첫 실행에는
  필요하지 않은 선택적 호환 adapter입니다.
- 깨끗한 macOS·Linux·Windows 사용자 환경에서 install/doctor/sync/uninstall
  전체 행렬을 통과하기 전 일반 Chrome 브리지는 `experimental`입니다.
