#!/bin/sh

set -eu

fail() {
  printf '%s\n' "coupangctl installer: $1" >&2
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
      [ "$#" -ge 2 ] || fail '--version requires a value'
      release_tag=$2
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail '--install-dir requires a value'
      install_dir=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ -n "$release_tag" ] || fail '--version is required; latest is intentionally unsupported'
printf '%s\n' "$release_tag" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
  fail 'version must be an immutable semantic release tag such as v0.1.0'

goos=${COUPANGCTL_INSTALL_GOOS:-}
if [ -z "$goos" ]; then
  case "$(uname -s)" in
    Darwin) goos=darwin ;;
    Linux) goos=linux ;;
    *) fail 'supported operating systems are macOS and Linux; use install.ps1 on Windows' ;;
  esac
fi

goarch=${COUPANGCTL_INSTALL_GOARCH:-}
if [ -z "$goarch" ]; then
  case "$(uname -m)" in
    x86_64|amd64) goarch=amd64 ;;
    arm64|aarch64) goarch=arm64 ;;
    *) fail 'supported architectures are amd64 and arm64' ;;
  esac
fi

case "$goos/$goarch" in
  darwin/amd64|darwin/arm64|linux/amd64|linux/arm64) ;;
  *) fail "unsupported release target: $goos/$goarch" ;;
esac

command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'tar is required'

base_url=${COUPANGCTL_INSTALL_BASE_URL:-https://github.com/JungHoonGhae/coupang-ctl/releases/download}
case "$base_url" in
  https://*) allowed_protocol='=https' ;;
  http://127.0.0.1:*|http://localhost:*) allowed_protocol='=http' ;;
  *) fail 'release base URL must use HTTPS (loopback HTTP is test-only)' ;;
esac

if [ -z "$install_dir" ]; then
  [ -n "${HOME:-}" ] || fail 'HOME is required when --install-dir is omitted'
  install_dir=${XDG_BIN_HOME:-"$HOME/.local/bin"}
fi

asset_version=${release_tag#v}
asset_name="coupangctl_${asset_version}_${goos}_${goarch}.tar.gz"
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/coupangctl-install.XXXXXX") || fail 'could not create a temporary directory'
staging_path=''
cleanup() {
  if [ -n "$staging_path" ]; then
    rm -f -- "$staging_path"
  fi
  rm -rf -- "$work_dir"
}
trap cleanup EXIT HUP INT TERM

archive_path="$work_dir/$asset_name"
checksums_path="$work_dir/checksums.txt"
curl --fail --location --silent --show-error --proto "$allowed_protocol" \
  "$base_url/$release_tag/checksums.txt" --output "$checksums_path"
curl --fail --location --silent --show-error --proto "$allowed_protocol" \
  "$base_url/$release_tag/$asset_name" --output "$archive_path"

expected_digest=$(awk -v name="$asset_name" '
  $2 == name || $2 == "*" name {
    if (found) exit 2
    print tolower($1)
    found = 1
  }
  END { if (!found) exit 1 }
' "$checksums_path") || fail 'release checksum entry is missing or duplicated'
printf '%s\n' "$expected_digest" | LC_ALL=C grep -Eq '^[0-9a-f]{64}$' || fail 'release checksum is not SHA-256'

if command -v sha256sum >/dev/null 2>&1; then
  actual_digest=$(sha256sum "$archive_path" | awk '{ print tolower($1) }')
elif command -v shasum >/dev/null 2>&1; then
  actual_digest=$(shasum -a 256 "$archive_path" | awk '{ print tolower($1) }')
else
  fail 'sha256sum or shasum is required'
fi
[ "$actual_digest" = "$expected_digest" ] || fail 'archive checksum mismatch'

entries_path="$work_dir/archive-entries.txt"
tar -tzf "$archive_path" >"$entries_path" || fail 'could not inspect the release archive'
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
' "$entries_path" || fail 'release archive contains unexpected entries'

extract_dir="$work_dir/extract"
mkdir "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir" -- coupangctl || fail 'could not extract coupangctl from the release archive'
candidate="$extract_dir/coupangctl"
[ ! -L "$candidate" ] || fail 'release executable must not be a symbolic link'
[ -f "$candidate" ] || fail 'release executable is missing or not a regular file'
chmod 0755 "$candidate"

version_output=$("$candidate" version) || fail 'downloaded executable did not report its version'
normalized_version_output=$(printf '%s' "$version_output" | tr -d '[:space:]')
printf '%s\n' "$normalized_version_output" | grep -Fq '"name":"coupangctl"' ||
  fail 'downloaded executable identity does not match coupangctl'
printf '%s\n' "$normalized_version_output" | grep -Fq "\"version\":\"$asset_version\"" ||
  fail 'downloaded executable version does not match the requested release'

if [ -L "$install_dir" ]; then
  fail 'install directory must not be a symbolic link'
fi
if [ -e "$install_dir" ] && [ ! -d "$install_dir" ]; then
  fail 'install path exists and is not a directory'
fi
mkdir -p -- "$install_dir"
[ ! -L "$install_dir" ] || fail 'install directory must not be a symbolic link'

destination="$install_dir/coupangctl"
if [ -L "$destination" ]; then
  fail 'refusing to replace a symbolic-link destination'
fi
if [ -e "$destination" ] && [ ! -f "$destination" ]; then
  fail 'refusing to replace a non-regular destination'
fi

staging_path=$(mktemp "$install_dir/.coupangctl.new.XXXXXX") || fail 'could not stage the executable in the install directory'
cp "$candidate" "$staging_path"
chmod 0755 "$staging_path"
mv -f -- "$staging_path" "$destination"
staging_path=''

printf '{"name":"coupangctl","version":"%s","status":"installed"}\n' "$asset_version"
