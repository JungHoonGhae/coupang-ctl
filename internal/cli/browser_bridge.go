package cli

import (
	"errors"
	"io"

	"github.com/JungHoonGhae/coupang-ctl/internal/browserbridge"
)

func runBrowserBridge(args []string, stdout io.Writer, stateDir, executable string) error {
	if len(args) != 1 {
		return errors.New("usage: coupangctl browser-bridge <install|doctor|uninstall>")
	}
	manager, err := browserbridge.NewDefault(stateDir, executable)
	if err != nil {
		return err
	}
	switch args[0] {
	case "install":
		result, err := manager.Install()
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "doctor":
		return writeJSON(stdout, manager.Doctor())
	case "uninstall":
		result, err := manager.Uninstall()
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return errors.New("usage: coupangctl browser-bridge <install|doctor|uninstall>")
	}
}
