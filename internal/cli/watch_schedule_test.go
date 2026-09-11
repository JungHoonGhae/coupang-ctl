package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestWatchScheduleCommandRendersWithoutOpeningBrowser(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "coupangctl")
	var stdout bytes.Buffer
	if err := runProductWatchSchedule([]string{"--format", "systemd", "--at", "04:20", "--limit", "7"}, &stdout, binaryPath); err != nil {
		t.Fatal(err)
	}
	var got core.ProductWatchSchedulePlan
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Format != "systemd" || got.LocalTime != "04:20" || got.Written || len(got.Artifacts) != 2 {
		t.Fatalf("unexpected schedule plan: %#v", got)
	}
	if len(got.Command) < 3 || got.Command[0] != binaryPath || got.Command[2] != "watch-refresh" {
		t.Fatalf("unexpected scheduled command: %#v", got.Command)
	}
}

func TestWatchScheduleWritesPrivateNewFilesAndNeverOverwrites(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "coupangctl")
	outputDir := filepath.Join(t.TempDir(), "schedule")
	var stdout bytes.Buffer
	args := []string{"--format", "cron", "--output-dir", outputDir}
	if err := runProductWatchSchedule(args, &stdout, binaryPath); err != nil {
		t.Fatal(err)
	}
	var got core.ProductWatchSchedulePlan
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Written || len(got.Artifacts) != 1 || got.Artifacts[0].WrittenPath == "" {
		t.Fatalf("unexpected written plan: %#v", got)
	}
	info, err := os.Stat(got.Artifacts[0].WrittenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("scheduler artifact is not a regular file")
	}
	// Windows FileMode does not report DACL permissions. Retain the POSIX
	// owner-only check without treating its Windows bits as ACL evidence.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode = %o, want 600", info.Mode().Perm())
	}
	before, err := os.ReadFile(got.Artifacts[0].WrittenPath)
	if err != nil || string(before) != got.Artifacts[0].Content {
		t.Fatal("scheduler artifact contents do not match the plan")
	}
	stdout.Reset()
	if err := runProductWatchSchedule(args, &stdout, binaryPath); err == nil {
		t.Fatal("expected an existing scheduler artifact to reject overwrite")
	}
	after, err := os.ReadFile(got.Artifacts[0].WrittenPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing scheduler artifact changed")
	}
}

func TestWatchScheduleRunDoesNotRequireBrowserOrStateDirectory(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "coupangctl")
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-path-would-fail-normal-startup")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"products", "watch-schedule", "--format", "cron", "--binary", binaryPath,
	}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestWatchScheduleRejectsRelativeBinaryOverride(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "coupangctl")
	var stdout bytes.Buffer
	if err := runProductWatchSchedule([]string{"--format", "cron", "--binary", "relative/coupangctl"}, &stdout, binaryPath); err == nil {
		t.Fatal("expected a relative binary path to fail")
	}
}
