package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = 1
const maxSessionFileSize = 4 << 20
const maxCookies = 512

var ErrNoSession = errors.New("no stored browser session")
var ErrInvalidSession = errors.New("invalid stored browser session")

type Cookie struct {
	Name         string  `json:"name"`
	Value        string  `json:"value"`
	Domain       string  `json:"domain"`
	Path         string  `json:"path"`
	Expires      float64 `json:"expires,omitempty"`
	HTTPOnly     bool    `json:"http_only,omitempty"`
	Secure       bool    `json:"secure,omitempty"`
	Session      bool    `json:"session,omitempty"`
	SameSite     string  `json:"same_site,omitempty"`
	Priority     string  `json:"priority,omitempty"`
	SameParty    bool    `json:"same_party,omitempty"`
	SourceScheme string  `json:"source_scheme,omitempty"`
	SourcePort   int     `json:"source_port,omitempty"`
}

type State struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Cookies []Cookie  `json:"cookies"`
}

type Store interface {
	Load(context.Context) (State, error)
	Save(context.Context, State) error
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, ErrNoSession
	}
	if err != nil {
		return State{}, err
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxSessionFileSize+1))
	if err != nil {
		return State{}, err
	}
	if len(payload) > maxSessionFileSize {
		return State{}, ErrInvalidSession
	}
	var state State
	if err := json.Unmarshal(payload, &state); err != nil || !validState(state) {
		return State{}, ErrInvalidSession
	}
	return state, nil
}

func (s *FileStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validState(state) {
		return ErrInvalidSession
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return ErrInvalidSession
	}
	if len(payload) > maxSessionFileSize {
		return ErrInvalidSession
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".coupangctl-session-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func validState(state State) bool {
	if state.Version != SchemaVersion || state.SavedAt.IsZero() || len(state.Cookies) == 0 || len(state.Cookies) > maxCookies {
		return false
	}
	for _, cookie := range state.Cookies {
		if !validCookie(cookie) {
			return false
		}
	}
	return true
}

func validCookie(cookie Cookie) bool {
	host := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
	if host != "coupang.com" && !strings.HasSuffix(host, ".coupang.com") {
		return false
	}
	if cookie.Name == "" || len(cookie.Name) > 1024 || len(cookie.Value) > 64<<10 {
		return false
	}
	if cookie.Path == "" || !strings.HasPrefix(cookie.Path, "/") {
		return false
	}
	switch cookie.SameSite {
	case "", "Strict", "Lax", "None":
	default:
		return false
	}
	return true
}
