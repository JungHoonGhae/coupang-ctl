package browserbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/extension"
)

type testInstallationRecord struct {
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

func TestInstallRecordsDigestsAndUpgradesOnlyOwnedArtifacts(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	record := readTestInstallationRecord(t, manager.installationRecordPath())
	if record.SchemaVersion != 2 || record.State != "active" || len(record.ArtifactSHA256) != 5 || len(record.TargetArtifactSHA256) != 0 {
		t.Fatalf("unexpected installation record: %#v", record)
	}
	for name, digest := range record.ArtifactSHA256 {
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
			t.Fatalf("artifact %q digest %q is not SHA-256", name, digest)
		}
	}

	priorAction := []byte("// synthetic reviewed prior version\n")
	record.ArtifactSHA256["extension/action.js"] = digestBytes(priorAction)
	writeTestInstallationRecord(t, manager.installationRecordPath(), record)
	actionPath := filepath.Join(manager.extensionPath(), "action.js")
	if err := os.WriteFile(actionPath, priorAction, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Install()
	if err != nil {
		t.Fatalf("Install() upgrade error = %v", err)
	}
	if result.Status != "upgraded" || result.NextAction != "reload_extension_in_chrome" {
		t.Fatalf("unexpected upgrade result: %#v", result)
	}
	wantAction, err := extensionbundle.Files.ReadFile("action.js")
	if err != nil {
		t.Fatal(err)
	}
	gotAction, err := os.ReadFile(actionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotAction, wantAction) {
		t.Fatal("upgrade did not restore the embedded action")
	}
	upgraded := readTestInstallationRecord(t, manager.installationRecordPath())
	if upgraded.State != "active" || upgraded.ArtifactSHA256["extension/action.js"] != digestBytes(wantAction) || len(upgraded.TargetArtifactSHA256) != 0 {
		t.Fatalf("upgrade record did not commit desired digests: %#v", upgraded)
	}
}

func TestInstallRefusesUnrecordedBundleTampering(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	actionPath := filepath.Join(manager.extensionPath(), "action.js")
	tampered := []byte("// unrecorded change\n")
	if err := os.WriteFile(actionPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject unrecorded bundle tampering")
	}
	got, err := os.ReadFile(actionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, tampered) {
		t.Fatal("rejected upgrade modified the unrecorded file")
	}
}

func TestInstallMigratesExactLegacyOwnershipRecord(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	legacy, err := manager.legacyInstallationRecordContent()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.installationRecordPath(), legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Install()
	if err != nil {
		t.Fatalf("Install() legacy migration error = %v", err)
	}
	if result.Status != "installed" {
		t.Fatalf("legacy migration status = %q, want installed", result.Status)
	}
	record := readTestInstallationRecord(t, manager.installationRecordPath())
	if record.SchemaVersion != 2 || record.State != "active" || len(record.ArtifactSHA256) != 5 {
		t.Fatalf("legacy record was not migrated: %#v", record)
	}
}

func TestInstallRecoversRecordedInterruptedUpgrade(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	record := readTestInstallationRecord(t, manager.installationRecordPath())
	desiredDigests := make(map[string]string, len(record.ArtifactSHA256))
	for name, digest := range record.ArtifactSHA256 {
		desiredDigests[name] = digest
	}
	priorAction := []byte("// prior action left by an interrupted upgrade\n")
	record.State = "upgrading"
	record.ArtifactSHA256["extension/action.js"] = digestBytes(priorAction)
	record.TargetArtifactSHA256 = desiredDigests
	writeTestInstallationRecord(t, manager.installationRecordPath(), record)
	if err := os.WriteFile(filepath.Join(manager.extensionPath(), "action.js"), priorAction, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Install()
	if err != nil {
		t.Fatalf("Install() interrupted recovery error = %v", err)
	}
	if result.Status != "upgraded" {
		t.Fatalf("interrupted recovery status = %q, want upgraded", result.Status)
	}
	recovered := readTestInstallationRecord(t, manager.installationRecordPath())
	if recovered.State != "active" || len(recovered.TargetArtifactSHA256) != 0 {
		t.Fatalf("interrupted record was not committed: %#v", recovered)
	}
}

func TestInstallRejectsUnexpectedManagedExtensionFile(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	unexpected := filepath.Join(manager.extensionPath(), "private-payload.json")
	if err := os.WriteFile(unexpected, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject an unexpected managed extension file")
	}
	if got, err := os.ReadFile(unexpected); err != nil || string(got) != "synthetic" {
		t.Fatalf("rejected install changed unexpected file: %q %v", got, err)
	}
}

func TestInstallUpdatesOwnedNativeManifestWhenExecutableMoves(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	newExecutable := filepath.Join(filepath.Dir(manager.environment.ExecutablePath), "next", "coupangctl")
	if err := os.MkdirAll(filepath.Dir(newExecutable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newExecutable, []byte("synthetic upgraded executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	nextEnvironment := manager.environment
	nextEnvironment.ExecutablePath = newExecutable
	next, err := New(nextEnvironment)
	if err != nil {
		t.Fatal(err)
	}

	result, err := next.Install()
	if err != nil {
		t.Fatalf("Install() moved executable error = %v", err)
	}
	if result.Status != "upgraded" {
		t.Fatalf("moved executable status = %q, want upgraded", result.Status)
	}
	manifestContent, err := os.ReadFile(result.NativeHostManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Path != newExecutable {
		t.Fatalf("native host path = %q, want %q", manifest.Path, newExecutable)
	}
	if report := next.Doctor(); !report.Ready {
		t.Fatalf("doctor after executable move = %#v", report)
	}
}

func readTestInstallationRecord(t *testing.T, path string) testInstallationRecord {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record testInstallationRecord
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func writeTestInstallationRecord(t *testing.T, path string, record testInstallationRecord) {
	t.Helper()
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func digestBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func TestInstallPreflightsConflictsBeforeWritingAnything(t *testing.T) {
	manager := newTestManager(t)
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("owned by another application\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a conflicting native host manifest")
	}
	if _, err := os.Stat(manager.extensionPath()); !os.IsNotExist(err) {
		t.Fatalf("failed preflight left an extension bundle behind: %v", err)
	}
	if _, err := os.Stat(manager.installationRecordPath()); !os.IsNotExist(err) {
		t.Fatalf("failed preflight left an ownership record behind: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil || string(content) != "owned by another application\n" {
		t.Fatalf("conflicting manifest was changed: %q %v", content, err)
	}
}

func TestInstallRejectsNonExecutableBeforeWritingArtifacts(t *testing.T) {
	manager := newTestManager(t)
	if err := os.Chmod(manager.environment.ExecutablePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a non-executable binary")
	}
	if _, err := os.Stat(manager.extensionPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected install wrote an extension bundle: %v", err)
	}
}

func TestInstallRejectsManagedDirectorySymlinkWithoutTouchingTarget(t *testing.T) {
	manager := newTestManager(t)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(victim, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.extensionPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, manager.extensionPath()); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a symlinked managed extension directory")
	}
	entries, err := os.ReadDir(victim)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("rejected install wrote through the symlink: %#v", entries)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("sentinel changed: %q %v", content, err)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "coupangctl")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Environment{
		GOOS:           "darwin",
		HomeDir:        filepath.Join(root, "home"),
		ConfigDir:      filepath.Join(root, "config"),
		StateDir:       filepath.Join(root, "state"),
		ExecutablePath: executable,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
