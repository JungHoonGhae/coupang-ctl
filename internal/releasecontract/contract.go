package releasecontract

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const ProjectName = "coupangctl"

var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type platform struct {
	goos           string
	goarch         string
	archiveFormat  string
	executableName string
}

var supportedPlatforms = []platform{
	{goos: "darwin", goarch: "amd64", archiveFormat: "tar.gz", executableName: ProjectName},
	{goos: "darwin", goarch: "arm64", archiveFormat: "tar.gz", executableName: ProjectName},
	{goos: "linux", goarch: "amd64", archiveFormat: "tar.gz", executableName: ProjectName},
	{goos: "linux", goarch: "arm64", archiveFormat: "tar.gz", executableName: ProjectName},
	{goos: "windows", goarch: "amd64", archiveFormat: "zip", executableName: ProjectName + ".exe"},
	{goos: "windows", goarch: "arm64", archiveFormat: "zip", executableName: ProjectName + ".exe"},
}

// Artifact is the canonical release identity for one supported platform.
// Tag includes the leading v used by GitHub Releases; Version is the exact
// value embedded by GoReleaser and used in archive names.
type Artifact struct {
	Tag              string `json:"tag"`
	Version          string `json:"version"`
	GOOS             string `json:"goos"`
	GOARCH           string `json:"goarch"`
	ArchiveFormat    string `json:"archive_format"`
	ArchiveName      string `json:"archive_name"`
	ExecutableName   string `json:"executable_name"`
	ReleaseAssetPath string `json:"release_asset_path"`
}

// Resolve returns the complete canonical artifact identity for one immutable
// release tag and platform. It accepts only the targets produced by the
// release workflow.
func Resolve(tag, goos, goarch string) (Artifact, error) {
	if !releaseTagPattern.MatchString(tag) {
		return Artifact{}, errors.New("release tag must be semantic and start with v")
	}
	var selected platform
	found := false
	for _, candidate := range supportedPlatforms {
		if candidate.goos == goos && candidate.goarch == goarch {
			selected = candidate
			found = true
			break
		}
	}
	if !found {
		return Artifact{}, fmt.Errorf("unsupported release target: %s/%s", goos, goarch)
	}
	version := strings.TrimPrefix(tag, "v")
	archiveName := fmt.Sprintf("%s_%s_%s_%s.%s", ProjectName, version, goos, goarch, selected.archiveFormat)
	return Artifact{
		Tag:              tag,
		Version:          version,
		GOOS:             goos,
		GOARCH:           goarch,
		ArchiveFormat:    selected.archiveFormat,
		ArchiveName:      archiveName,
		ExecutableName:   selected.executableName,
		ReleaseAssetPath: tag + "/" + archiveName,
	}, nil
}

// Artifacts returns every supported release artifact in stable platform order.
func Artifacts(tag string) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(supportedPlatforms))
	for _, target := range supportedPlatforms {
		artifact, err := Resolve(tag, target.goos, target.goarch)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}
