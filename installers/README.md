# 직접 설치기 계약

이 디렉터리의 설치기는 공개 태그 릴리스가 생긴 뒤 Homebrew나 WinGet을 사용할 수
없는 환경에서 쓰는 보조 경로입니다. 현재는 공개 태그가 없으므로 실제 다운로드
명령을 README의 기본 설치 경로로 노출하지 않습니다.

설치기는 다음 원칙을 지킵니다.

- `latest` 대신 `v0.1.0` 같은 불변 SemVer 태그를 반드시 입력받습니다.
- macOS·Linux의 amd64·arm64와 Windows의 amd64·arm64만 허용합니다.
- GitHub Release의 `checksums.txt`에서 선택한 아카이브의 SHA-256을 확인합니다.
- 아카이브 안에 `coupangctl`, `LICENSE`, `README.md`, `BROWSER_BRIDGE.md` 외의
  파일이 있으면 설치하지 않습니다. Windows 실행 파일명은 `coupangctl.exe`입니다.
- 내려받은 실행 파일의 구조화된 `version` 응답이 요청한 태그와 일치해야 합니다.
- 임시 파일에서 모든 검증을 끝낸 뒤 설치 디렉터리 안의 staging 파일을 원자적으로
  교체합니다. 실패하면 기존 바이너리는 그대로 둡니다.
- 관리자 권한을 자동으로 요청하거나 Gatekeeper·SmartScreen을 우회하지 않습니다.
- 브라우저 프로필, 쿠키, OTP, 주문 DB를 읽거나 이동하거나 삭제하지 않습니다.
- 설치 경로를 바꾸더라도 사용자 데이터는 별도 상태 디렉터리에 그대로 남습니다.

첫 공개 태그 이후 POSIX 사용자는 태그에 포함된 스크립트를 파일로 받은 뒤 내용을
확인하고 실행합니다.

```bash
sh ./install.sh --version v0.1.0
```

기본 설치 위치는 `${XDG_BIN_HOME:-$HOME/.local/bin}`입니다. 다른 사용자 소유
디렉터리는 `--install-dir`로 지정할 수 있습니다. 설치기는 `PATH`를 묵시적으로
수정하지 않습니다.

Windows PowerShell에서는 다음처럼 실행합니다.

```powershell
pwsh -NoProfile -File .\install.ps1 -Version v0.1.0
```

기본 설치 위치는 `%LOCALAPPDATA%\Programs\coupangctl`입니다. 다른 위치는
`-InstallDir`로 지정할 수 있습니다. 일반 사용자 기본 배포에서는 WinGet이 설치,
명령 alias, 업그레이드와 제거를 함께 소유하도록 하는 편이 우선입니다.

두 설치기는 성공할 때 설치 경로나 사용자 이름을 노출하지 않고 다음 shape의 JSON만
출력합니다.

```json
{"name":"coupangctl","version":"0.1.0","status":"installed"}
```

SHA-256은 파일 손상과 manifest 불일치를 확인하지만 게시자 신원을 단독으로
증명하지는 않습니다. 릴리스의 `checksums.txt`와 아카이브에 대한 GitHub provenance
검증 및 운영체제 코드 서명 계약은 [`../RELEASING.md`](../RELEASING.md)를 따릅니다.
