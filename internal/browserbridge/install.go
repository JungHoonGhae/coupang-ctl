package browserbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/JungHoonGhae/coupang-ctl/extension"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const NativeHostName = "com.coupangctl.browser_bridge"

type Environment struct {
	GOOS           string
	HomeDir        string
	ConfigDir      string
	StateDir       string
	ExecutablePath string
}

type Manager struct {
	environment  Environment
	registration platformRegistration
}

func NewDefault(stateDir, executablePath string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	return New(Environment{
		GOOS:           runtime.GOOS,
		HomeDir:        home,
		ConfigDir:      config,
		StateDir:       stateDir,
		ExecutablePath: executablePath,
	})
}

func New(environment Environment) (*Manager, error) {
	for name, value := range map[string]string{
		"home directory":        environment.HomeDir,
		"config directory":      environment.ConfigDir,
		"state directory":       environment.StateDir,
		"coupangctl executable": environment.ExecutablePath,
	} {
		if value == "" || !filepath.IsAbs(value) {
			return nil, fmt.Errorf("%s must be an absolute path", name)
		}
	}
	switch environment.GOOS {
	case "darwin", "linux", "windows":
	default:
		return nil, fmt.Errorf("ordinary browser bridge installation is not supported on %s", environment.GOOS)
	}
	registration, err := newPlatformRegistration(environment.GOOS)
	if err != nil {
		return nil, err
	}
	return &Manager{environment: environment, registration: registration}, nil
}

func (manager *Manager) Install() (core.BrowserBridgeInstallation, error) {
	extensionPath := manager.extensionPath()
	recordPath := manager.installationRecordPath()
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	if err := manager.checkManagedDirectories(); err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	if manager.checkExecutable().Status != core.CheckOK {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("%w: coupangctl executable must be a regular executable file", ErrInstallationConflict)
	}
	manifestContent, err := manager.nativeHostManifestContent()
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	recordContent, err := manager.installationRecordContent()
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	type artifact struct {
		path    string
		content []byte
	}
	artifacts := make([]artifact, 0, len(extensionbundle.Filenames)+2)
	for _, filename := range extensionbundle.Filenames {
		content, readErr := extensionbundle.Files.ReadFile(filename)
		if readErr != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("read embedded extension file %s: %w", filename, readErr)
		}
		artifacts = append(artifacts, artifact{path: filepath.Join(extensionPath, filename), content: content})
	}
	artifacts = append(artifacts,
		artifact{path: manifestPath, content: manifestContent},
		artifact{path: recordPath, content: recordContent},
	)
	for _, artifact := range artifacts {
		if err := preflightExactFile(artifact.path, artifact.content); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("%w at %s: %v", ErrInstallationConflict, filepath.Base(artifact.path), err)
		}
	}
	if err := manager.registration.Preflight(manifestPath); err != nil {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("%w: native registration: %v", ErrInstallationConflict, err)
	}

	if err := os.MkdirAll(extensionPath, 0o700); err != nil {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("create ordinary browser extension directory: %w", err)
	}
	if err := os.Chmod(extensionPath, 0o700); err != nil {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("secure ordinary browser extension directory: %w", err)
	}
	for _, artifact := range artifacts {
		if err := writeExactPrivateFile(artifact.path, artifact.content); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("write browser bridge installation artifact %s: %w", filepath.Base(artifact.path), err)
		}
	}
	if err := manager.registration.Install(manifestPath); err != nil {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("register browser bridge native host: %w", err)
	}

	return core.BrowserBridgeInstallation{
		SchemaVersion:          core.BrowserBridgeSchemaVersion,
		Status:                 "installed",
		Browser:                "chrome",
		ExtensionID:            browser.OrdinaryBrowserExtensionID,
		ExtensionPath:          extensionPath,
		NativeHostName:         NativeHostName,
		NativeHostManifestPath: manifestPath,
		InstallationRecordPath: recordPath,
		NextAction:             "load_unpacked_extension",
	}, nil
}

