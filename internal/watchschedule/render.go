package watchschedule

import (
	"errors"
	"fmt"
	"html"
	"runtime"
	"strconv"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func Plan(request core.ProductWatchScheduleRequest) (core.ProductWatchSchedulePlan, error) {
	if err := request.Validate(); err != nil {
		return core.ProductWatchSchedulePlan{}, err
	}
	format := request.Format
	if format == "auto" {
		switch runtime.GOOS {
		case "darwin":
			format = "launchd"
		case "linux":
			format = "systemd"
		case "windows":
			format = "windows_task_scheduler"
		default:
			format = "cron"
		}
	}
	command := []string{request.BinaryPath, "products", "watch-refresh", "--limit", strconv.Itoa(request.Limit), "--stale-hours", strconv.Itoa(request.StaleHours)}
	result := core.ProductWatchSchedulePlan{
		SchemaVersion: core.ProductWatchScheduleSchemaVersion,
		Visibility:    "private_local",
		Format:        format,
		Schedule:      "daily_local_time",
		LocalTime:     request.LocalTime,
		Command:       command,
		Written:       false,
		Artifacts:     []core.ProductWatchArtifact{},
		Activation:    []string{},
		Limitations: []string{
			"the generated task refreshes only due local watch entries and never adds to cart, checks out, orders, or pays",
			"the host must retain a usable coupangctl state directory and network access; protected reads may still require explicit headed reauthentication",
		},
		Provenance: "derived_from_explicit_schedule_parameters",
	}
	var err error
	switch format {
	case "launchd":
		result.Artifacts, result.Activation, err = launchdArtifacts(request, command)
	case "systemd":
		result.Artifacts, result.Activation, err = systemdArtifacts(request, command)
	case "cron":
		result.Artifacts, result.Activation, err = cronArtifacts(request, command)
	case "windows_task_scheduler":
		result.Artifacts, result.Activation, err = windowsArtifacts(request, command)
	default:
		err = errors.New("unsupported resolved scheduler format")
	}
	if err != nil {
		return core.ProductWatchSchedulePlan{}, err
	}
	return result, nil
}

func launchdArtifacts(request core.ProductWatchScheduleRequest, command []string) ([]core.ProductWatchArtifact, []string, error) {
	hour, minute := splitTime(request.LocalTime)
	var arguments strings.Builder
	for _, argument := range command {
		arguments.WriteString("    <string>")
		arguments.WriteString(html.EscapeString(argument))
		arguments.WriteString("</string>\n")
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.coupangctl.price-watch</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>%s</integer><key>Minute</key><integer>%s</integer></dict>
  <key>RunAtLoad</key><false/>
</dict>
</plist>
`, arguments.String(), hour, minute)
	return []core.ProductWatchArtifact{{Filename: "com.coupangctl.price-watch.plist", Mode: "0600", Content: content}},
		[]string{"copy the plist to the user's LaunchAgents directory", "run launchctl bootstrap for that plist"}, nil
}

func systemdArtifacts(request core.ProductWatchScheduleRequest, command []string) ([]core.ProductWatchArtifact, []string, error) {
	quoted := make([]string, len(command))
	for index, argument := range command {
		quoted[index] = strconv.Quote(argument)
	}
	service := fmt.Sprintf(`[Unit]
Description=Refresh coupangctl price watchlist

[Service]
Type=oneshot
ExecStart=%s
`, strings.Join(quoted, " "))
	timer := fmt.Sprintf(`[Unit]
Description=Run coupangctl price watchlist refresh daily

[Timer]
OnCalendar=*-*-* %s:00
Persistent=true

[Install]
WantedBy=timers.target
`, request.LocalTime)
	return []core.ProductWatchArtifact{
		{Filename: "coupangctl-price-watch.service", Mode: "0600", Content: service},
		{Filename: "coupangctl-price-watch.timer", Mode: "0600", Content: timer},
	}, []string{"copy both files to the user's systemd directory", "run systemctl --user daemon-reload", "run systemctl --user enable --now coupangctl-price-watch.timer"}, nil
}

func cronArtifacts(request core.ProductWatchScheduleRequest, command []string) ([]core.ProductWatchArtifact, []string, error) {
	hour, minute := splitTime(request.LocalTime)
	quoted := make([]string, len(command))
	for index, argument := range command {
		quoted[index] = shellQuote(argument)
	}
	content := fmt.Sprintf("%s %s * * * %s\n", minute, hour, strings.Join(quoted, " "))
	return []core.ProductWatchArtifact{{Filename: "coupangctl-price-watch.cron", Mode: "0600", Content: content}},
		[]string{"review the generated line", "install it with the target user's crontab"}, nil
}

func windowsArtifacts(request core.ProductWatchScheduleRequest, command []string) ([]core.ProductWatchArtifact, []string, error) {
	arguments := make([]string, 0, len(command)-1)
	for _, argument := range command[1:] {
		arguments = append(arguments, windowsArgument(argument))
	}
	content := fmt.Sprintf(`$action = New-ScheduledTaskAction -Execute '%s' -Argument '%s'
$trigger = New-ScheduledTaskTrigger -Daily -At '%s'
Register-ScheduledTask -TaskName 'coupangctl-price-watch' -Action $action -Trigger $trigger -Description 'Refresh coupangctl price watchlist'
`, powershellQuote(command[0]), powershellQuote(strings.Join(arguments, " ")), request.LocalTime)
	return []core.ProductWatchArtifact{{Filename: "install-coupangctl-price-watch.ps1", Mode: "0600", Content: content}},
		[]string{"review the PowerShell script", "run it as the Windows user that owns the coupangctl state directory"}, nil
}

func splitTime(value string) (string, string) {
	parts := strings.SplitN(value, ":", 2)
	return parts[0], parts[1]
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func powershellQuote(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func windowsArgument(value string) string {
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
