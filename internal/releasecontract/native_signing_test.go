package releasecontract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type scriptedNativeTool struct {
	results []NativeToolResult
	errors  []error
	hooks   []func()
	calls   int
}

func (tool *scriptedNativeTool) Run(_ context.Context, _ string, _ ...string) (NativeToolResult, error) {
	index := tool.calls
	tool.calls++
	if index >= len(tool.results) {
		return NativeToolResult{}, errors.New("unexpected native tool call")
	}
	if index < len(tool.hooks) && tool.hooks[index] != nil {
		tool.hooks[index]()
	}
	var err error
	if index < len(tool.errors) {
		err = tool.errors[index]
	}
	return tool.results[index], err
}

func TestMacOSNativeSigningVerifierRejectsArtifactChangedDuringVerification(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic signed binary")
	tool := &scriptedNativeTool{
		results: []NativeToolResult{
			{},
			{},
			{},
			{Stderr: []byte("flags=0x10000(runtime)\nTimestamp=Sep 3, 2026 at 10:00:00 AM\n")},
			{},
		},
		hooks: []func(){nil, nil, nil, nil, func() {
			if err := os.WriteFile(artifact, []byte("changed after trust checks"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	verifier, err := NewNativeSigningVerifier("darwin", tool)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := verifier.VerifyMacOS(context.Background(), MacOSNativeSigningRequest{
		ArtifactPath:           artifact,
		ExpectedTeamID:         "A1B2C3D4E5",
		ExpectedCodeIdentifier: "io.github.example.coupangctl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Verified || evidence.ReasonCode != "artifact_changed" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestMacOSNativeSigningVerifierRequiresEveryTrustProperty(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic signed binary")
	tool := &scriptedNativeTool{results: []NativeToolResult{
		{},
		{},
		{},
		{Stderr: []byte("flags=0x10000(runtime)\nTimestamp=Sep 3, 2026 at 10:00:00 AM\n")},
		{},
	}}
	verifier, err := NewNativeSigningVerifier("darwin", tool)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := verifier.VerifyMacOS(context.Background(), MacOSNativeSigningRequest{
		ArtifactPath:           artifact,
		ExpectedTeamID:         "A1B2C3D4E5",
		ExpectedCodeIdentifier: "io.github.example.coupangctl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.SchemaVersion != 1 || evidence.Platform != "darwin" || evidence.Artifact != filepath.Base(artifact) ||
		evidence.ArtifactSHA256 != "6993795d8ecdfcc6c004e3b145f569cb4e0dfe917f084a65ebe809eeff04fcbb" ||
		!evidence.SignatureValid || !evidence.DeveloperIDValid || !evidence.CodeIdentifierValid || !evidence.EntitlementsValid || !evidence.HardenedRuntimeValid ||
		!evidence.SecureTimestampValid || !evidence.NotarizationValid || !evidence.Verified || evidence.ReasonCode != "verified" {
		t.Fatalf("evidence = %#v", evidence)
	}
	if tool.calls != 5 {
		t.Fatalf("native tool calls = %d, want 5", tool.calls)
	}
}

func TestMacOSNativeSigningVerifierRejectsOrdinarySigningTime(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic signed binary")
	tool := &scriptedNativeTool{results: []NativeToolResult{
		{},
		{},
		{},
		{Stderr: []byte("flags=0x10000(runtime)\nSigned Time=Sep 3, 2026 at 10:00:00 AM\n")},
		{},
	}}
	verifier, err := NewNativeSigningVerifier("darwin", tool)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := verifier.VerifyMacOS(context.Background(), MacOSNativeSigningRequest{
		ArtifactPath:           artifact,
		ExpectedTeamID:         "A1B2C3D4E5",
		ExpectedCodeIdentifier: "io.github.example.coupangctl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Verified || evidence.SecureTimestampValid || evidence.ReasonCode != "secure_timestamp_missing" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestWindowsNativeSigningVerifierRequiresTrustedPublisherAndTimestamp(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic signed binary")
	tool := &scriptedNativeTool{results: []NativeToolResult{{Stdout: []byte(`{
  "authenticode_valid": true,
  "signtool_valid": true,
  "publisher_valid": true,
  "secure_timestamp_valid": true
}`)}}}
	verifier, err := NewNativeSigningVerifier("windows", tool)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := verifier.VerifyWindows(context.Background(), WindowsNativeSigningRequest{
		ArtifactPath:                   artifact,
		ExpectedPublisherSubjectSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.SchemaVersion != 1 || evidence.Platform != "windows" || evidence.Artifact != filepath.Base(artifact) ||
		evidence.ArtifactSHA256 != "6993795d8ecdfcc6c004e3b145f569cb4e0dfe917f084a65ebe809eeff04fcbb" ||
		!evidence.AuthenticodeValid || !evidence.SignToolValid || !evidence.PublisherValid || !evidence.SecureTimestampValid ||
		!evidence.Verified || evidence.ReasonCode != "verified" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestWindowsNativeSigningVerifierRejectsArtifactChangedDuringVerification(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic signed binary")
	tool := &scriptedNativeTool{
		results: []NativeToolResult{{Stdout: []byte(`{
  "authenticode_valid": true,
  "signtool_valid": true,
  "publisher_valid": true,
  "secure_timestamp_valid": true
}`)}},
		hooks: []func(){func() {
			if err := os.WriteFile(artifact, []byte("changed after trust checks"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	verifier, err := NewNativeSigningVerifier("windows", tool)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := verifier.VerifyWindows(context.Background(), WindowsNativeSigningRequest{
		ArtifactPath:                   artifact,
		ExpectedPublisherSubjectSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Verified || evidence.ReasonCode != "artifact_changed" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestNativeSigningVerifierFailsClosedWithoutCallingWrongHostTools(t *testing.T) {
	artifact := writeSyntheticNativeArtifact(t, "synthetic unsigned binary")
	tool := &scriptedNativeTool{}
	verifier, err := NewNativeSigningVerifier("linux", tool)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := verifier.VerifyMacOS(context.Background(), MacOSNativeSigningRequest{
		ArtifactPath:           artifact,
		ExpectedTeamID:         "A1B2C3D4E5",
		ExpectedCodeIdentifier: "io.github.example.coupangctl",
	}); !errors.Is(err, ErrNativeSigningPlatformMismatch) {
		t.Fatalf("VerifyMacOS() error = %v", err)
	}
	if _, err := verifier.VerifyWindows(context.Background(), WindowsNativeSigningRequest{
		ArtifactPath:                   artifact,
		ExpectedPublisherSubjectSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}); !errors.Is(err, ErrNativeSigningPlatformMismatch) {
		t.Fatalf("VerifyWindows() error = %v", err)
	}
	if tool.calls != 0 {
		t.Fatalf("native tool calls = %d, want 0", tool.calls)
	}
}

func writeSyntheticNativeArtifact(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coupangctl")
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
