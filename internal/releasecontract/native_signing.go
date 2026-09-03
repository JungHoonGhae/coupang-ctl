package releasecontract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

var (
	ErrNativeSigningPlatformMismatch = errors.New("native signing verifier must run on the artifact platform")
	macOSTeamIDPattern               = regexp.MustCompile(`^[A-Z0-9]{10}$`)
	macOSCodeIdentifierPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{2,127}$`)
	sha256Pattern                    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// NativeToolResult contains process output for an OS-native trust tool. Raw
// output is deliberately consumed inside this package and never copied into
// release evidence because it can contain local paths or publisher identity.
type NativeToolResult struct {
	Stdout []byte
	Stderr []byte
}

// NativeToolRunner is the narrow system boundary used by the native signing
// verifier. Release workflows use ExecNativeToolRunner; tests can supply a
// deterministic implementation without creating signing credentials.
type NativeToolRunner interface {
	Run(context.Context, string, ...string) (NativeToolResult, error)
}

// ExecNativeToolRunner invokes an OS-native executable without a shell.
type ExecNativeToolRunner struct{}

func (ExecNativeToolRunner) Run(ctx context.Context, executable string, arguments ...string) (NativeToolResult, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return NativeToolResult{Stdout: []byte(stdout.String()), Stderr: []byte(stderr.String())}, err
}

type NativeSigningVerifier struct {
	hostOS string
	tool   NativeToolRunner
}

func NewNativeSigningVerifier(hostOS string, tool NativeToolRunner) (*NativeSigningVerifier, error) {
	if strings.TrimSpace(hostOS) == "" || tool == nil {
		return nil, errors.New("native signing verifier requires a host platform and tool runner")
	}
	return &NativeSigningVerifier{hostOS: hostOS, tool: tool}, nil
}

type MacOSNativeSigningRequest struct {
	ArtifactPath           string
	ExpectedTeamID         string
	ExpectedCodeIdentifier string
}

type MacOSNativeSigningEvidence struct {
	SchemaVersion        int    `json:"schema_version"`
	Platform             string `json:"platform"`
	Artifact             string `json:"artifact"`
	ArtifactSHA256       string `json:"artifact_sha256"`
	SignatureValid       bool   `json:"signature_valid"`
	DeveloperIDValid     bool   `json:"developer_id_valid"`
	CodeIdentifierValid  bool   `json:"code_identifier_valid"`
	HardenedRuntimeValid bool   `json:"hardened_runtime_valid"`
	SecureTimestampValid bool   `json:"secure_timestamp_valid"`
	NotarizationValid    bool   `json:"notarization_valid"`
	EntitlementsValid    bool   `json:"entitlements_valid"`
	Verified             bool   `json:"verified"`
	ReasonCode           string `json:"reason_code"`
}

type WindowsNativeSigningRequest struct {
	ArtifactPath                   string
	ExpectedPublisherSubjectSHA256 string
}

type WindowsNativeSigningEvidence struct {
	SchemaVersion        int    `json:"schema_version"`
	Platform             string `json:"platform"`
	Artifact             string `json:"artifact"`
	ArtifactSHA256       string `json:"artifact_sha256"`
	AuthenticodeValid    bool   `json:"authenticode_valid"`
	SignToolValid        bool   `json:"signtool_valid"`
	PublisherValid       bool   `json:"publisher_valid"`
	SecureTimestampValid bool   `json:"secure_timestamp_valid"`
	Verified             bool   `json:"verified"`
	ReasonCode           string `json:"reason_code"`
}

func (verifier *NativeSigningVerifier) VerifyMacOS(ctx context.Context, request MacOSNativeSigningRequest) (MacOSNativeSigningEvidence, error) {
	if verifier.hostOS != "darwin" {
		return MacOSNativeSigningEvidence{}, ErrNativeSigningPlatformMismatch
	}
	if !macOSTeamIDPattern.MatchString(request.ExpectedTeamID) {
		return MacOSNativeSigningEvidence{}, errors.New("expected macOS Team ID must contain 10 uppercase letters or digits")
	}
	if !macOSCodeIdentifierPattern.MatchString(request.ExpectedCodeIdentifier) || strings.Contains(request.ExpectedCodeIdentifier, "..") {
		return MacOSNativeSigningEvidence{}, errors.New("expected macOS code identifier is invalid")
	}
	evidence, err := newMacOSEvidence(request.ArtifactPath)
	if err != nil {
		return MacOSNativeSigningEvidence{}, err
	}

	if _, err := verifier.tool.Run(ctx, "/usr/bin/codesign", "--verify", "--strict=all", "--verbose=2", request.ArtifactPath); err != nil {
		evidence.ReasonCode = "signature_invalid"
		return evidence, nil
	}
	evidence.SignatureValid = true

	developerIDRequirement := `identifier "` + request.ExpectedCodeIdentifier + `" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "` + request.ExpectedTeamID + `"`
	if _, err := verifier.tool.Run(ctx, "/usr/bin/codesign", "--verify", "--strict=all", "--verbose=2", "-R", developerIDRequirement, request.ArtifactPath); err != nil {
		evidence.ReasonCode = "developer_id_or_identifier_invalid"
		return evidence, nil
	}
	evidence.DeveloperIDValid = true
	evidence.CodeIdentifierValid = true

	if _, err := verifier.tool.Run(ctx, "/usr/bin/codesign", "--verify", "--strict=all", "--verbose=2", "--check-notarization", "-R=notarized", request.ArtifactPath); err != nil {
		evidence.ReasonCode = "notarization_invalid"
		return evidence, nil
	}
	evidence.NotarizationValid = true

	metadata, err := verifier.tool.Run(ctx, "/usr/bin/codesign", "--display", "--verbose=4", request.ArtifactPath)
	if err != nil {
		evidence.ReasonCode = "signature_metadata_unavailable"
		return evidence, nil
	}
	display := string(metadata.Stdout) + "\n" + string(metadata.Stderr)
	evidence.HardenedRuntimeValid = hasMacOSRuntimeFlag(display)
	if !evidence.HardenedRuntimeValid {
		evidence.ReasonCode = "hardened_runtime_missing"
		return evidence, nil
	}
	evidence.SecureTimestampValid = hasMacOSSecureTimestamp(display)
	if !evidence.SecureTimestampValid {
		evidence.ReasonCode = "secure_timestamp_missing"
		return evidence, nil
	}
	entitlements, err := verifier.tool.Run(ctx, "/usr/bin/codesign", "--display", "--entitlements", "-", "--xml", request.ArtifactPath)
	if err != nil {
		evidence.ReasonCode = "entitlements_unavailable"
		return evidence, nil
	}
	if strings.TrimSpace(string(entitlements.Stdout)) != "" {
		evidence.ReasonCode = "unexpected_entitlements"
		return evidence, nil
	}
	evidence.EntitlementsValid = true
	if !nativeArtifactDigestMatches(request.ArtifactPath, evidence.ArtifactSHA256) {
		evidence.ReasonCode = "artifact_changed"
		return evidence, nil
	}
	evidence.Verified = true
	evidence.ReasonCode = "verified"
	return evidence, nil
}

func (verifier *NativeSigningVerifier) VerifyWindows(ctx context.Context, request WindowsNativeSigningRequest) (WindowsNativeSigningEvidence, error) {
	if verifier.hostOS != "windows" {
		return WindowsNativeSigningEvidence{}, ErrNativeSigningPlatformMismatch
	}
	if !sha256Pattern.MatchString(request.ExpectedPublisherSubjectSHA256) {
		return WindowsNativeSigningEvidence{}, errors.New("expected Windows publisher subject hash must be lowercase SHA-256")
	}
	evidence, err := newWindowsEvidence(request.ArtifactPath)
	if err != nil {
		return WindowsNativeSigningEvidence{}, err
	}

	encodedCommand := encodePowerShellCommand(authenticodePowerShell(request.ArtifactPath, request.ExpectedPublisherSubjectSHA256))
	result, runErr := verifier.tool.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedCommand)
	if runErr != nil {
		evidence.ReasonCode = "authenticode_check_failed"
		return evidence, nil
	}
	var checks struct {
		AuthenticodeValid    bool `json:"authenticode_valid"`
		SignToolValid        bool `json:"signtool_valid"`
		PublisherValid       bool `json:"publisher_valid"`
		SecureTimestampValid bool `json:"secure_timestamp_valid"`
	}
	if err := json.Unmarshal(result.Stdout, &checks); err != nil {
		evidence.ReasonCode = "authenticode_result_invalid"
		return evidence, nil
	}
	evidence.AuthenticodeValid = checks.AuthenticodeValid
	evidence.SignToolValid = checks.SignToolValid
	evidence.PublisherValid = checks.PublisherValid
	evidence.SecureTimestampValid = checks.SecureTimestampValid
	switch {
	case !evidence.SignToolValid:
		evidence.ReasonCode = "signtool_invalid"
	case !evidence.AuthenticodeValid:
		evidence.ReasonCode = "authenticode_invalid"
	case !evidence.PublisherValid:
		evidence.ReasonCode = "publisher_mismatch"
	case !evidence.SecureTimestampValid:
		evidence.ReasonCode = "secure_timestamp_missing"
	case !nativeArtifactDigestMatches(request.ArtifactPath, evidence.ArtifactSHA256):
		evidence.ReasonCode = "artifact_changed"
	default:
		evidence.Verified = true
		evidence.ReasonCode = "verified"
	}
	return evidence, nil
}

func newMacOSEvidence(artifactPath string) (MacOSNativeSigningEvidence, error) {
	name, digest, err := inspectNativeArtifact(artifactPath)
	if err != nil {
		return MacOSNativeSigningEvidence{}, err
	}
	return MacOSNativeSigningEvidence{
		SchemaVersion:  1,
		Platform:       "darwin",
		Artifact:       name,
		ArtifactSHA256: digest,
		ReasonCode:     "not_verified",
	}, nil
}

func newWindowsEvidence(artifactPath string) (WindowsNativeSigningEvidence, error) {
	name, digest, err := inspectNativeArtifact(artifactPath)
	if err != nil {
		return WindowsNativeSigningEvidence{}, err
	}
	return WindowsNativeSigningEvidence{
		SchemaVersion:  1,
		Platform:       "windows",
		Artifact:       name,
		ArtifactSHA256: digest,
		ReasonCode:     "not_verified",
	}, nil
}

func inspectNativeArtifact(artifactPath string) (string, string, error) {
	info, err := os.Lstat(artifactPath)
	if err != nil {
		return "", "", errors.New("native signing artifact is unavailable")
	}
	if !info.Mode().IsRegular() {
		return "", "", errors.New("native signing artifact must be a regular file")
	}
	digest, err := fileSHA256(artifactPath)
	if err != nil {
		return "", "", errors.New("could not hash native signing artifact")
	}
	return filepath.Base(artifactPath), digest, nil
}

func nativeArtifactDigestMatches(artifactPath, expected string) bool {
	digest, err := fileSHA256(artifactPath)
	return err == nil && digest == expected
}

func hasMacOSRuntimeFlag(display string) bool {
	for _, line := range strings.Split(display, "\n") {
		if strings.Contains(line, "flags=") && strings.Contains(line, "(runtime)") {
			return true
		}
	}
	return false
}

func hasMacOSSecureTimestamp(display string) bool {
	for _, line := range strings.Split(display, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Timestamp=") {
			return true
		}
	}
	return false
}

func authenticodePowerShell(artifactPath, expectedSubjectSHA256 string) string {
	pathBase64 := base64.StdEncoding.EncodeToString([]byte(artifactPath))
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$artifact = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$expected = '%s'
$result = [ordered]@{
  authenticode_valid = $false
  signtool_valid = $false
  publisher_valid = $false
  secure_timestamp_valid = $false
}
try {
  $signTool = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($null -eq $signTool) {
    $signTool = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue |
      Sort-Object FullName -Descending |
      Select-Object -First 1
  }
  if ($null -ne $signTool) {
    $signToolPath = if ($null -ne $signTool.Source) { $signTool.Source } else { $signTool.FullName }
    $signToolOutput = & $signToolPath verify /pa /all /tw /v $artifact 2>&1
    $result.signtool_valid = ($LASTEXITCODE -eq 0)
  }
  $signature = Get-AuthenticodeSignature -LiteralPath $artifact
  $result.authenticode_valid = ([string]$signature.Status -eq 'Valid')
  if ($null -ne $signature.SignerCertificate) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
      $subjectBytes = [Text.Encoding]::UTF8.GetBytes([string]$signature.SignerCertificate.Subject)
      $subjectHash = ([BitConverter]::ToString($sha.ComputeHash($subjectBytes))).Replace('-', '').ToLowerInvariant()
      $result.publisher_valid = ($subjectHash -eq $expected)
    } finally {
      $sha.Dispose()
    }
  }
  $result.secure_timestamp_valid = ($null -ne $signature.TimeStamperCertificate)
} catch {}
$result | ConvertTo-Json -Compress
`, pathBase64, expectedSubjectSHA256)
}

func encodePowerShellCommand(command string) string {
	codeUnits := utf16.Encode([]rune(command))
	bytes := make([]byte, len(codeUnits)*2)
	for index, codeUnit := range codeUnits {
		bytes[index*2] = byte(codeUnit)
		bytes[index*2+1] = byte(codeUnit >> 8)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