func (manager *Manager) Doctor() core.BrowserBridgeDoctorReport {
	extensionPath := manager.extensionPath()
	recordPath := manager.installationRecordPath()
	manifestPath, manifestPathErr := manager.nativeHostManifestPath()
	checks := []core.Check{manager.checkExecutable()}
	managedDirectoriesOK := manager.checkManagedDirectories() == nil
	directoryCheck := core.Check{Name: "managed_directories", Status: core.CheckOK}
	if !managedDirectoriesOK {
		directoryCheck.Status = core.CheckError
		directoryCheck.Message = "replace non-directory or linked browser bridge paths before reinstalling"
	}
	checks = append(checks, directoryCheck)
	extensionExists := pathExists(extensionPath)
	manifestExists := manifestPathErr == nil && pathExists(manifestPath)
	recordExists := pathExists(recordPath)

	recordCheck := core.Check{Name: "installation_record", Status: core.CheckOK}
	expectedRecord, recordErr := manager.installationRecordContent()
	if !managedDirectoriesOK || recordErr != nil || manager.checkManagedFile(recordPath, expectedRecord) != nil {
		recordCheck.Status = core.CheckError
		recordCheck.Message = "run coupangctl browser-bridge install to create a matching ownership record"
	}
	checks = append(checks, recordCheck)

	bundleCheck := core.Check{Name: "extension_bundle", Status: core.CheckOK}
	for _, filename := range extensionbundle.Filenames {
		content, err := extensionbundle.Files.ReadFile(filename)
		if !managedDirectoriesOK || err != nil || manager.checkManagedFile(filepath.Join(extensionPath, filename), content) != nil {
			bundleCheck.Status = core.CheckError
			bundleCheck.Message = "run coupangctl browser-bridge install to restore the reviewed extension bundle"
			break
		}
	}
	checks = append(checks, bundleCheck)

	manifestCheck := core.Check{Name: "native_host_manifest", Status: core.CheckOK}
	expectedManifest, expectedErr := manager.nativeHostManifestContent()
	if manifestPathErr != nil || expectedErr != nil || manager.checkManagedFile(manifestPath, expectedManifest) != nil {
		manifestCheck.Status = core.CheckError
		manifestCheck.Message = "run coupangctl browser-bridge install after resolving any conflicting native host registration"
	}
	checks = append(checks, manifestCheck)
	registrationCheck := core.Check{Name: "native_host_registration", Status: core.CheckOK}
	if manifestPathErr != nil || manager.registration.Check(manifestPath) != nil {
		registrationCheck.Status = core.CheckError
		registrationCheck.Message = "run coupangctl browser-bridge install to restore the current-user native host registration"
	}
	checks = append(checks, registrationCheck)

	ready := true
	for _, check := range checks {
		if check.Status != core.CheckOK {
			ready = false
			break
		}
	}
	status := "ready"
	if !ready {
		status = "repair_required"
		if !extensionExists && !manifestExists && !recordExists {
			status = "not_installed"
		}
	}
	return core.BrowserBridgeDoctorReport{
		SchemaVersion:          core.BrowserBridgeSchemaVersion,
		Status:                 status,
		Ready:                  ready,
		Browser:                "chrome",
		ExtensionID:            browser.OrdinaryBrowserExtensionID,
		ExtensionPath:          extensionPath,
		NativeHostName:         NativeHostName,
		NativeHostManifestPath: manifestPath,
		InstallationRecordPath: recordPath,
		ExtensionLoadStatus:    "not_checked",
		NextAction:             "load_or_verify_extension_in_chrome",
		Checks:                 checks,
	}
}

func (manager *Manager) Uninstall() (core.BrowserBridgeUninstallResult, error) {
	extensionPath := manager.extensionPath()
	recordPath := manager.installationRecordPath()
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return core.BrowserBridgeUninstallResult{}, err
	}
	if err := manager.checkManagedDirectories(); err != nil {
		return core.BrowserBridgeUninstallResult{}, err
	}
	if !pathExists(recordPath) {
		if !pathExists(extensionPath) && !pathExists(manifestPath) {
			return core.BrowserBridgeUninstallResult{
				SchemaVersion: core.BrowserBridgeSchemaVersion,
				Status:        "not_installed",
				Removed:       []string{},
			}, nil
		}
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("%w: refusing removal without a matching installation record", ErrInstallationConflict)
	}
	expectedRecord, err := manager.installationRecordContent()
	if err != nil || manager.checkManagedFile(recordPath, expectedRecord) != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("%w: refusing removal with a modified installation record", ErrInstallationConflict)
	}
	expectedManifest, err := manager.nativeHostManifestContent()
	if err != nil || manager.checkManagedFile(manifestPath, expectedManifest) != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("%w: refusing removal of a modified native host manifest", ErrInstallationConflict)
	}
	for _, filename := range extensionbundle.Filenames {
		expected, readErr := extensionbundle.Files.ReadFile(filename)
		if readErr != nil || manager.checkManagedFile(filepath.Join(extensionPath, filename), expected) != nil {
			return core.BrowserBridgeUninstallResult{}, fmt.Errorf("%w: refusing removal of a modified extension bundle file: %s", ErrInstallationConflict, filename)
		}
	}
	if err := manager.registration.Check(manifestPath); err != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("%w: refusing removal of a modified native host registration", ErrInstallationConflict)
	}

	if err := manager.registration.Uninstall(manifestPath); err != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("remove native host registration: %w", err)
	}
	if err := os.Remove(manifestPath); err != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("remove native host manifest: %w", err)
	}
	for _, filename := range extensionbundle.Filenames {
		if err := os.Remove(filepath.Join(extensionPath, filename)); err != nil {
			return core.BrowserBridgeUninstallResult{}, fmt.Errorf("remove extension bundle file %s: %w", filename, err)
		}
	}
	if err := os.Remove(recordPath); err != nil {
		return core.BrowserBridgeUninstallResult{}, fmt.Errorf("remove browser bridge installation record: %w", err)
	}
	_ = os.Remove(extensionPath)
	_ = os.Remove(filepath.Dir(extensionPath))

	return core.BrowserBridgeUninstallResult{
		SchemaVersion: core.BrowserBridgeSchemaVersion,
		Status:        "removed",
		Removed:       []string{"native_host_registration", "extension_bundle", "installation_record"},
	}, nil
}

