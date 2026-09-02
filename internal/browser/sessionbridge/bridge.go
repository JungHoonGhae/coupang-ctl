package sessionbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/session"
)

type Protocol interface {
	Call(context.Context, string, any, any) error
}

type Bridge struct {
	store session.Store
	now   func() time.Time
}

func New(store session.Store, now func() time.Time) *Bridge {
	if now == nil {
		now = time.Now
	}
	return &Bridge{store: store, now: now}
}

func (b *Bridge) Restore(ctx context.Context, protocol Protocol) error {
	state, err := b.store.Load(ctx)
	if errors.Is(err, session.ErrNoSession) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load private browser session: %w", err)
	}
	params := make([]cookieParam, 0, len(state.Cookies))
	for _, stored := range state.Cookies {
		params = append(params, cookieToParam(stored))
	}
	if err := protocol.Call(ctx, "Storage.setCookies", map[string]any{"cookies": params}, nil); err != nil {
		return fmt.Errorf("restore private browser session: %w", err)
	}
	return nil
}

func (b *Bridge) Capture(ctx context.Context, protocol Protocol) error {
	var result struct {
		Cookies []protocolCookie `json:"cookies"`
	}
	if err := protocol.Call(ctx, "Storage.getCookies", struct{}{}, &result); err != nil {
		return fmt.Errorf("capture private browser session: %w", err)
	}
	cookies := make([]session.Cookie, 0, len(result.Cookies))
	for _, captured := range result.Cookies {
		if !isCoupangDomain(captured.Domain) {
			continue
		}
		cookies = append(cookies, session.Cookie{
			Name:         captured.Name,
			Value:        captured.Value,
			Domain:       captured.Domain,
			Path:         captured.Path,
			Expires:      captured.Expires,
			HTTPOnly:     captured.HTTPOnly,
			Secure:       captured.Secure,
			Session:      captured.Session,
			SameSite:     captured.SameSite,
			Priority:     captured.Priority,
			SameParty:    captured.SameParty,
			SourceScheme: captured.SourceScheme,
			SourcePort:   captured.SourcePort,
		})
	}
	if len(cookies) == 0 {
		return session.ErrNoSession
	}
	if err := b.store.Save(ctx, session.State{
		Version: session.SchemaVersion,
		SavedAt: b.now().UTC(),
		Cookies: cookies,
	}); err != nil {
		return fmt.Errorf("save private browser session: %w", err)
	}
	return nil
}

type protocolCookie struct {
	Name         string  `json:"name"`
	Value        string  `json:"value"`
	Domain       string  `json:"domain"`
	Path         string  `json:"path"`
	Expires      float64 `json:"expires"`
	HTTPOnly     bool    `json:"httpOnly"`
	Secure       bool    `json:"secure"`
	Session      bool    `json:"session"`
	SameSite     string  `json:"sameSite"`
	Priority     string  `json:"priority"`
	SameParty    bool    `json:"sameParty"`
	SourceScheme string  `json:"sourceScheme"`
	SourcePort   int     `json:"sourcePort"`
}

type cookieParam struct {
	Name         string   `json:"name"`
	Value        string   `json:"value"`
	Domain       string   `json:"domain"`
	Path         string   `json:"path"`
	Secure       bool     `json:"secure,omitempty"`
	HTTPOnly     bool     `json:"httpOnly,omitempty"`
	SameSite     string   `json:"sameSite,omitempty"`
	Expires      *float64 `json:"expires,omitempty"`
	Priority     string   `json:"priority,omitempty"`
	SameParty    bool     `json:"sameParty,omitempty"`
	SourceScheme string   `json:"sourceScheme,omitempty"`
	SourcePort   int      `json:"sourcePort,omitempty"`
}

func cookieToParam(stored session.Cookie) cookieParam {
	param := cookieParam{
		Name:         stored.Name,
		Value:        stored.Value,
		Domain:       stored.Domain,
		Path:         stored.Path,
		Secure:       stored.Secure,
		HTTPOnly:     stored.HTTPOnly,
		SameSite:     stored.SameSite,
		Priority:     stored.Priority,
		SameParty:    stored.SameParty,
		SourceScheme: stored.SourceScheme,
		SourcePort:   stored.SourcePort,
	}
	if !stored.Session && stored.Expires > 0 {
		expires := stored.Expires
		param.Expires = &expires
	}
	return param
}

func isCoupangDomain(domain string) bool {
	host := strings.TrimPrefix(strings.ToLower(domain), ".")
	return host == "coupang.com" || strings.HasSuffix(host, ".coupang.com")
}
