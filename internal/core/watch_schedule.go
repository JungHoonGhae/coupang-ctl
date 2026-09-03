package core

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

const ProductWatchScheduleSchemaVersion = 1

var dailyLocalTimePattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

type ProductWatchScheduleRequest struct {
	Format     string `json:"format"`
	LocalTime  string `json:"local_time"`
	Limit      int    `json:"limit"`
	StaleHours int    `json:"stale_hours"`
	BinaryPath string `json:"binary_path"`
}

func (r ProductWatchScheduleRequest) Validate() error {
	switch r.Format {
	case "auto", "launchd", "systemd", "cron", "windows_task_scheduler":
	default:
		return errors.New("format must be auto, launchd, systemd, cron, or windows_task_scheduler")
	}
	if !dailyLocalTimePattern.MatchString(r.LocalTime) {
		return errors.New("local_time must use 24-hour HH:MM")
	}
	if r.Limit < 1 || r.Limit > 50 {
		return errors.New("limit must be between 1 and 50")
	}
	if r.StaleHours < 1 || r.StaleHours > 720 {
		return errors.New("stale_hours must be between 1 and 720")
	}
	if !filepath.IsAbs(r.BinaryPath) || strings.ContainsAny(r.BinaryPath, "\x00\r\n") {
		return errors.New("binary_path must be an absolute single-line path")
	}
	return nil
}

type ProductWatchSchedulePlan struct {
	SchemaVersion   int                    `json:"schema_version"`
	Visibility      string                 `json:"visibility"`
	Format          string                 `json:"format"`
	Schedule        string                 `json:"schedule"`
	LocalTime       string                 `json:"local_time"`
	Command         []string               `json:"command"`
	Artifacts       []ProductWatchArtifact `json:"artifacts"`
	OutputDirectory string                 `json:"output_directory,omitempty"`
	Written         bool                   `json:"written"`
	Activation      []string               `json:"activation"`
	Limitations     []string               `json:"limitations"`
	Provenance      string                 `json:"provenance"`
}

type ProductWatchArtifact struct {
	Filename    string `json:"filename"`
	Mode        string `json:"mode"`
	Content     string `json:"content"`
	WrittenPath string `json:"written_path,omitempty"`
}
