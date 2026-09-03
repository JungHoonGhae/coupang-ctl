package watchschedule

import (
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestPlanRendersDailyHeadlessRefreshForEverySupportedScheduler(t *testing.T) {
	for _, format := range []string{"launchd", "systemd", "cron", "windows_task_scheduler"} {
		t.Run(format, func(t *testing.T) {
			got, err := Plan(core.ProductWatchScheduleRequest{
				Format: format, LocalTime: "03:15", Limit: 20, StaleHours: 24, BinaryPath: "/opt/coupang ctl/coupangctl",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Format != format || got.Schedule != "daily_local_time" || got.Written || len(got.Artifacts) == 0 {
				t.Fatalf("unexpected plan: %#v", got)
			}
			joined := ""
			for _, artifact := range got.Artifacts {
				joined += artifact.Content
				if artifact.Mode != "0600" || artifact.Filename == "" {
					t.Fatalf("unsafe artifact: %#v", artifact)
				}
			}
			for _, required := range []string{"watch-refresh", "--limit", "20", "--stale-hours", "24"} {
				if !strings.Contains(joined, required) {
					t.Fatalf("%s artifact omitted %q: %s", format, required, joined)
				}
			}
			for _, forbidden := range []string{"cart-add", "checkout", "payment"} {
				if strings.Contains(strings.ToLower(joined), forbidden) {
					t.Fatalf("%s artifact crossed commerce boundary: %s", format, joined)
				}
			}
			if strings.Contains(joined, "-Force") {
				t.Fatalf("%s artifact may overwrite an existing scheduler entry: %s", format, joined)
			}
		})
	}
}

func TestPlanRejectsUnsafeOrAmbiguousInputs(t *testing.T) {
	requests := []core.ProductWatchScheduleRequest{
		{Format: "unknown", LocalTime: "03:00", Limit: 10, StaleHours: 24, BinaryPath: "/bin/coupangctl"},
		{Format: "cron", LocalTime: "3am", Limit: 10, StaleHours: 24, BinaryPath: "/bin/coupangctl"},
		{Format: "cron", LocalTime: "03:00", Limit: 0, StaleHours: 24, BinaryPath: "/bin/coupangctl"},
		{Format: "cron", LocalTime: "03:00", Limit: 10, StaleHours: 24, BinaryPath: "relative"},
		{Format: "cron", LocalTime: "03:00", Limit: 10, StaleHours: 24, BinaryPath: "/bin/coupangctl\nmalicious"},
	}
	for _, request := range requests {
		if _, err := Plan(request); err == nil {
			t.Fatalf("expected invalid request to fail: %#v", request)
		}
	}
}
