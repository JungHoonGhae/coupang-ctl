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

func (s *Service) Verify(ctx context.Context) (core.AuthStatus, error) {
	status, err := s.browser.Inspect(ctx)
	if err != nil {
		return core.AuthStatus{}, err
	}
	if err := s.browser.Verify(ctx); err != nil {
		return core.AuthStatus{}, err
	}
	return core.AuthStatus{
		State:          core.AuthVerified,
		Browser:        status.Name,
		ProfilePresent: true,
		CheckedAt:      s.now().UTC(),
		NextAction:     "the read-only browser session is available",
	}, nil
}

type BrowserStatus struct {
	Name           string
	ProfilePresent bool
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
		NextAction:     "run `coupangctl auth login` on an interactive desktop",
	}
	if status.ProfilePresent {
		if err := s.browser.Verify(ctx); err != nil {
			switch {
			case errors.Is(err, core.ErrAuthenticationRequired):
				result.State = core.AuthUnverified
				result.NextAction = "run `coupangctl auth login` to renew the expired session"
				return result, nil
			case errors.Is(err, core.ErrBrowserAccessDenied):
				result.State = core.AuthAccessBlocked
				result.NextAction = "retry later, or explicitly run `coupangctl auth verify --headed` when an interactive check is acceptable"
				return result, nil
			}
			return core.AuthStatus{}, err
		}
		result.State = core.AuthVerified
		result.NextAction = "the read-only browser session is available"
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
	result.NextAction = "the protected read-only browser session is available"
	return result, nil
}

// Recover performs the non-visible status check before deciding whether a QR
// login is actually necessary. A temporary background access block is not
// treated as an expired session and therefore never opens a surprise window.
func (s *Service) Recover(ctx context.Context, request core.AuthRecoveryRequest) (core.AuthRecoveryResult, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return core.AuthRecoveryResult{}, err
	}
	result := core.AuthRecoveryResult{
		SchemaVersion: core.AuthRecoverySchemaVersion,
		BeforeState:   status.State,
		State:         status.State,
		NextAction:    status.NextAction,
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
	result.VisibleBrowserOpened = true
	result.Mode = login.Mode
	result.NextAction = login.NextAction
	return result, nil
}
