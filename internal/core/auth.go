package core

import (
	"context"
	"errors"
	"strings"
	"time"
)

type LoginMode string

const (
	LoginModeQR    LoginMode = "qr"
	LoginModePhone LoginMode = "phone"
)

var ErrInvalidLoginMode = errors.New("invalid login mode")
var ErrInvalidLoginRequest = errors.New("invalid login request")
var ErrInteractiveConfirmationRequired = errors.New("interactive login confirmation required")
var ErrAuthenticationRequired = errors.New("authenticated session required")
var ErrBrowserAccessDenied = errors.New("browser access denied")

const AuthRecoverySchemaVersion = 1

type LoginRequest struct {
	Mode          LoginMode       `json:"mode"`
	QROutputPath  string          `json:"-"`
	PresentQRLink QRLinkPresenter `json:"-"`
	Phone         string          `json:"-"`
	ReadOTP       OTPProvider     `json:"-"`
}

type OTPProvider func(context.Context) (string, error)

// QRLoginLink is ephemeral authentication material. It is only delivered to
// an explicitly configured presenter and must never enter structured output,
// logs, persisted sessions, fixtures, or error messages.
type QRLoginLink struct {
	URL          string `json:"-"`
	ApprovalCode string `json:"-"`
}

type QRLinkPresenter func(context.Context, QRLoginLink) error

func (r LoginRequest) Validate() error {
	switch r.Mode {
	case LoginModeQR, LoginModePhone:
		if r.QROutputPath != "" && r.Mode != LoginModeQR {
			return ErrInvalidLoginRequest
		}
		if r.PresentQRLink != nil && r.Mode != LoginModeQR {
			return ErrInvalidLoginRequest
		}
		if r.Mode == LoginModeQR && (r.Phone != "" || r.ReadOTP != nil || (r.QROutputPath != "" && r.PresentQRLink != nil)) {
			return ErrInvalidLoginRequest
		}
		if r.Mode == LoginModePhone {
			if r.ReadOTP == nil || !validPhoneDigits(r.Phone) {
				return ErrInvalidLoginRequest
			}
		}
		return nil
	default:
		return ErrInvalidLoginMode
	}
}

func validPhoneDigits(value string) bool {
	if len(value) < 10 || len(value) > 11 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

type AuthState string

const (
	AuthNotConfigured AuthState = "not_configured"
	AuthUnverified    AuthState = "unverified"
	AuthVerified      AuthState = "verified"
	AuthAccessBlocked AuthState = "access_blocked"
)

type AuthStatus struct {
	State          AuthState `json:"state"`
	Browser        string    `json:"browser,omitempty"`
	ProfilePresent bool      `json:"profile_present"`
	CheckedAt      time.Time `json:"checked_at"`
	NextAction     string    `json:"next_action,omitempty"`
}

type LoginResult struct {
	State      AuthState `json:"state"`
	Mode       LoginMode `json:"mode"`
	NextAction string    `json:"next_action"`
}

// AuthRecoveryRequest gates the only MCP authentication path that may open a
// visible browser. The workflow still performs a quiet status check first and
// opens QR login only when the profile is missing or the session is expired.
type AuthRecoveryRequest struct {
	Confirmed bool `json:"confirmed"`
}

type AuthRecoveryResult struct {
	SchemaVersion        int       `json:"schema_version"`
	BeforeState          AuthState `json:"before_state"`
	State                AuthState `json:"state"`
	VisibleBrowserOpened bool      `json:"visible_browser_opened"`
	Mode                 LoginMode `json:"mode,omitempty"`
	NextAction           string    `json:"next_action"`
}

type OTPResendResult struct {
	Requested            bool `json:"requested"`
	UITransitionVerified bool `json:"ui_transition_verified"`
	DeliveryVerified     bool `json:"delivery_verified"`
}
