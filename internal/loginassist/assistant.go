package loginassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrUnsupported = errors.New("login assistance is unsupported on this platform")
var ErrDedicatedBrowserNotRunning = errors.New("dedicated browser is not running")
var ErrAccessibilityPermissionRequired = errors.New("accessibility permission required")
var ErrResendControlNotFound = errors.New("OTP resend control not found")
var ErrResendOutcomeUnverified = errors.New("OTP resend outcome unverified")

type Assistant struct {
	profileDir     string
	findBrowserPID func(context.Context, string) (int, error)
	pressResend    func(context.Context, int) error
}

func New(profileDir string) *Assistant {
	return &Assistant{
		profileDir:     filepath.Clean(profileDir),
		findBrowserPID: findDedicatedChromePID,
		pressResend:    pressMacOSResend,
	}
}

func (a *Assistant) Resend(ctx context.Context) (core.OTPResendResult, error) {
	if runtime.GOOS != "darwin" {
		return core.OTPResendResult{}, ErrUnsupported
	}
	pid, err := a.findBrowserPID(ctx, a.profileDir)
	if err != nil {
		return core.OTPResendResult{}, err
	}
	if err := a.pressResend(ctx, pid); err != nil {
		return core.OTPResendResult{}, err
	}
	return core.OTPResendResult{Requested: true, UITransitionVerified: true, DeliveryVerified: false}, nil
}

func findDedicatedChromePID(ctx context.Context, profileDir string) (int, error) {
	output, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return 0, errors.New("inspect dedicated browser process")
	}
	needle := "--user-data-dir=" + profileDir
	var found int
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, "Google Chrome") || strings.Contains(line, "Google Chrome Helper") || !strings.Contains(line, needle) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, parseErr := strconv.Atoi(fields[0])
		if parseErr != nil {
			continue
		}
		if found != 0 && found != pid {
			return 0, errors.New("multiple dedicated browser processes found")
		}
		found = pid
	}
	if found == 0 {
		return 0, ErrDedicatedBrowserNotRunning
	}
	return found, nil
}

func pressMacOSResend(ctx context.Context, pid int) error {
	const script = `function usable(node) {
  try {
    const size = node.size();
    return node.enabled() && size[0] > 0 && size[1] > 0;
  } catch (_) {
    return false;
  }
}
function find(node) {
  let children = [];
  try { children = node.uiElements(); } catch (_) {}
  for (const child of children) {
    let name = "";
    try { name = child.name(); } catch (_) {}
    if ((name === "재발송" || name === "인증번호 발송") && usable(child)) return child;
    const nested = find(child);
    if (nested) return nested;
  }
  return null;
}
function run(argv) {
  const pid = Number(argv[0]);
  const systemEvents = Application("System Events");
  const processes = systemEvents.processes.whose({unixId: pid})();
  if (processes.length !== 1 || Number(processes[0].unixId()) !== pid) throw new Error("exact browser targeting unsupported");
  const windows = processes[0].windows();
  if (windows.length === 0) throw new Error("dedicated browser window not found");
  const control = find(windows[0]);
  if (!control) throw new Error("resend control not found");
  control.click();
  delay(2);
  const postWindows = processes[0].windows();
  if (postWindows.length === 0 || !find(postWindows[0])) throw new Error("resend outcome not verified");
  return JSON.stringify({requested: true, ui_verified: true});
}`
	command := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", script, strconv.Itoa(pid))
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.ToLower(string(output))
		switch {
		case strings.Contains(message, "-25211"), strings.Contains(message, "not allowed assistive access"), strings.Contains(message, "보조 접근이 허용되지"):
			return ErrAccessibilityPermissionRequired
		case strings.Contains(message, "resend control not found"):
			return ErrResendControlNotFound
		case strings.Contains(message, "resend outcome not verified"):
			return ErrResendOutcomeUnverified
		case strings.Contains(message, "exact browser targeting unsupported"):
			return ErrUnsupported
		default:
			return errors.New("request OTP resend through accessibility")
		}
	}
	return validateResendOutput(output)
}

func validateResendOutput(output []byte) error {
	var result struct {
		Requested  bool `json:"requested"`
		UIVerified bool `json:"ui_verified"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("request OTP resend through accessibility: invalid response")
	}
	if !result.Requested || !result.UIVerified {
		return ErrResendOutcomeUnverified
	}
	return nil
}
