package auth

import (
	"context"
	"errors"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type Browser interface {
	Inspect(context.Context) (BrowserStatus, error)
	Login(context.Context, core.LoginRequest) error
	Verify(context.Context) error
}

const verifiedSessionNextAction = "authentication is verified; account identity, order access and history coverage require separate checks"

func (s *Service) Verify(ctx context.Context) (core.AuthStatus, error) {
	status, err := s.browser.Inspect(ctx)
	if err != nil {
		return core.AuthStatus{}, err
	}
	if err := s.browser.Verify(ctx); err != nil {
		return core.AuthStatus{}, err
	}
	return core.AuthStatus{
		VerificationScope: core.AuthVerificationSession,
		State:             core.AuthVerified,
		Browser:           status.Name,
		ProfilePresent:    true,
		CheckedAt:         s.now().UTC(),
		NextAction:        verifiedSessionNextAction,
	}, nil
}

type BrowserStatus struct {
	Name           string
	ProfilePresent bool
	// An explicit adapter may need recovery within its own profile rather
	// than the legacy Chrome --headed path. This is a public, static hint.
	AccessBlockedAction string
}

type Service struct {
	browser Browser
	now     func() time.Time
}

func NewService(browser Browser) *Service {
	return &Service{browser: browser, now: time.Now}
}

func (s *Service) Status(ctx context.Context) (core.AuthStatus, error) {
	status, err := s.browser.Inspect(ctx)
	if err != nil {
		return core.AuthStatus{}, err
	}

	result := core.AuthStatus{
		State:          core.AuthNotConfigured,
		Browser:        status.Name,
		ProfilePresent: status.ProfilePresent,
		CheckedAt:      s.now().UTC(),
		NextAction:     "run `coupangctl login` on an interactive desktop",
	}
	if status.ProfilePresent {
		if err := s.browser.Verify(ctx); err != nil {
			switch {
			case errors.Is(err, core.ErrAuthenticationRequired):
				result.State = core.AuthUnverified
				result.NextAction = "run `coupangctl login` to renew the expired session"
				return result, nil
			case errors.Is(err, core.ErrBrowserAccessDenied):
				result.State = core.AuthAccessBlocked
				result.NextAction = "retry later, or explicitly run `coupangctl auth verify --headed` when an interactive check is acceptable"
				if status.AccessBlockedAction != "" {
					result.NextAction = status.AccessBlockedAction
				}
				return result, nil
			}
			return core.AuthStatus{}, err
		}
		result.State = core.AuthVerified
		result.VerificationScope = core.AuthVerificationSession
		result.NextAction = verifiedSessionNextAction
	}
	return result, nil
}

func (s *Service) Login(ctx context.Context, request core.LoginRequest) (core.LoginResult, error) {
	if err := request.Validate(); err != nil {
		return core.LoginResult{}, err
	}
	if err := s.browser.Login(ctx, request); err != nil {
		return core.LoginResult{}, err
	}
	result := core.LoginResult{
		State:      core.AuthUnverified,
		Mode:       request.Mode,
		NextAction: "run `coupangctl auth verify` to check the protected read-only session",
	}
	result.State = core.AuthVerified
	result.VerificationScope = core.AuthVerificationSession
	result.NextAction = verifiedSessionNextAction
	return result, nil
}

// Recover performs the non-visible status check before deciding whether a QR
// login is actually necessary. A temporary background access block is not
// treated as an expired session and therefore never opens a surprise window.
func (s *Service) Recover(ctx context.Context, request core.AuthRecoveryRequest) (core.AuthRecoveryResult, error) {
	// An explicitly selected ordinary session owns its interactive lifecycle.
	// Do not start a different native profile to recover that connection.
	if selected, ok := s.browser.(interface {
		RecoverSelectedSession(context.Context, core.AuthRecoveryRequest) (core.AuthRecoveryResult, error)
	}); ok {
		return selected.RecoverSelectedSession(ctx, request)
	}
	status, err := s.Status(ctx)
	if err != nil {
		return core.AuthRecoveryResult{}, err
	}
	result := core.AuthRecoveryResult{
		VerificationScope: status.VerificationScope,
		SchemaVersion:     core.AuthRecoverySchemaVersion,
		BeforeState:       status.State,
		State:             status.State,
		NextAction:        status.NextAction,
	}
	if status.State == core.AuthVerified || status.State == core.AuthAccessBlocked {
		return result, nil
	}
	if !request.Confirmed {
		return core.AuthRecoveryResult{}, core.ErrInteractiveConfirmationRequired
	}
	login, err := s.Login(ctx, core.LoginRequest{Mode: core.LoginModeQR})
	if err != nil {
		return core.AuthRecoveryResult{}, err
	}
	result.State = login.State
	result.VerificationScope = login.VerificationScope
	result.VisibleBrowserOpened = true
	result.Mode = login.Mode
	result.NextAction = login.NextAction
	return result, nil
}
