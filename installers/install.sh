#!/bin/sh

set -eu

fail() {
  code=$1
  message=$2
  printf '{"error":{"code":"%s","message":"%s"}}\n' "$code" "$message" >&2
  exit 1
}

usage() {
  printf '%s\n' 'usage: install.sh --version vX.Y.Z [--install-dir DIRECTORY]'
}

release_tag=''
install_dir=${COUPANGCTL_INSTALL_DIR:-}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail 'missing_argument' '--version requires a value'
      release_tag=$2
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail 'missing_argument' '--install-dir requires a value'
      install_dir=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail 'unknown_argument' 'unknown installer argument'
      ;;
  esac
done

[ -n "$release_tag" ] || fail 'missing_version' '--version is required; latest is intentionally unsupported'
printf '%s\n' "$release_tag" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
  fail 'invalid_version' 'version must be an immutable semantic release tag such as v0.1.0'

goos=${COUPANGCTL_INSTALL_GOOS:-}
if [ -z "$goos" ]; then
  case "$(uname -s)" in
    Darwin) goos=darwin ;;
    Linux) goos=linux ;;
    *) fail 'unsupported_operating_system' 'supported operating systems are macOS and Linux; use install.ps1 on Windows' ;;
  esac
fi

goarch=${COUPANGCTL_INSTALL_GOARCH:-}
if [ -z "$goarch" ]; then
  case "$(uname -m)" in
    x86_64|amd64) goarch=amd64 ;;
    arm64|aarch64) goarch=arm64 ;;
    *) fail 'unsupported_architecture' 'supported architectures are amd64 and arm64' ;;
  esac
fi

case "$goos/$goarch" in
  darwin/amd64|darwin/arm64|linux/amd64|linux/arm64) ;;
  *) fail 'unsupported_target' 'supported targets are macOS and Linux on amd64 or arm64' ;;
esac

command -v curl >/dev/null 2>&1 || fail 'missing_dependency' 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'missing_dependency' 'tar is required'

base_url=${COUPANGCTL_INSTALL_BASE_URL:-https://github.com/JungHoonGhae/coupang-ctl/releases/download}
case "$base_url" in
  https://*) allowed_protocol='=https' ;;
  http://127.0.0.1:*|http://localhost:*) allowed_protocol='=http' ;;
  *) fail 'invalid_release_source' 'release base URL must use HTTPS; loopback HTTP is test-only' ;;
esac

if [ -z "$install_dir" ]; then
  [ -n "${HOME:-}" ] || fail 'install_directory_unavailable' 'HOME is required when --install-dir is omitted'
  install_dir=${XDG_BIN_HOME:-"$HOME/.local/bin"}
fi

