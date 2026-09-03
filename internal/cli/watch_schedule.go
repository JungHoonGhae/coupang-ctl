package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/watchschedule"
)

func runProductWatchSchedule(args []string, stdout io.Writer, executable string) error {
	flags := newFlagSet("products watch-schedule")
	format := flags.String("format", "auto", "auto, launchd, systemd, cron, or windows_task_scheduler")
	localTime := flags.String("at", "03:00", "daily local time in HH:MM")
	limit := flags.Int("limit", 20, "maximum due watch entries per run")
	staleHours := flags.Int("stale-hours", 24, "minimum hours since the last check")
	binaryPath := flags.String("binary", executable, "absolute coupangctl binary path used by the scheduler")
	outputDir := flags.String("output-dir", "", "optional directory for new scheduler files")
	const commandUsage = "usage: coupangctl products watch-schedule [--format auto|launchd|systemd|cron|windows_task_scheduler] [--at HH:MM] [--limit N] [--stale-hours N] [--binary ABSOLUTE_PATH] [--output-dir DIR]"
	if err := parseFlags(flags, args, commandUsage); err != nil {
		return err
	}
	plan, err := watchschedule.Plan(core.ProductWatchScheduleRequest{
		Format: *format, LocalTime: *localTime, Limit: *limit, StaleHours: *staleHours, BinaryPath: *binaryPath,
	})
	if err != nil {
		return err
	}
	if *outputDir != "" {
		if err := writeWatchScheduleArtifacts(&plan, *outputDir); err != nil {
			return err
		}
	}
	return writeJSON(stdout, plan)
}

func writeWatchScheduleArtifacts(plan *core.ProductWatchSchedulePlan, outputDir string) error {
	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve scheduler output directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return fmt.Errorf("create scheduler output directory: %w", err)
	}
	written := []string{}
	rollback := func() {
		for _, path := range written {
			_ = os.Remove(path)
		}
	}
	for index := range plan.Artifacts {
		artifact := &plan.Artifacts[index]
		if artifact.Filename == "" || filepath.Base(artifact.Filename) != artifact.Filename {
			rollback()
			return errors.New("invalid scheduler artifact filename")
		}
		path := filepath.Join(absolute, artifact.Filename)
		file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			rollback()
			return fmt.Errorf("create scheduler artifact: %w", openErr)
		}
		_, writeErr := io.WriteString(file, artifact.Content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			rollback()
			if writeErr != nil {
				return fmt.Errorf("write scheduler artifact: %w", writeErr)
			}
			return fmt.Errorf("close scheduler artifact: %w", closeErr)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			_ = os.Remove(path)
			rollback()
			return fmt.Errorf("secure scheduler artifact: %w", err)
		}
		written = append(written, path)
		artifact.WrittenPath = path
	}
	plan.OutputDirectory = absolute
	plan.Written = true
	return nil
}
