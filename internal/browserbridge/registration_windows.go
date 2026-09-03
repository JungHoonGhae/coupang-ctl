//go:build windows

package browserbridge

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const chromeNativeHostRegistryKey = `Software\Google\Chrome\NativeMessagingHosts\` + NativeHostName

type windowsPlatformRegistration struct {
	keyPath string
}

func newPlatformRegistration(goos string) (platformRegistration, error) {
	if goos != "windows" {
		return nil, fmt.Errorf("browser bridge installer target %s does not match Windows", goos)
	}
	return windowsPlatformRegistration{keyPath: chromeNativeHostRegistryKey}, nil
}

func (registration windowsPlatformRegistration) Preflight(manifestPath string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registration.keyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	registered, _, err := key.GetStringValue("")
	if err != nil {
		return err
	}
	if registered != manifestPath {
		return errors.New("an existing Chrome native host registry value points elsewhere")
	}
	return nil
}

func (registration windowsPlatformRegistration) Install(manifestPath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registration.keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue("", manifestPath)
}

func (registration windowsPlatformRegistration) Check(manifestPath string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registration.keyPath, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	registered, _, err := key.GetStringValue("")
	if err != nil {
		return err
	}
	if registered != manifestPath {
		return errors.New("Chrome native host registry value does not match")
	}
	return nil
}

func (registration windowsPlatformRegistration) Uninstall(string) error {
	if err := registry.DeleteKey(registry.CURRENT_USER, registration.keyPath); err != nil {
		return err
	}
	return nil
}
