package releasecontract

import "testing"

func TestUnsignedPublicationPolicyAllowsOnlyPrereleases(t *testing.T) {
	tests := []struct {
		tag            string
		channel        string
		publishAllowed bool
		reasonCode     string
	}{
		{tag: "v1.2.3", channel: "stable", publishAllowed: false, reasonCode: "stable_native_signing_required"},
		{tag: "v1.2.3+build.01", channel: "stable", publishAllowed: false, reasonCode: "stable_native_signing_required"},
		{tag: "v1.2.3-rc.1", channel: "prerelease", publishAllowed: true, reasonCode: "unsigned_prerelease_allowed"},
		{tag: "v1.2.3-rc.1+build.5", channel: "prerelease", publishAllowed: true, reasonCode: "unsigned_prerelease_allowed"},
		{tag: "v1.2.3-beta.2", channel: "prerelease", publishAllowed: true, reasonCode: "unsigned_prerelease_allowed"},
	}
	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			policy, err := EvaluateUnsignedPublication(test.tag)
			if err != nil {
				t.Fatal(err)
			}
			if policy.SchemaVersion != 1 || policy.Tag != test.tag || policy.Channel != test.channel ||
				policy.NativeSigning != "not_implemented" || policy.PublishAllowed != test.publishAllowed || policy.ReasonCode != test.reasonCode {
				t.Fatalf("policy = %#v", policy)
			}
		})
	}
}

func TestUnsignedPublicationPolicyRejectsMalformedTags(t *testing.T) {
	for _, tag := range []string{"", "main", "1.2.3", "v1.2", "v1.2.3-", "v01.2.3", "v1.2.3-..", "v1.2.3+", "v1.2.3+build..5"} {
		if _, err := EvaluateUnsignedPublication(tag); err == nil {
			t.Fatalf("EvaluateUnsignedPublication(%q) accepted malformed tag", tag)
		}
	}
}
