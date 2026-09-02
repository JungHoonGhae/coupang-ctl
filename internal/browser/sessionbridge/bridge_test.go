package sessionbridge_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser/sessionbridge"
	"github.com/JungHoonGhae/coupang-ctl/internal/session"
)

type memoryStore struct {
	state session.State
	saved session.State
}

func (s *memoryStore) Load(context.Context) (session.State, error) { return s.state, nil }
func (s *memoryStore) Save(_ context.Context, state session.State) error {
	s.saved = state
	return nil
}

type storageProtocol struct {
	restoredDomains []string
}

func (p *storageProtocol) Call(_ context.Context, method string, params, result any) error {
	switch method {
	case "Storage.setCookies":
		payload, _ := json.Marshal(params)
		var request struct {
			Cookies []struct {
				Domain string `json:"domain"`
			} `json:"cookies"`
		}
		_ = json.Unmarshal(payload, &request)
		for _, cookie := range request.Cookies {
			p.restoredDomains = append(p.restoredDomains, cookie.Domain)
		}
	case "Storage.getCookies":
		payload := []byte(`{"cookies":[{"name":"SYNTHETIC_SESSION","value":"rotated-synthetic-secret","domain":".coupang.com","path":"/","httpOnly":true,"secure":true,"session":true,"sameSite":"Lax"},{"name":"IGNORED","value":"synthetic","domain":".example.com","path":"/"}]}`)
		return json.Unmarshal(payload, result)
	}
	return nil
}

func TestBridgeRestoresSessionAndPersistsRefreshedCoupangCookies(t *testing.T) {
	fixed := time.Date(2026, time.September, 2, 1, 2, 3, 0, time.UTC)
	store := &memoryStore{state: session.State{
		Version: session.SchemaVersion,
		SavedAt: fixed.Add(-time.Hour),
		Cookies: []session.Cookie{{
			Name: "SYNTHETIC_SESSION", Value: "synthetic-secret", Domain: ".coupang.com", Path: "/", Session: true,
		}},
	}}
	protocol := &storageProtocol{}
	bridge := sessionbridge.New(store, func() time.Time { return fixed })

	if err := bridge.Restore(context.Background(), protocol); err != nil {
		t.Fatal(err)
	}
	if len(protocol.restoredDomains) != 1 || protocol.restoredDomains[0] != ".coupang.com" {
		t.Fatalf("restored domains = %#v", protocol.restoredDomains)
	}
	if err := bridge.Capture(context.Background(), protocol); err != nil {
		t.Fatal(err)
	}
	if len(store.saved.Cookies) != 1 || store.saved.Cookies[0].Domain != ".coupang.com" {
		t.Fatalf("captured session was not restricted to Coupang")
	}
	if store.saved.Cookies[0].Value != "rotated-synthetic-secret" {
		t.Fatal("refreshed session value was not persisted")
	}
	if !store.saved.SavedAt.Equal(fixed) || store.saved.Version != session.SchemaVersion {
		t.Fatalf("captured metadata is invalid")
	}
}
