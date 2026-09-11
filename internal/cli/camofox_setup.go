package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

func runCamofoxSetup(ctx context.Context, args []string, stdout io.Writer) error {
	const usage = "usage: coupangctl camofox setup --runtime PATH --engine-dir PATH [--node PATH] [--user-id NAME]"
	if len(args) == 0 || args[0] == "--help" || (len(args) == 2 && args[0] == "setup" && args[1] == "--help") {
		_, err := fmt.Fprintln(stdout, usage)
		return err
	}
	if args[0] != "setup" {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	flags := newFlagSet("camofox setup")
	runtimeDir := flags.String("runtime", "", "installed Camofox server directory")
	engineDir := flags.String("engine-dir", "", "verified installed Camoufox engine directory")
	node := flags.String("node", "", "Node executable; defaults to node on PATH")
	userID := flags.String("user-id", "", "dedicated Camofox profile label; preserves existing selection by default")
	if err := parseFlags(flags, args[1:], usage); err != nil {
		return err
	}
	if !filepath.IsAbs(*runtimeDir) || !filepath.IsAbs(*engineDir) {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if *node == "" {
		path, err := exec.LookPath("node")
		if err != nil {
			return browser.ErrCamofoxUnavailable
		}
		*node = path
	}
	if !filepath.IsAbs(*node) {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := exec.CommandContext(probe, *node, "--version").Output()
	if err != nil {
		return browser.ErrCamofoxUnavailable
	}
	major, err := strconv.Atoi(strings.Split(strings.TrimPrefix(strings.TrimSpace(string(version)), "v"), ".")[0])
	if err != nil || major < 22 {
		return browser.ErrCamofoxUnavailable
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		return err
	}
	if *userID == "" {
		if old, err := browser.ReadCamofoxConfig(paths.StateDir); err == nil {
			*userID = old.UserID
		} else {
			*userID = "coupangctl"
		}
	}
	cfg := browser.CamofoxConfig{SchemaVersion: 1, Node: *node, Server: filepath.Join(*runtimeDir, "server.js"), EngineDir: *engineDir, UserID: *userID, DefaultBrowser: true}
	if err := browser.SaveCamofoxConfig(paths.StateDir, cfg); err != nil {
		return err
	}
	return writeJSON(stdout, map[string]any{"schema_version": 1, "browser": "Camofox", "default_browser": true, "browser_started": false, "authentication_checked": false, "next_action": "coupangctl auth status; if authentication is required, coupangctl login --manual or --qr --link"})
}
