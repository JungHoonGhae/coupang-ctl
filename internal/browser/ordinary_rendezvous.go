package browser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const ordinaryRendezvousTokenBytes = 32
const ordinaryRendezvousMaxMetadataBytes = 4 << 10
const ordinaryRendezvousLifetime = 2 * time.Minute
const ordinaryRendezvousFilename = "ordinary-browser-rendezvous.json"
const ordinaryRendezvousOperationTimeout = 30 * time.Second

var ErrOrdinaryRendezvous = errors.New("ordinary browser bridge rendezvous unavailable")
var errOrdinaryRendezvousNotOwner = errors.New("ordinary browser bridge rendezvous ownership changed")

type ordinaryRendezvousMetadata struct {
	SchemaVersion int       `json:"schema_version"`
	Address       string    `json:"address"`
	Token         string    `json:"token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type ordinaryRendezvousMessage struct {
	SchemaVersion int    `json:"schema_version"`
	Type          string `json:"type"`
	Token         string `json:"token,omitempty"`
}

type ordinaryBridgeRendezvous struct {
	mu       sync.Mutex
	listener net.Listener
	conn     net.Conn
	path     string
	token    string
	expires  time.Time
	now      func() time.Time
	closed   bool
}

type ordinaryRendezvousJobs struct {
	mu        sync.Mutex
	conn      net.Conn
	pendingID string
	closed    bool
}

// OrdinaryBrowserBridge is the CLI-side seam for one explicitly paired
// ordinary-browser session. Its implementation keeps rendezvous credentials
// and transport details private.
type OrdinaryBrowserBridge struct {
	rendezvous *ordinaryBridgeRendezvous
}

func StartOrdinaryBrowserBridge(stateDir string) (*OrdinaryBrowserBridge, error) {
	rendezvous, err := startOrdinaryBridgeRendezvous(stateDir, time.Now, rand.Reader)
	if err != nil {
		return nil, err
	}
	return &OrdinaryBrowserBridge{rendezvous: rendezvous}, nil
}

func (b *OrdinaryBrowserBridge) RoundTrip(ctx context.Context, request OrdinaryBridgeRequest) (OrdinaryBridgeResponse, error) {
	if b == nil || b.rendezvous == nil {
		return OrdinaryBridgeResponse{}, ErrOrdinaryRendezvous
	}
	return b.rendezvous.RoundTrip(ctx, request)
}

func (b *OrdinaryBrowserBridge) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	requestID, err := newOrdinaryBridgeRequestID(rand.Reader)
	if err != nil {
		return core.OrderPage{}, err
	}
	request := OrdinaryBridgeRequest{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Operation:     OrdinaryBridgeReadOrders,
	}
	if cursor != nil {
		cursorCopy := *cursor
		request.Cursor = &cursorCopy
	}
	response, err := b.RoundTrip(ctx, request)
	if err != nil {
		return core.OrderPage{}, err
	}
	page, err := decodeOrdinaryBridgeResponse(response, requestID)
	if err != nil {
		return core.OrderPage{}, err
	}
	return *page, nil
}

func (b *OrdinaryBrowserBridge) Close() error {
	if b == nil || b.rendezvous == nil {
		return nil
	}
	return b.rendezvous.Close()
}

// RunOrdinaryBrowserNativeHost is the Chrome-spawned host seam. It validates
// the exact extension origin before opening the private CLI rendezvous.
func RunOrdinaryBrowserNativeHost(ctx context.Context, stateDir, origin, allowedExtensionID string, stdin io.Reader, stdout io.Writer) error {
	if err := validateOrdinaryNativeOrigin(origin, allowedExtensionID); err != nil {
		return err
	}
	if stdin == nil || stdout == nil {
		return ErrOrdinaryNativeProtocol
	}
	jobs, err := connectOrdinaryBridgeRendezvous(ctx, stateDir, time.Now)
	if err != nil {
		return err
	}
	defer jobs.Close()
	return runOrdinaryNativeHost(ctx, origin, allowedExtensionID, stdin, stdout, jobs)
}

func startOrdinaryBridgeRendezvous(stateDir string, now func() time.Time, entropy io.Reader) (*ordinaryBridgeRendezvous, error) {
	if !filepath.IsAbs(stateDir) || now == nil {
		return nil, ErrOrdinaryRendezvous
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	failed := true
	defer func() {
		if failed {
			_ = listener.Close()
		}
	}()

	tokenBytes := make([]byte, ordinaryRendezvousTokenBytes)
	if _, err := io.ReadFull(entropy, tokenBytes); err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	metadata := ordinaryRendezvousMetadata{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		Address:       listener.Addr().String(),
		Token:         token,
		ExpiresAt:     now().Add(ordinaryRendezvousLifetime).UTC(),
	}
	if !validOrdinaryRendezvousMetadata(metadata, now()) {
		return nil, ErrOrdinaryRendezvous
	}
	payload, err := json.Marshal(metadata)
	if err != nil || len(payload) > ordinaryRendezvousMaxMetadataBytes {
		return nil, ErrOrdinaryRendezvous
	}
	path := ordinaryRendezvousPath(stateDir)
	if err := removeExpiredOrdinaryRendezvous(path, now()); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	created := true
	defer func() {
		if created {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return nil, ErrOrdinaryRendezvous
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, ErrOrdinaryRendezvous
	}
	if err := file.Close(); err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	created = false
	failed = false
	return &ordinaryBridgeRendezvous{listener: listener, path: path, token: token, expires: metadata.ExpiresAt, now: now}, nil
}

func (r *ordinaryBridgeRendezvous) RoundTrip(ctx context.Context, request OrdinaryBridgeRequest) (OrdinaryBridgeResponse, error) {
	if err := request.Validate(); err != nil {
		return OrdinaryBridgeResponse{}, ErrOrdinaryBridgeProtocol
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return OrdinaryBridgeResponse{}, ErrOrdinaryRendezvous
	}
	if r.conn == nil {
		conn, err := r.accept(ctx)
		if err != nil {
			return OrdinaryBridgeResponse{}, err
		}
		r.conn = conn
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return OrdinaryBridgeResponse{}, ErrOrdinaryBridgeProtocol
	}
	restoreDeadline := ordinaryConnContextUntil(ctx, r.conn, time.Now().Add(ordinaryRendezvousOperationTimeout))
	defer restoreDeadline()
	if err := writeOrdinaryNativeFrame(r.conn, payload); err != nil {
		return OrdinaryBridgeResponse{}, ordinaryRendezvousIOError(ctx)
	}
	responsePayload, err := readOrdinaryNativeFrame(r.conn)
	if err != nil {
		return OrdinaryBridgeResponse{}, ordinaryRendezvousIOError(ctx)
	}
	response, err := decodeOrdinaryBridgeResponseFrame(responsePayload, request.RequestID)
	if err != nil {
		return OrdinaryBridgeResponse{}, err
	}
	return response, nil
}

func (r *ordinaryBridgeRendezvous) accept(ctx context.Context) (net.Conn, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !r.now().Before(r.expires) {
			return nil, ErrOrdinaryRendezvous
		}
		restoreDeadline := ordinaryListenerContext(ctx, r.listener, r.expires)
		conn, err := r.listener.Accept()
		restoreDeadline()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrOrdinaryRendezvous
		}
		if err := authenticateOrdinaryRendezvousServer(ctx, conn, r.token, r.expires); err != nil {
			_ = conn.Close()
			continue
		}
		if err := removeOwnedOrdinaryRendezvous(r.path, r.token); err != nil {
			_ = conn.Close()
			return nil, ErrOrdinaryRendezvous
		}
		_ = r.listener.Close()
		r.listener = nil
		return conn, nil
	}
}

func (r *ordinaryBridgeRendezvous) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.conn != nil {
		_ = r.conn.Close()
	}
	if r.listener != nil {
		_ = r.listener.Close()
	}
	if err := removeOwnedOrdinaryRendezvous(r.path, r.token); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errOrdinaryRendezvousNotOwner) {
		return ErrOrdinaryRendezvous
	}
	return nil
}

func connectOrdinaryBridgeRendezvous(ctx context.Context, stateDir string, now func() time.Time) (*ordinaryRendezvousJobs, error) {
	metadata, err := loadOrdinaryRendezvous(ordinaryRendezvousPath(stateDir), now)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp4", metadata.Address)
	if err != nil {
		return nil, ErrOrdinaryRendezvous
	}
	if err := authenticateOrdinaryRendezvousClient(ctx, conn, metadata.Token, metadata.ExpiresAt); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &ordinaryRendezvousJobs{conn: conn}, nil
}

func (j *ordinaryRendezvousJobs) Next(ctx context.Context) (OrdinaryBridgeRequest, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.pendingID != "" {
		return OrdinaryBridgeRequest{}, ErrOrdinaryRendezvous
	}
	restoreDeadline := ordinaryConnContext(ctx, j.conn)
	defer restoreDeadline()
	payload, err := readOrdinaryNativeFrame(j.conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return OrdinaryBridgeRequest{}, io.EOF
		}
		return OrdinaryBridgeRequest{}, ordinaryRendezvousIOError(ctx)
	}
	request, err := decodeOrdinaryBridgeRequestFrame(payload)
	if err != nil {
		return OrdinaryBridgeRequest{}, err
	}
	j.pendingID = request.RequestID
	return request, nil
}

func (j *ordinaryRendezvousJobs) Complete(ctx context.Context, response OrdinaryBridgeResponse) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.pendingID == "" || response.Validate(j.pendingID) != nil {
		return ErrOrdinaryBridgeProtocol
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return ErrOrdinaryBridgeProtocol
	}
	restoreDeadline := ordinaryConnContext(ctx, j.conn)
	defer restoreDeadline()
	if err := writeOrdinaryNativeFrame(j.conn, payload); err != nil {
		return ordinaryRendezvousIOError(ctx)
	}
	j.pendingID = ""
	return nil
}

func (j *ordinaryRendezvousJobs) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	return j.conn.Close()
}

func authenticateOrdinaryRendezvousServer(ctx context.Context, conn net.Conn, token string, expires time.Time) error {
	restoreDeadline := ordinaryConnContextUntil(ctx, conn, expires)
	defer restoreDeadline()
	payload, err := readOrdinaryNativeFrame(conn)
	if err != nil {
		return ErrOrdinaryRendezvous
	}
	var message ordinaryRendezvousMessage
	if decodeOrdinaryBridgeJSON(payload, &message) != nil || message.SchemaVersion != OrdinaryBridgeSchemaVersion || message.Type != "authenticate" || !sameOrdinaryRendezvousToken(message.Token, token) {
		return ErrOrdinaryRendezvous
	}
	acknowledgement, _ := json.Marshal(ordinaryRendezvousMessage{SchemaVersion: OrdinaryBridgeSchemaVersion, Type: "authenticated"})
	if err := writeOrdinaryNativeFrame(conn, acknowledgement); err != nil {
		return ErrOrdinaryRendezvous
	}
	return nil
}

func authenticateOrdinaryRendezvousClient(ctx context.Context, conn net.Conn, token string, expires time.Time) error {
	restoreDeadline := ordinaryConnContextUntil(ctx, conn, expires)
	defer restoreDeadline()
	payload, _ := json.Marshal(ordinaryRendezvousMessage{SchemaVersion: OrdinaryBridgeSchemaVersion, Type: "authenticate", Token: token})
	if err := writeOrdinaryNativeFrame(conn, payload); err != nil {
		return ordinaryRendezvousIOError(ctx)
	}
	acknowledgementPayload, err := readOrdinaryNativeFrame(conn)
	if err != nil {
		return ordinaryRendezvousIOError(ctx)
	}
	var acknowledgement ordinaryRendezvousMessage
	if decodeOrdinaryBridgeJSON(acknowledgementPayload, &acknowledgement) != nil || acknowledgement.SchemaVersion != OrdinaryBridgeSchemaVersion || acknowledgement.Type != "authenticated" || acknowledgement.Token != "" {
		return ErrOrdinaryRendezvous
	}
	return nil
}

func loadOrdinaryRendezvous(path string, now func() time.Time) (ordinaryRendezvousMetadata, error) {
	if now == nil {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	metadata, err := readOrdinaryRendezvousMetadata(path)
	if err != nil || !validOrdinaryRendezvousMetadata(metadata, now()) {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	return metadata, nil
}

func readOrdinaryRendezvousMetadata(path string) (ordinaryRendezvousMetadata, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ordinaryRendezvousMetadata{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	file, err := os.Open(path)
	if err != nil {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, ordinaryRendezvousMaxMetadataBytes+1))
	if err != nil || len(payload) > ordinaryRendezvousMaxMetadataBytes {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	var metadata ordinaryRendezvousMetadata
	if decodeOrdinaryBridgeJSON(payload, &metadata) != nil || !validOrdinaryRendezvousMetadataShape(metadata) {
		return ordinaryRendezvousMetadata{}, ErrOrdinaryRendezvous
	}
	return metadata, nil
}

func validOrdinaryRendezvousMetadata(metadata ordinaryRendezvousMetadata, now time.Time) bool {
	return validOrdinaryRendezvousMetadataShape(metadata) && metadata.ExpiresAt.After(now) && !metadata.ExpiresAt.After(now.Add(ordinaryRendezvousLifetime))
}

func validOrdinaryRendezvousMetadataShape(metadata ordinaryRendezvousMetadata) bool {
	address, err := netip.ParseAddrPort(metadata.Address)
	if err != nil || address.Addr() != netip.MustParseAddr("127.0.0.1") || address.Port() == 0 {
		return false
	}
	token, err := base64.RawURLEncoding.DecodeString(metadata.Token)
	return metadata.SchemaVersion == OrdinaryBridgeSchemaVersion && err == nil && len(token) == ordinaryRendezvousTokenBytes && !metadata.ExpiresAt.IsZero()
}

func ordinaryRendezvousPath(stateDir string) string {
	return filepath.Join(stateDir, ordinaryRendezvousFilename)
}

func removeExpiredOrdinaryRendezvous(path string, now time.Time) error {
	metadata, err := readOrdinaryRendezvousMetadata(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || metadata.ExpiresAt.After(now) {
		return ErrOrdinaryRendezvous
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrOrdinaryRendezvous
	}
	return nil
}

func removeOwnedOrdinaryRendezvous(path, token string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	payload, readErr := io.ReadAll(io.LimitReader(file, ordinaryRendezvousMaxMetadataBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(payload) > ordinaryRendezvousMaxMetadataBytes {
		return ErrOrdinaryRendezvous
	}
	var metadata ordinaryRendezvousMetadata
	if decodeOrdinaryBridgeJSON(payload, &metadata) != nil {
		return ErrOrdinaryRendezvous
	}
	if !sameOrdinaryRendezvousToken(metadata.Token, token) {
		return errOrdinaryRendezvousNotOwner
	}
	return os.Remove(path)
}

func sameOrdinaryRendezvousToken(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func ordinaryConnContext(ctx context.Context, conn net.Conn) func() {
	return ordinaryConnContextUntil(ctx, conn, time.Time{})
}

func ordinaryConnContextUntil(ctx context.Context, conn net.Conn, limit time.Time) func() {
	deadline := limit
	if contextDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || contextDeadline.Before(deadline)) {
		deadline = contextDeadline
	}
	if !deadline.IsZero() {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	return func() {
		stop()
		_ = conn.SetDeadline(time.Time{})
	}
}

func ordinaryListenerContext(ctx context.Context, listener net.Listener, expires time.Time) func() {
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		return func() {}
	}
	deadline := expires
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = tcpListener.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = tcpListener.SetDeadline(time.Now()) })
	return func() {
		stop()
		_ = tcpListener.SetDeadline(time.Time{})
	}
}

func ordinaryRendezvousIOError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrOrdinaryRendezvous
}
