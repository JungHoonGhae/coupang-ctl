package releasecontract

import (
	"errors"
	"strings"
)

// PublicationPolicy describes whether the current unsigned release pipeline
// may publish a tag. It deliberately has no caller-supplied "signed" switch:
// future stable enablement must come from cryptographically verified native
// signing evidence.
type PublicationPolicy struct {
	SchemaVersion  int    `json:"schema_version"`
	Tag            string `json:"tag"`
	Channel        string `json:"channel"`
	NativeSigning  string `json:"native_signing"`
	PublishAllowed bool   `json:"publish_allowed"`
	ReasonCode     string `json:"reason_code"`
}

// EvaluateUnsignedPublication permits explicitly marked prereleases while the
// repository has no native signing pipeline. Stable tags fail closed.
func EvaluateUnsignedPublication(tag string) (PublicationPolicy, error) {
	if !releaseTagPattern.MatchString(tag) {
		return PublicationPolicy{}, errors.New("release tag must be semantic and start with v")
	}
	policy := PublicationPolicy{
		SchemaVersion: 1,
		Tag:           tag,
		NativeSigning: "not_implemented",
	}
	version := strings.TrimPrefix(tag, "v")
	if buildIndex := strings.IndexByte(version, '+'); buildIndex >= 0 {
		version = version[:buildIndex]
	}
	if strings.Contains(version, "-") {
		policy.Channel = "prerelease"
		policy.PublishAllowed = true
		policy.ReasonCode = "unsigned_prerelease_allowed"
		return policy, nil
	}
	policy.Channel = "stable"
	policy.ReasonCode = "stable_native_signing_required"
	return policy, nil
}
