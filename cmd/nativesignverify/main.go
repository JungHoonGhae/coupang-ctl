// Command nativesignverify validates one unpacked release executable with the
// trust tools of its target operating system. It emits only typed, redacted
// evidence and never treats credential presence or caller assertions as proof.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

type nativeSigningVerifier interface {
	VerifyMacOS(context.Context, releasecontract.MacOSNativeSigningRequest) (releasecontract.MacOSNativeSigningEvidence, error)
	VerifyWindows(context.Context, releasecontract.WindowsNativeSigningRequest) (releasecontract.WindowsNativeSigningEvidence, error)
}

func main() {
	verifier, err := releasecontract.NewNativeSigningVerifier(runtime.GOOS, releasecontract.ExecNativeToolRunner{})
	if err != nil {
		writeError(os.Stderr, "verifier_unavailable", "native signing verifier could not be initialized")
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, verifier))
}

func run(args []string, stdout, stderr io.Writer, verifier nativeSigningVerifier) int {
	flags := flag.NewFlagSet("nativesignverify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	platform := flags.String("platform", "", "artifact platform: darwin or windows")
	artifact := flags.String("artifact", "", "unpacked executable to verify")
	expectedTeamID := flags.String("expected-team-id", "", "expected Apple Developer Team ID")
	expectedCodeIdentifier := flags.String("expected-code-identifier", "", "expected stable macOS code-signing identifier")
	expectedPublisherHash := flags.String("expected-publisher-subject-sha256", "", "SHA-256 of the expected Authenticode publisher subject")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *artifact == "" {
		writeError(stderr, "invalid_request", "provide one platform, artifact, and its platform-specific expected publisher identity")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch *platform {
	case "darwin":
		if *expectedTeamID == "" || *expectedCodeIdentifier == "" || *expectedPublisherHash != "" {
			writeError(stderr, "invalid_request", "darwin verification requires only --expected-team-id and --expected-code-identifier")
			return 2
		}
		evidence, err := verifier.VerifyMacOS(ctx, releasecontract.MacOSNativeSigningRequest{
			ArtifactPath:           *artifact,
			ExpectedTeamID:         *expectedTeamID,
			ExpectedCodeIdentifier: *expectedCodeIdentifier,
		})
		return writeEvidence(stdout, stderr, evidence, evidence.Verified, err)
	case "windows":
		if *expectedPublisherHash == "" || *expectedTeamID != "" || *expectedCodeIdentifier != "" {
			writeError(stderr, "invalid_request", "windows verification requires only --expected-publisher-subject-sha256")
			return 2
		}
		evidence, err := verifier.VerifyWindows(ctx, releasecontract.WindowsNativeSigningRequest{
			ArtifactPath:                   *artifact,
			ExpectedPublisherSubjectSHA256: *expectedPublisherHash,
		})
		return writeEvidence(stdout, stderr, evidence, evidence.Verified, err)
	default:
		writeError(stderr, "invalid_request", "--platform must be darwin or windows")
		return 2
	}
}

func writeEvidence(stdout, stderr io.Writer, evidence any, verified bool, err error) int {
	if err != nil {
		writeError(stderr, "verification_unavailable", "native signing evidence could not be produced")
		return 1
	}
	if err := writeJSON(stdout, evidence); err != nil {
		writeError(stderr, "output_failed", "could not write native signing evidence")
		return 1
	}
	if !verified {
		return 1
	}
	return 0
}

func writeError(writer io.Writer, code, message string) {
	response := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	response.Error.Code = code
	response.Error.Message = message
	_ = writeJSON(writer, response)
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
