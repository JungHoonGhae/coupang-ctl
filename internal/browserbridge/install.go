package browserbridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/extension"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const NativeHostName = "com.coupangctl.browser_bridge"

const (
	installationRecordSchemaVersion = 2
	installationStateActive         = "active"
	installationStateUpgrading      = "upgrading"
	manifestArtifactName            = "native_host_manifest"
)

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

type managedArtifact struct {
	name    string
	path    string
	content []byte
}

type installationRecord struct {
	SchemaVersion          int               `json:"schema_version"`
	State                  string            `json:"state"`
	Browser                string            `json:"browser"`
	ExtensionID            string            `json:"extension_id"`
	ExtensionPath          string            `json:"extension_path"`
	NativeHostName         string            `json:"native_host_name"`
	NativeHostManifestPath string            `json:"native_host_manifest_path"`
	ExecutablePath         string            `json:"executable_path"`
	BundleFiles            []string          `json:"bundle_files"`
	ArtifactSHA256         map[string]string `json:"artifact_sha256"`
	TargetArtifactSHA256   map[string]string `json:"target_artifact_sha256,omitempty"`
}

type installMode int

const (
	installFresh installMode = iota
	installIdempotent
	installMigrateLegacy
	installUpgrade
)

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
	if err := manager.checkManagedDirectories(); err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	if manager.checkExecutable().Status != core.CheckOK {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("%w: coupangctl executable must be a regular executable file", ErrInstallationConflict)
	}
	artifacts, err := manager.desiredArtifacts()
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	desiredRecord := manager.desiredInstallationRecord(artifacts)
	desiredRecordContent, err := encodeInstallationRecord(desiredRecord)
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
	}
	mode, currentDigests, existingRecordContent, err := manager.preflightInstall(artifacts, desiredRecord, desiredRecordContent)
	if err != nil {
		return core.BrowserBridgeInstallation{}, err
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
	switch mode {
	case installFresh, installIdempotent:
		for _, artifact := range artifacts {
			if err := writeExactPrivateFile(artifact.path, artifact.content); err != nil {
				return core.BrowserBridgeInstallation{}, fmt.Errorf("write browser bridge installation artifact %s: %w", filepath.Base(artifact.path), err)
			}
		}
		if err := writeExactPrivateFile(recordPath, desiredRecordContent); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("write browser bridge installation record: %w", err)
		}
	case installMigrateLegacy:
		for _, artifact := range artifacts {
			if err := writeExactPrivateFile(artifact.path, artifact.content); err != nil {
				return core.BrowserBridgeInstallation{}, fmt.Errorf("secure legacy browser bridge artifact %s: %w", filepath.Base(artifact.path), err)
			}
		}
		if err := replaceOwnedPrivateFile(recordPath, desiredRecordContent, digestContent(existingRecordContent)); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("migrate browser bridge installation record: %w", err)
		}
	case installUpgrade:
		transition := desiredRecord
		transition.State = installationStateUpgrading
		transition.ArtifactSHA256 = currentDigests
		transition.TargetArtifactSHA256 = desiredRecord.ArtifactSHA256
		transitionContent, encodeErr := encodeInstallationRecord(transition)
		if encodeErr != nil {
			return core.BrowserBridgeInstallation{}, encodeErr
		}
		if err := replaceOwnedPrivateFile(recordPath, transitionContent, digestContent(existingRecordContent)); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("start browser bridge upgrade: %w", err)
		}
		for _, artifact := range artifacts {
			if digestContent(artifact.content) == currentDigests[artifact.name] {
				continue
			}
			if err := replaceOwnedPrivateFile(artifact.path, artifact.content, currentDigests[artifact.name]); err != nil {
				return core.BrowserBridgeInstallation{}, fmt.Errorf("upgrade browser bridge artifact %s: %w", filepath.Base(artifact.path), err)
			}
		}
		if err := replaceOwnedPrivateFile(recordPath, desiredRecordContent, digestContent(transitionContent)); err != nil {
			return core.BrowserBridgeInstallation{}, fmt.Errorf("commit browser bridge upgrade: %w", err)
		}
	}
	if err := manager.registration.Install(manifestPath); err != nil {
		return core.BrowserBridgeInstallation{}, fmt.Errorf("register browser bridge native host: %w", err)
	}
	status := "installed"
	nextAction := "load_unpacked_extension"
	if mode == installUpgrade {
		status = "upgraded"
		nextAction = "reload_extension_in_chrome"
	}

	return core.BrowserBridgeInstallation{
		SchemaVersion:          core.BrowserBridgeSchemaVersion,
		Status:                 status,
		Browser:                "chrome",
		ExtensionID:            browser.OrdinaryBrowserExtensionID,
		ExtensionPath:          extensionPath,
		NativeHostName:         NativeHostName,
		NativeHostManifestPath: manifestPath,
		InstallationRecordPath: recordPath,
		NextAction:             nextAction,
	}, nil
}

