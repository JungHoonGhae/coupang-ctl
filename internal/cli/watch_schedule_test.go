package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestWatchScheduleCommandRendersWithoutOpeningBrowser(t *testing.T) {
	var stdout bytes.Buffer
	if err := runProductWatchSchedule([]string{"--format", "systemd", "--at", "04:20", "--limit", "7"}, &stdout, "/opt/coupangctl"); err != nil {
		t.Fatal(err)
	}
	var got core.ProductWatchSchedulePlan
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Format != "systemd" || got.LocalTime != "04:20" || got.Written || len(got.Artifacts) != 2 {
		t.Fatalf("unexpected schedule plan: %#v", got)
	}
	if len(got.Command) < 3 || got.Command[0] != "/opt/coupangctl" || got.Command[2] != "watch-refresh" {
		t.Fatalf("unexpected scheduled command: %#v", got.Command)
	}
}

func TestWatchScheduleWritesPrivateNewFilesAndNeverOverwrites(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "schedule")
	var stdout bytes.Buffer
	args := []string{"--format", "cron", "--output-dir", outputDir}
	if err := runProductWatchSchedule(args, &stdout, "/opt/coupangctl"); err != nil {
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
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode = %o, want 600", info.Mode().Perm())
	}
	stdout.Reset()
	if err := runProductWatchSchedule(args, &stdout, "/opt/coupangctl"); err == nil {
		t.Fatal("expected an existing scheduler artifact to reject overwrite")
	}
}

func TestWatchScheduleRunDoesNotRequireBrowserOrStateDirectory(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-path-would-fail-normal-startup")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{
		"products", "watch-schedule", "--format", "cron", "--binary", "/opt/coupangctl",
	}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestWatchScheduleRejectsRelativeBinaryOverride(t *testing.T) {
	var stdout bytes.Buffer
	if err := runProductWatchSchedule([]string{"--format", "cron", "--binary", "relative/coupangctl"}, &stdout, "/opt/coupangctl"); err == nil {
		t.Fatal("expected a relative binary path to fail")
	}
}