asset_version=${release_tag#v}
asset_name="coupangctl_${asset_version}_${goos}_${goarch}.tar.gz"
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/coupangctl-install.XXXXXX" 2>/dev/null) || fail 'temporary_directory_failed' 'could not create a temporary directory'
staging_path=''
cleanup() {
  if [ -n "$staging_path" ]; then
    rm -f -- "$staging_path" 2>/dev/null || :
  fi
  rm -rf -- "$work_dir" 2>/dev/null || :
}
trap cleanup EXIT HUP INT TERM

archive_path="$work_dir/$asset_name"
checksums_path="$work_dir/checksums.txt"
curl --fail --location --silent --show-error --proto "$allowed_protocol" \
  "$base_url/$release_tag/checksums.txt" --output "$checksums_path" 2>/dev/null ||
  fail 'download_failed' 'could not download the release checksum manifest'
curl --fail --location --silent --show-error --proto "$allowed_protocol" \
  "$base_url/$release_tag/$asset_name" --output "$archive_path" 2>/dev/null ||
  fail 'download_failed' 'could not download the requested release archive'

expected_digest=$(awk -v name="$asset_name" '
  $2 == name || $2 == "*" name {
    if (found) exit 2
    print tolower($1)
    found = 1
  }
  END { if (!found) exit 1 }
' "$checksums_path" 2>/dev/null) || fail 'invalid_checksum_manifest' 'release checksum entry is missing or duplicated'
printf '%s\n' "$expected_digest" | LC_ALL=C grep -Eq '^[0-9a-f]{64}$' || fail 'invalid_checksum_manifest' 'release checksum is not SHA-256'

if command -v sha256sum >/dev/null 2>&1; then
  actual_digest=$(sha256sum "$archive_path" 2>/dev/null | awk '{ print tolower($1) }')
elif command -v shasum >/dev/null 2>&1; then
  actual_digest=$(shasum -a 256 "$archive_path" 2>/dev/null | awk '{ print tolower($1) }')
else
  fail 'missing_dependency' 'sha256sum or shasum is required'
fi
[ "$actual_digest" = "$expected_digest" ] || fail 'checksum_mismatch' 'archive checksum mismatch'

entries_path="$work_dir/archive-entries.txt"
tar -tzf "$archive_path" >"$entries_path" 2>/dev/null || fail 'invalid_archive' 'could not inspect the release archive'
awk '
  BEGIN {
    expected["coupangctl"] = 1
    expected["LICENSE"] = 1
    expected["README.md"] = 1
    expected["BROWSER_BRIDGE.md"] = 1
  }
  {
    if (!($0 in expected) || seen[$0]) exit 1
    seen[$0] = 1
  }
  END {
    for (name in expected) {
      if (!(name in seen)) exit 1
    }
  }
' "$entries_path" 2>/dev/null || fail 'unexpected_archive_content' 'release archive contains unexpected entries'

extract_dir="$work_dir/extract"
mkdir "$extract_dir" 2>/dev/null || fail 'temporary_directory_failed' 'could not prepare the temporary extraction directory'
tar -xzf "$archive_path" -C "$extract_dir" -- coupangctl 2>/dev/null || fail 'invalid_archive' 'could not extract coupangctl from the release archive'
candidate="$extract_dir/coupangctl"
[ ! -L "$candidate" ] || fail 'invalid_executable' 'release executable must not be a symbolic link'
[ -f "$candidate" ] || fail 'invalid_executable' 'release executable is missing or not a regular file'
chmod 0755 "$candidate" 2>/dev/null || fail 'invalid_executable' 'could not mark the release executable as executable'

version_output=$("$candidate" version 2>/dev/null) || fail 'invalid_executable' 'downloaded executable did not report its version'
normalized_version_output=$(printf '%s' "$version_output" | tr -d '[:space:]')
printf '%s\n' "$normalized_version_output" | grep -Fq '"name":"coupangctl"' ||
  fail 'version_mismatch' 'downloaded executable identity does not match coupangctl'
printf '%s\n' "$normalized_version_output" | grep -Fq "\"version\":\"$asset_version\"" ||
  fail 'version_mismatch' 'downloaded executable version does not match the requested release'

if [ -L "$install_dir" ]; then
  fail 'unsafe_install_directory' 'install directory must not be a symbolic link'
fi
if [ -e "$install_dir" ] && [ ! -d "$install_dir" ]; then
  fail 'unsafe_install_directory' 'install path exists and is not a directory'
fi
mkdir -p -- "$install_dir" 2>/dev/null || fail 'install_directory_failed' 'could not create the install directory'
[ ! -L "$install_dir" ] || fail 'unsafe_install_directory' 'install directory must not be a symbolic link'

destination="$install_dir/coupangctl"
if [ -L "$destination" ]; then
  fail 'unsafe_destination' 'refusing to replace a symbolic-link destination'
fi
if [ -e "$destination" ] && [ ! -f "$destination" ]; then
  fail 'unsafe_destination' 'refusing to replace a non-regular destination'
fi

staging_path=$(mktemp "$install_dir/.coupangctl.new.XXXXXX" 2>/dev/null) || fail 'install_failed' 'could not stage the executable in the install directory'
cp "$candidate" "$staging_path" 2>/dev/null || fail 'install_failed' 'could not copy the executable into the install directory'
chmod 0755 "$staging_path" 2>/dev/null || fail 'install_failed' 'could not mark the staged executable as executable'
mv -f -- "$staging_path" "$destination" 2>/dev/null || fail 'install_failed' 'could not atomically replace the installed executable'
staging_path=''

printf '{"name":"coupangctl","version":"%s","status":"installed"}\n' "$asset_version"