func (manager *Manager) desiredArtifacts() ([]managedArtifact, error) {
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return nil, err
	}
	manifestContent, err := manager.nativeHostManifestContent()
	if err != nil {
		return nil, err
	}
	artifacts := make([]managedArtifact, 0, len(extensionbundle.Filenames)+1)
	for _, filename := range extensionbundle.Filenames {
		content, readErr := extensionbundle.Files.ReadFile(filename)
		if readErr != nil {
			return nil, fmt.Errorf("read embedded extension file %s: %w", filename, readErr)
		}
		artifacts = append(artifacts, managedArtifact{
			name:    "extension/" + filename,
			path:    filepath.Join(manager.extensionPath(), filename),
			content: content,
		})
	}
	return append(artifacts, managedArtifact{name: manifestArtifactName, path: manifestPath, content: manifestContent}), nil
}

func (manager *Manager) desiredInstallationRecord(artifacts []managedArtifact) installationRecord {
	digests := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		digests[artifact.name] = digestContent(artifact.content)
	}
	manifestPath, _ := manager.nativeHostManifestPath()
	return installationRecord{
		SchemaVersion:          installationRecordSchemaVersion,
		State:                  installationStateActive,
		Browser:                "chrome",
		ExtensionID:            browser.OrdinaryBrowserExtensionID,
		ExtensionPath:          manager.extensionPath(),
		NativeHostName:         NativeHostName,
		NativeHostManifestPath: manifestPath,
		ExecutablePath:         manager.environment.ExecutablePath,
		BundleFiles:            slices.Clone(extensionbundle.Filenames),
		ArtifactSHA256:         digests,
	}
}

func (manager *Manager) preflightInstall(artifacts []managedArtifact, desired installationRecord, desiredContent []byte) (installMode, map[string]string, []byte, error) {
	recordPath := manager.installationRecordPath()
	info, err := os.Lstat(recordPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := manager.preflightFreshArtifacts(artifacts); err != nil {
			return 0, nil, nil, err
		}
		return installFresh, maps.Clone(desired.ArtifactSHA256), nil, nil
	}
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: inspect installation record: %v", ErrInstallationConflict, err)
	}
	if !info.Mode().IsRegular() || (manager.environment.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		return 0, nil, nil, fmt.Errorf("%w: installation record is not a private regular file", ErrInstallationConflict)
	}
	existingContent, err := os.ReadFile(recordPath)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: read installation record: %v", ErrInstallationConflict, err)
	}
	if bytes.Equal(existingContent, desiredContent) {
		if err := manager.preflightFreshArtifacts(artifacts); err != nil {
			return 0, nil, nil, err
		}
		return installIdempotent, maps.Clone(desired.ArtifactSHA256), existingContent, nil
	}

	legacyContent, legacyErr := manager.legacyInstallationRecordContent()
	if legacyErr == nil && bytes.Equal(existingContent, legacyContent) {
		if err := manager.preflightFreshArtifacts(artifacts); err != nil {
			return 0, nil, nil, err
		}
		return installMigrateLegacy, maps.Clone(desired.ArtifactSHA256), existingContent, nil
	}

	record, err := decodeInstallationRecord(existingContent)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: installation record is not a supported owned record", ErrInstallationConflict)
	}
	canonical, err := encodeInstallationRecord(record)
	if err != nil || !bytes.Equal(canonical, existingContent) {
		return 0, nil, nil, fmt.Errorf("%w: installation record is not canonical", ErrInstallationConflict)
	}
	if err := manager.validateInstallationRecord(record); err != nil {
		return 0, nil, nil, fmt.Errorf("%w: %v", ErrInstallationConflict, err)
	}
	currentDigests, err := manager.validateRecordedArtifacts(artifacts, record)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: %v", ErrInstallationConflict, err)
	}
	return installUpgrade, currentDigests, existingContent, nil
}

func (manager *Manager) preflightFreshArtifacts(artifacts []managedArtifact) error {
	if err := manager.checkExtensionDirectoryEntries(true); err != nil {
		return fmt.Errorf("%w: %v", ErrInstallationConflict, err)
	}
	for _, artifact := range artifacts {
		if err := preflightExactFile(artifact.path, artifact.content); err != nil {
			return fmt.Errorf("%w at %s: %v", ErrInstallationConflict, filepath.Base(artifact.path), err)
		}
	}
	return nil
}

