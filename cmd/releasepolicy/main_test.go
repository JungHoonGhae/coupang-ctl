package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

func TestRunReturnsStructuredPrereleasePolicy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"--tag", "v1.2.3-rc.1"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %s", exitCode, stderr.String())
	}
	var policy releasecontract.PublicationPolicy
	if err := json.Unmarshal(stdout.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if !policy.PublishAllowed || policy.Channel != "prerelease" || policy.NativeSigning != "not_implemented" || stderr.Len() != 0 {
		t.Fatalf("policy = %#v, stderr = %q", policy, stderr.String())
	}
}

func TestRunDeniesUnsignedStableAndRejectsInvalidTag(t *testing.T) {
	var stableOut, stableErr bytes.Buffer
	if exitCode := run([]string{"--tag", "v1.2.3"}, &stableOut, &stableErr); exitCode != 1 {
		t.Fatalf("stable exit code = %d", exitCode)
	}
	var policy releasecontract.PublicationPolicy
	if err := json.Unmarshal(stableOut.Bytes(), &policy); err != nil || policy.PublishAllowed || policy.ReasonCode != "stable_native_signing_required" || stableErr.Len() != 0 {
		t.Fatalf("stable policy = %#v, error = %v, stderr = %q", policy, err, stableErr.String())
	}

	var invalidOut, invalidErr bytes.Buffer
	if exitCode := run([]string{"--tag", "main"}, &invalidOut, &invalidErr); exitCode != 2 {
		t.Fatalf("invalid exit code = %d", exitCode)
	}
	if invalidOut.Len() != 0 {
		t.Fatalf("invalid stdout = %q", invalidOut.String())
	}
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(invalidErr.Bytes(), &response); err != nil || response.Error.Code != "invalid_tag" || response.Error.Message == "" {
		t.Fatalf("invalid response = %#v, error = %v", response, err)
	}
}
