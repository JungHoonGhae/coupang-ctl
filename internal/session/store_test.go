package session_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/session"
)

func TestFileStoreRoundTripsPrivateBrowserSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "session.json")
	store := session.NewFileStore(path)
	want := session.State{
		Version: session.SchemaVersion,
		SavedAt: time.Date(2026, time.September, 2, 1, 2, 3, 0, time.UTC),
		Cookies: []session.Cookie{{
			Name: "SYNTHETIC_SESSION", Value: "synthetic-secret", Domain: ".coupang.com", Path: "/",
			Expires: 1_900_000_000, HTTPOnly: true, Secure: true, SameSite: "Lax",
		}},
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("session mode = %o, want 600", got)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || !got.SavedAt.Equal(want.SavedAt) || len(got.Cookies) != 1 {
		t.Fatalf("session metadata did not round trip")
	}
	if got.Cookies[0] != want.Cookies[0] {
		t.Fatalf("session cookie did not round trip")
	}
}

func TestFileStoreRejectsNonCoupangCookieAndReportsMissingSession(t *testing.T) {
	store := session.NewFileStore(filepath.Join(t.TempDir(), "session.json"))
	if _, err := store.Load(context.Background()); !errors.Is(err, session.ErrNoSession) {
		t.Fatalf("missing load error = %v, want ErrNoSession", err)
	}
	err := store.Save(context.Background(), session.State{
		Version: session.SchemaVersion,
		SavedAt: time.Now().UTC(),
		Cookies: []session.Cookie{{Name: "SYNTHETIC", Value: "synthetic", Domain: ".example.com", Path: "/"}},
	})
	if !errors.Is(err, session.ErrInvalidSession) {
		t.Fatalf("invalid save error = %v, want ErrInvalidSession", err)
	}
}