func (manager *Manager) validateInstallationRecord(record installationRecord) error {
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		return err
	}
	if record.SchemaVersion != installationRecordSchemaVersion ||
		record.Browser != "chrome" ||
		record.ExtensionID != browser.OrdinaryBrowserExtensionID ||
		record.ExtensionPath != manager.extensionPath() ||
		record.NativeHostName != NativeHostName ||
		record.NativeHostManifestPath != manifestPath ||
		!filepath.IsAbs(record.ExecutablePath) ||
		!slices.Equal(record.BundleFiles, extensionbundle.Filenames) {
		return errors.New("installation ownership metadata does not match this managed location")
	}
	expectedNames := artifactNames(record.BundleFiles)
	if err := validateDigestSet(record.ArtifactSHA256, expectedNames); err != nil {
		return fmt.Errorf("invalid installed artifact digests: %w", err)
	}
	switch record.State {
	case installationStateActive:
		if len(record.TargetArtifactSHA256) != 0 {
			return errors.New("active installation record contains target digests")
		}
	case installationStateUpgrading:
		if err := validateDigestSet(record.TargetArtifactSHA256, expectedNames); err != nil {
			return fmt.Errorf("invalid upgrade target digests: %w", err)
		}
	default:
		return errors.New("installation record state is invalid")
	}
	return nil
}

func (manager *Manager) validateRecordedArtifacts(artifacts []managedArtifact, record installationRecord) (map[string]string, error) {
	if err := manager.checkExtensionDirectoryEntries(false); err != nil {
		return nil, err
	}
	current := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		content, err := manager.readPrivateRegularFile(artifact.path)
		if err != nil {
			return nil, fmt.Errorf("owned artifact %s is unavailable", filepath.Base(artifact.path))
		}
		digest := digestContent(content)
		allowed := digest == record.ArtifactSHA256[artifact.name]
		if record.State == installationStateUpgrading {
			allowed = allowed || digest == record.TargetArtifactSHA256[artifact.name]
		}
		if !allowed {
			return nil, fmt.Errorf("owned artifact %s has unrecorded content", filepath.Base(artifact.path))
		}
		current[artifact.name] = digest
	}
	return current, nil
}

func (manager *Manager) checkExtensionDirectoryEntries(allowMissing bool) error {
	entries, err := os.ReadDir(manager.extensionPath())
	if errors.Is(err, os.ErrNotExist) && allowMissing {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect managed extension directory: %w", err)
	}
	if len(entries) > len(extensionbundle.Filenames) {
		return errors.New("managed extension directory contains an unexpected file")
	}
	expected := make(map[string]struct{}, len(extensionbundle.Filenames))
	for _, filename := range extensionbundle.Filenames {
		expected[filename] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := expected[entry.Name()]; !ok {
			return errors.New("managed extension directory contains an unexpected file")
		}
	}
	if !allowMissing && len(entries) != len(expected) {
		return errors.New("managed extension directory is incomplete")
	}
	return nil
}

func (manager *Manager) readPrivateRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (manager.environment.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		return nil, errors.New("file is not a private regular file")
	}
	return os.ReadFile(path)
}

func artifactNames(bundleFiles []string) []string {
	names := make([]string, 0, len(bundleFiles)+1)
	for _, filename := range bundleFiles {
		names = append(names, "extension/"+filename)
	}
	return append(names, manifestArtifactName)
}

func validateDigestSet(digests map[string]string, expectedNames []string) error {
	if len(digests) != len(expectedNames) {
		return fmt.Errorf("digest count = %d, want %d", len(digests), len(expectedNames))
	}
	for _, name := range expectedNames {
		digest, ok := digests[name]
		if !ok || len(digest) != sha256.Size*2 {
			return fmt.Errorf("missing or invalid digest for %s", name)
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("invalid digest for %s", name)
		}
	}
	return nil
}

func digestContent(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func encodeInstallationRecord(record installationRecord) ([]byte, error) {
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode browser bridge installation record: %w", err)
	}
	return append(content, '\n'), nil
}

func decodeInstallationRecord(content []byte) (installationRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var record installationRecord
	if err := decoder.Decode(&record); err != nil {
		return installationRecord{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return installationRecord{}, errors.New("installation record has trailing data")
	}
	return record, nil
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
	pingCheck := core.Check{Name: "native_host_ping", Status: core.CheckOK}
	for _, check := range checks {
		if check.Status != core.CheckOK {
			pingCheck.Status = core.CheckError
			pingCheck.Message = "resolve the failed installation checks before retrying the synthetic native-host ping"
			break
		}
	}
	if pingCheck.Status == core.CheckOK {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := browser.PingOrdinaryBrowserNativeHost(ctx, manager.environment.StateDir)
		cancel()
		if pingErr != nil {
			pingCheck.Status = core.CheckError
			pingCheck.Message = "finish any active ordinary-browser sync, then rerun doctor or reinstall the bridge"
		}
	}
	checks = append(checks, pingCheck)

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
	artifacts, err := manager.desiredArtifacts()
	if err != nil {
		return nil, err
	}
	return encodeInstallationRecord(manager.desiredInstallationRecord(artifacts))
}

func (manager *Manager) legacyInstallationRecordContent() ([]byte, error) {
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

func replaceOwnedPrivateFile(path string, content []byte, expectedDigest string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("refusing to replace a non-regular file")
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digestContent(existing) != expectedDigest {
		return errors.New("existing file changed after ownership validation")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".coupangctl-upgrade-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	latest, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digestContent(latest) != expectedDigest {
		return errors.New("existing file changed during owned replacement")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
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
