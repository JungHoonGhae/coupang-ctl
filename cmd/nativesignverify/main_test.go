package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

type fakeNativeSigningVerifier struct {
	macOS   releasecontract.MacOSNativeSigningEvidence
	windows releasecontract.WindowsNativeSigningEvidence
}

func (verifier fakeNativeSigningVerifier) VerifyMacOS(_ context.Context, _ releasecontract.MacOSNativeSigningRequest) (releasecontract.MacOSNativeSigningEvidence, error) {
	return verifier.macOS, nil
}

func (verifier fakeNativeSigningVerifier) VerifyWindows(_ context.Context, _ releasecontract.WindowsNativeSigningRequest) (releasecontract.WindowsNativeSigningEvidence, error) {
	return verifier.windows, nil
}

func TestRunEmitsVerifiedMacOSEvidence(t *testing.T) {
	verifier := fakeNativeSigningVerifier{macOS: releasecontract.MacOSNativeSigningEvidence{
		SchemaVersion:  1,
		Platform:       "darwin",
		Artifact:       "coupangctl",
		ArtifactSHA256: "6993795d8ecdfcc6c004e3b145f569cb4e0dfe917f084a65ebe809eeff04fcbb",
		Verified:       true,
		ReasonCode:     "verified",
	}}
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"--platform", "darwin",
		"--artifact", "/synthetic/release/coupangctl",
		"--expected-team-id", "A1B2C3D4E5",
		"--expected-code-identifier", "io.github.example.coupangctl",
	}, &stdout, &stderr, verifier)

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var evidence releasecontract.MacOSNativeSigningEvidence
	if err := json.Unmarshal(stdout.Bytes(), &evidence); err != nil {
		t.Fatal(err)
	}
	if !evidence.Verified || evidence.Platform != "darwin" || evidence.ReasonCode != "verified" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestRunFailsClosedWithoutLeakingRejectedInput(t *testing.T) {
	const privatePath = `C:\Users\private-person\coupangctl.exe`
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"--platform", "windows",
		"--artifact", privatePath,
	}, &stdout, &stderr, fakeNativeSigningVerifier{})

	if exitCode != 2 || stdout.Len() != 0 {
		t.Fatalf("exit code = %d, stdout = %q", exitCode, stdout.String())
	}
	if strings.Contains(stderr.String(), privatePath) || strings.Contains(stderr.String(), "private-person") {
		t.Fatalf("stderr leaked rejected input: %q", stderr.String())
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &response); err != nil || response.Error.Code != "invalid_request" {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
}
