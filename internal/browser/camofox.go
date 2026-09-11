package browser

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrCamofoxUnavailable = core.WithErrorCode("camofox_unavailable", errors.New("Camofox runtime unavailable; check the dedicated runtime configuration (no browser fallback)"))
var ErrCamofoxUnsupported = core.WithErrorCode("camofox_operation_unsupported", errors.New("operation not supported by the Camofox read-only adapter"))

type CamofoxError struct{ Reason string }

func (e *CamofoxError) Error() string { return "Camofox runtime: " + e.Reason }
func (e *CamofoxError) Unwrap() error {
	code := "camofox_request_failed"
	if e.Reason == "document_changed" || e.Reason == "timeout" {
		code = "camofox_" + e.Reason
	}
	return core.WithErrorCode(code, ErrCamofoxUnavailable)
}

// CamofoxConfig contains installation paths, never authentication material.
// Profile storage is owned by this adapter, independently of Aside or Chrome.
type CamofoxConfig struct {
	SchemaVersion  int    `json:"schema_version"`
	Node           string `json:"node"`
	Server         string `json:"server"`
	EngineDir      string `json:"engine_dir"`
	UserID         string `json:"user_id"`
	DefaultSearch  bool   `json:"default_search"`
	DefaultBrowser bool   `json:"default_browser,omitempty"`
}

func ReadCamofoxConfig(stateDir string) (CamofoxConfig, error) {
	var c CamofoxConfig
	f, err := os.Open(filepath.Join(stateDir, "camofox.json"))
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || !validCamofoxConfig(c) {
		return CamofoxConfig{}, ErrCamofoxUnavailable
	}
	return c, nil
}

func validCamofoxConfig(c CamofoxConfig) bool {
	return c.SchemaVersion == 1 && filepath.IsAbs(c.Node) && filepath.IsAbs(c.Server) && filepath.IsAbs(c.EngineDir) && regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(c.UserID)
}

func SaveCamofoxConfig(stateDir string, c CamofoxConfig) error {
	if !validCamofoxConfig(c) {
		return ErrCamofoxUnavailable
	}
	if err := CheckCamofoxInstallation(c); err != nil {
		return err
	}
	if old, err := ReadCamofoxConfig(stateDir); err == nil && old.UserID != c.UserID {
		return ErrCamofoxUnavailable
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrCamofoxUnavailable
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return ErrCamofoxUnavailable
	}
	f, err := os.CreateTemp(stateDir, ".camofox-config-*")
	if err != nil {
		return ErrCamofoxUnavailable
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(c); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrCamofoxUnavailable
	}
	if os.Rename(f.Name(), filepath.Join(stateDir, "camofox.json")) != nil {
		return ErrCamofoxUnavailable
	}
	return nil
}

//go:embed camofox_runner.mjs
var camofoxRunner string

type Camofox struct {
	stateDir string
	cfg      CamofoxConfig
	run      func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error)
}

func NewCamofox(stateDir string, cfg CamofoxConfig) *Camofox {
	c := &Camofox{stateDir: stateDir, cfg: cfg}
	c.run = func(ctx context.Context, script, mode string, present core.QRLinkPresenter) ([]byte, error) {
		return c.runProgram(ctx, script, mode, present)
	}
	return c
}

func (c *Camofox) runProgram(ctx context.Context, script, mode string, present core.QRLinkPresenter) ([]byte, error) {
	cfg, stateDir := c.cfg, c.stateDir
	profile := filepath.Join(stateDir, "camofox", "profiles")
	if os.MkdirAll(profile, 0o700) != nil {
		return nil, ErrCamofoxUnavailable
	}
	lock, err := acquireProfileLock(profile)
	if err != nil {
		return nil, err
	}
	defer lock.release()
	input, err := json.Marshal(struct {
		Config   CamofoxConfig `json:"config"`
		StateDir string        `json:"state_dir"`
		Script   string        `json:"script"`
		Mode     string        `json:"mode"`
	}{cfg, filepath.Join(stateDir, "camofox"), script, mode})
	if err != nil {
		return nil, ErrCamofoxUnavailable
	}
	cmd := exec.CommandContext(ctx, cfg.Node, "--input-type=module", "-e", camofoxRunner)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = io.Discard
	// Give the runner time to checkpoint its own profile and stop its child.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 12 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return nil, ErrCamofoxUnavailable
	}
	data, readErr := consumeCamofoxOutput(ctx, stdout, present)
	if readErr != nil {
		_ = cmd.Cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrCamofoxUnavailable
	}
	var envelope struct {
		RuntimeError *struct {
			Reason string `json:"reason"`
		} `json:"runtime_error"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.RuntimeError != nil {
		reason := envelope.RuntimeError.Reason
		if reason != "document_changed" && reason != "timeout" {
			reason = "request_failed"
		}
		return nil, &CamofoxError{Reason: reason}
	}
	return data, nil
}

func consumeCamofoxOutput(ctx context.Context, output io.Reader, present core.QRLinkPresenter) ([]byte, error) {
	scanner := bufio.NewScanner(io.LimitReader(output, (1<<20)+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var result []byte
	presented := false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "COUPANGCTL_QR ") || strings.HasPrefix(line, "COUPANGCTL_QR_IMAGE ") {
			var value struct {
				URL          string `json:"url"`
				ApprovalCode string `json:"approvalCode"`
				PNG          string `json:"png"`
			}
			_, encoded, _ := strings.Cut(line, " ")
			if present == nil || presented || result != nil || json.Unmarshal([]byte(encoded), &value) != nil {
				return nil, ErrDocumentProtocol
			}
			if strings.HasPrefix(line, "COUPANGCTL_QR_IMAGE ") {
				var err error
				value.URL, err = decodeCamofoxQRImage(value.PNG)
				if err != nil {
					return nil, err
				}
			}
			link := core.QRLoginLink{URL: value.URL, ApprovalCode: value.ApprovalCode}
			if !validQRLoginLink(link) {
				return nil, ErrQRLinkUnavailable
			}
			if err := present(ctx, link); err != nil {
				return nil, ErrQRLinkUnavailable
			}
			presented = true
		} else if strings.HasPrefix(line, "COUPANGCTL_RESULT ") {
			if result != nil {
				return nil, ErrDocumentProtocol
			}
			result = []byte(strings.TrimPrefix(line, "COUPANGCTL_RESULT "))
		} else {
			return nil, ErrDocumentProtocol
		}
	}
	if scanner.Err() != nil || !json.Valid(result) {
		return nil, ErrCamofoxUnavailable
	}
	return result, nil
}

func (c *Camofox) reader(mode string) *documentBrowser {
	return &documentBrowser{run: func(ctx context.Context, script string) ([]byte, error) { return c.run(ctx, script, mode, nil) }}
}

func (c *Camofox) FetchProductSearch(ctx context.Context, req core.ProductSearchRequest) ([]byte, error) {
	document, err := c.reader("search").FetchProductSearch(ctx, req)
	if err != nil {
		return nil, err
	}
	var result struct {
		searchDocument
		Coverage core.ProductCoverage `json:"coverage"`
	}
	if json.Unmarshal(document, &result) != nil {
		return nil, ErrDocumentProtocol
	}
	result.Coverage.Source = "camofox_search_document"
	return json.Marshal(result)
}

func (*Camofox) AddProductToCart(context.Context, core.CartAddRequest) ([]byte, error) {
	return nil, ErrCamofoxUnsupported
}