func (manager *Manager) extensionPath() string {
	return filepath.Join(manager.environment.StateDir, "ordinary-browser-bridge", "extension")
}

func (manager *Manager) installationRecordPath() string {
	return filepath.Join(manager.environment.StateDir, "ordinary-browser-bridge", "installation.json")
}

func (manager *Manager) checkManagedDirectories() error {
	paths := []string{
		filepath.Join(manager.environment.StateDir, "ordinary-browser-bridge"),
		manager.extensionPath(),
	}
	if manager.environment.GOOS == "windows" {
		manifestPath, err := manager.nativeHostManifestPath()
		if err != nil {
			return err
		}
		paths = append(paths, filepath.Dir(manifestPath))
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect managed browser bridge directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: managed browser bridge path is not a directory: %s", ErrInstallationConflict, filepath.Base(path))
		}
	}
	return nil
}

func (manager *Manager) installationRecordContent() ([]byte, error) {
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return nil, err
	}
	record := struct {
		SchemaVersion          int      `json:"schema_version"`
		Browser                string   `json:"browser"`
		ExtensionID            string   `json:"extension_id"`
		ExtensionPath          string   `json:"extension_path"`
		NativeHostName         string   `json:"native_host_name"`
		NativeHostManifestPath string   `json:"native_host_manifest_path"`
		ExecutablePath         string   `json:"executable_path"`
		BundleFiles            []string `json:"bundle_files"`
	}{
		SchemaVersion:          core.BrowserBridgeSchemaVersion,
		Browser:                "chrome",
		ExtensionID:            browser.OrdinaryBrowserExtensionID,
		ExtensionPath:          manager.extensionPath(),
		NativeHostName:         NativeHostName,
		NativeHostManifestPath: manifestPath,
		ExecutablePath:         manager.environment.ExecutablePath,
		BundleFiles:            extensionbundle.Filenames,
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode browser bridge installation record: %w", err)
	}
	return append(content, '\n'), nil
}

func (manager *Manager) nativeHostManifestContent() ([]byte, error) {
	manifest := struct {
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Path           string   `json:"path"`
		Type           string   `json:"type"`
		AllowedOrigins []string `json:"allowed_origins"`
	}{
		Name:           NativeHostName,
		Description:    "Local read-only ordinary-browser bridge for coupangctl",
		Path:           manager.environment.ExecutablePath,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + browser.OrdinaryBrowserExtensionID + "/"},
	}
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode native host manifest: %w", err)
	}
	manifestContent = append(manifestContent, '\n')
	return manifestContent, nil
}

func (manager *Manager) nativeHostManifestPath() (string, error) {
	switch manager.environment.GOOS {
	case "darwin":
		return filepath.Join(manager.environment.HomeDir, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts", NativeHostName+".json"), nil
	case "linux":
		return filepath.Join(manager.environment.ConfigDir, "google-chrome", "NativeMessagingHosts", NativeHostName+".json"), nil
	case "windows":
		return filepath.Join(manager.environment.StateDir, "ordinary-browser-bridge", "native-host", NativeHostName+".json"), nil
	default:
		return "", fmt.Errorf("ordinary browser bridge installation is not supported on %s", manager.environment.GOOS)
	}
}

func writeExactPrivateFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("refusing to follow or replace a non-regular file")
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, content) {
			return errors.New("refusing to overwrite a different existing file")
		}
		return os.Chmod(path, 0o600)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return os.Chmod(path, 0o600)
}

func preflightExactFile(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("existing path is not a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return errors.New("existing file has different content")
	}
	return nil
}

func (manager *Manager) checkManagedFile(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (manager.environment.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		return errors.New("file is not a private regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return errors.New("file content does not match")
	}
	return nil
}

func (manager *Manager) checkExecutable() core.Check {
	check := core.Check{Name: "executable", Status: core.CheckOK}
	info, err := os.Lstat(manager.environment.ExecutablePath)
	if err != nil || !info.Mode().IsRegular() || (manager.environment.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		check.Status = core.CheckError
		check.Message = "reinstall coupangctl and run browser-bridge install again"
	}
	return check
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
