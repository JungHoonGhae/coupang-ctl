package packagemanifests

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

type Report struct {
	Tag     string   `json:"tag"`
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

// WriteNew materializes a rendered bundle into one new directory. It never
// merges with or replaces an existing output directory.
func WriteNew(outputDir string, bundle Bundle) (Report, error) {
	if outputDir == "" || !filepath.IsAbs(outputDir) {
		return Report{}, errors.New("output directory must be an absolute new path")
	}
	cleanOutput := filepath.Clean(outputDir)
	if cleanOutput == filepath.VolumeName(cleanOutput)+string(filepath.Separator) {
		return Report{}, errors.New("output directory must not be a filesystem root")
	}
	if _, err := os.Lstat(cleanOutput); err == nil {
		return Report{}, errors.New("output directory already exists")
	} else if !os.IsNotExist(err) {
		return Report{}, fmt.Errorf("inspect output directory: %w", err)
	}
	parentInfo, err := os.Lstat(filepath.Dir(cleanOutput))
	if err != nil || !parentInfo.IsDir() {
		return Report{}, errors.New("output parent must be an existing directory")
	}
	if err := os.Mkdir(cleanOutput, 0o700); err != nil {
		return Report{}, fmt.Errorf("create output directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(cleanOutput)
		}
	}()

	report := Report{Tag: bundle.Tag, Version: bundle.Version, Files: make([]string, 0, len(bundle.Files))}
	seen := make(map[string]struct{}, len(bundle.Files))
	for _, file := range bundle.Files {
		cleanRelative := path.Clean(file.Path)
		if cleanRelative == "." || cleanRelative != file.Path || path.IsAbs(cleanRelative) || cleanRelative == ".." || len(cleanRelative) >= 3 && cleanRelative[:3] == "../" {
			return Report{}, fmt.Errorf("unsafe generated path: %q", file.Path)
		}
		if _, duplicate := seen[cleanRelative]; duplicate {
			return Report{}, fmt.Errorf("duplicate generated path: %q", file.Path)
		}
		seen[cleanRelative] = struct{}{}
		destination := filepath.Join(cleanOutput, filepath.FromSlash(cleanRelative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return Report{}, fmt.Errorf("create generated directory: %w", err)
		}
		handle, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return Report{}, fmt.Errorf("create generated file: %w", err)
		}
		_, writeErr := handle.WriteString(file.Contents)
		closeErr := handle.Close()
		if writeErr != nil {
			return Report{}, fmt.Errorf("write generated file: %w", writeErr)
		}
		if closeErr != nil {
			return Report{}, fmt.Errorf("close generated file: %w", closeErr)
		}
		report.Files = append(report.Files, cleanRelative)
	}
	committed = true
	return report, nil
}
