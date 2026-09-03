package packagemanifests

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

const (
	packageIdentifier     = "JungHoonGhae.coupangctl"
	wingetManifestVersion = "1.12.0"
)

var sha256Pattern = regexp.MustCompile(`^[0-9A-Fa-f]{64}$`)

type Request struct {
	Tag                string `json:"tag"`
	SourceSHA256       string `json:"source_sha256"`
	WindowsAMD64SHA256 string `json:"windows_amd64_sha256"`
	WindowsARM64SHA256 string `json:"windows_arm64_sha256"`
}

type File struct {
	Path     string `json:"path"`
	Contents string `json:"-"`
}

type Bundle struct {
	Tag     string `json:"tag"`
	Version string `json:"version"`
	Files   []File `json:"files"`
}

// Render returns the complete package-manager metadata for one immutable
// release without writing files or contacting package repositories.
func Render(request Request) (Bundle, error) {
	if !sha256Pattern.MatchString(request.SourceSHA256) ||
		!sha256Pattern.MatchString(request.WindowsAMD64SHA256) ||
		!sha256Pattern.MatchString(request.WindowsARM64SHA256) {
		return Bundle{}, errors.New("source and Windows artifact hashes must be SHA-256")
	}
	amd64, err := releasecontract.Resolve(request.Tag, "windows", "amd64")
	if err != nil {
		return Bundle{}, err
	}
	arm64, err := releasecontract.Resolve(request.Tag, "windows", "arm64")
	if err != nil {
		return Bundle{}, err
	}
	root := fmt.Sprintf("winget/manifests/j/JungHoonGhae/coupangctl/%s/", amd64.Version)
	return Bundle{
		Tag:     request.Tag,
		Version: amd64.Version,
		Files: []File{
			{Path: "homebrew/Formula/coupangctl.rb", Contents: renderHomebrewFormula(request.Tag, strings.ToLower(request.SourceSHA256))},
			{Path: root + packageIdentifier + ".yaml", Contents: renderWinGetVersion(amd64.Version)},
			{Path: root + packageIdentifier + ".installer.yaml", Contents: renderWinGetInstaller(amd64, arm64, request)},
			{Path: root + packageIdentifier + ".locale.ko-KR.yaml", Contents: renderWinGetLocale(request.Tag, amd64.Version)},
		},
	}, nil
}

func renderHomebrewFormula(tag, sourceSHA256 string) string {
	return fmt.Sprintf(`class Coupangctl < Formula
  desc "Sync and analyze personal Coupang orders locally"
  homepage "https://github.com/JungHoonGhae/coupang-ctl"
  url "https://github.com/JungHoonGhae/coupang-ctl/archive/refs/tags/%s.tar.gz"
  sha256 "%s"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/coupangctl"
  end

  test do
    output = shell_output("#{bin}/coupangctl version")
    assert_match %%Q{"name": "coupangctl"}, output
    assert_match %%Q{"version": "#{version}"}, output
  end
end
`, tag, sourceSHA256)
}

func renderWinGetVersion(version string) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.%s.schema.json
PackageIdentifier: %s
PackageVersion: %s
DefaultLocale: ko-KR
ManifestType: version
ManifestVersion: %s
`, wingetManifestVersion, packageIdentifier, version, wingetManifestVersion)
}

func renderWinGetInstaller(amd64, arm64 releasecontract.Artifact, request Request) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.%s.schema.json
PackageIdentifier: %s
PackageVersion: %s
InstallerType: zip
NestedInstallerType: portable
Scope: user
UpgradeBehavior: install
Commands:
  - coupangctl
Installers:
  - Architecture: x64
    NestedInstallerFiles:
      - RelativeFilePath: coupangctl.exe
        PortableCommandAlias: coupangctl
    InstallerUrl: https://github.com/JungHoonGhae/coupang-ctl/releases/download/%s/%s
    InstallerSha256: %s
  - Architecture: arm64
    NestedInstallerFiles:
      - RelativeFilePath: coupangctl.exe
        PortableCommandAlias: coupangctl
    InstallerUrl: https://github.com/JungHoonGhae/coupang-ctl/releases/download/%s/%s
    InstallerSha256: %s
ManifestType: installer
ManifestVersion: %s
`, wingetManifestVersion, packageIdentifier, amd64.Version,
		amd64.Tag, amd64.ArchiveName, strings.ToUpper(request.WindowsAMD64SHA256),
		arm64.Tag, arm64.ArchiveName, strings.ToUpper(request.WindowsARM64SHA256),
		wingetManifestVersion)
}

func renderWinGetLocale(tag, version string) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.%s.schema.json
PackageIdentifier: %s
PackageVersion: %s
PackageLocale: ko-KR
Publisher: JungHoonGhae
PublisherUrl: https://github.com/JungHoonGhae
PublisherSupportUrl: https://github.com/JungHoonGhae/coupang-ctl/issues
PackageName: coupangctl
PackageUrl: https://github.com/JungHoonGhae/coupang-ctl
License: MIT
LicenseUrl: https://github.com/JungHoonGhae/coupang-ctl/blob/%s/LICENSE
ShortDescription: 내 쿠팡 주문을 로컬에 동기화하고 CLI와 AI로 검색·분석하는 도구
Tags:
  - cli
  - coupang
  - local-first
  - mcp
ManifestType: defaultLocale
ManifestVersion: %s
`, wingetManifestVersion, packageIdentifier, version, tag, wingetManifestVersion)
}
