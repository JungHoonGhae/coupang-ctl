package core

import "time"

const CurrentBrowserStatusSchemaVersion = 1

type CurrentBrowserState string

const (
	CurrentBrowserNotEnabled        CurrentBrowserState = "not_enabled"
	CurrentBrowserEndpointAvailable CurrentBrowserState = "endpoint_available"
)

// CurrentBrowserStatus reports only passive endpoint readiness. It never
// includes the local port, debugger token, or browser profile path.
type CurrentBrowserStatus struct {
	SchemaVersion              int                 `json:"schema_version"`
	State                      CurrentBrowserState `json:"state"`
	Browser                    string              `json:"browser,omitempty"`
	EndpointAvailable          bool                `json:"endpoint_available"`
	ConnectionApprovalVerified bool                `json:"connection_approval_verified"`
	CheckedAt                  time.Time           `json:"checked_at"`
	NextAction                 string              `json:"next_action"`
}
