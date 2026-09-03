package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type notifyingWriter struct {
	once    chan struct{}
	inner   io.Writer
	written bool
}

func (w *notifyingWriter) Write(payload []byte) (int, error) {
	if !w.written {
		w.written = true
		close(w.once)
	}
	return w.inner.Write(payload)
}

type fakeOrdinaryNativeJobs struct {
	requests  []OrdinaryBridgeRequest
	completed []OrdinaryBridgeResponse
}

func (f *fakeOrdinaryNativeJobs) Next(context.Context) (OrdinaryBridgeRequest, error) {
	if len(f.requests) == 0 {
		return OrdinaryBridgeRequest{}, io.EOF
	}
	request := f.requests[0]
	f.requests = f.requests[1:]
	return request, nil
}

func (f *fakeOrdinaryNativeJobs) Complete(_ context.Context, response OrdinaryBridgeResponse) error {
	f.completed = append(f.completed, response)
	return nil
}

func TestOrdinaryNativeFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"schema_version":1,"type":"synthetic"}`)
	var framed bytes.Buffer
	if err := writeOrdinaryNativeFrame(&framed, payload); err != nil {
		t.Fatal(err)
	}
	got, err := readOrdinaryNativeFrame(&framed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("frame payload = %q, want %q", got, payload)
	}
}

func TestOrdinaryNativeFrameFailsClosed(t *testing.T) {
	oversizedHeader := make([]byte, 4)
	binary.NativeEndian.PutUint32(oversizedHeader, ordinaryBridgeMaxFrameBytes+1)
	truncatedHeader := make([]byte, 4)
	binary.NativeEndian.PutUint32(truncatedHeader, 8)

	for name, input := range map[string][]byte{
		"empty":        {0, 0, 0, 0},
		"oversized":    oversizedHeader,
		"truncated":    append(truncatedHeader, []byte(`{}`)...),
		"invalid json": append(nativeFrameHeader(8), []byte(`not-json`)...),
		"json scalar":  append(nativeFrameHeader(4), []byte(`null`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readOrdinaryNativeFrame(bytes.NewReader(input)); !errors.Is(err, ErrOrdinaryNativeProtocol) {
				t.Fatalf("error = %v, want ErrOrdinaryNativeProtocol", err)
			}
		})
	}

	if err := writeOrdinaryNativeFrame(io.Discard, []byte(`[]`)); !errors.Is(err, ErrOrdinaryNativeProtocol) {
		t.Fatalf("array write error = %v, want ErrOrdinaryNativeProtocol", err)
	}
	if err := writeOrdinaryNativeFrame(io.Discard, bytes.Repeat([]byte{'x'}, ordinaryBridgeMaxFrameBytes+1)); !errors.Is(err, ErrOrdinaryNativeProtocol) {
		t.Fatalf("oversized write error = %v, want ErrOrdinaryNativeProtocol", err)
	}
}

func TestOrdinaryNativeOriginRequiresExactReleasedExtension(t *testing.T) {
	extensionID := strings.Repeat("a", 32)
	if err := validateOrdinaryNativeOrigin("chrome-extension://"+extensionID+"/", extensionID); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{
		"", "chrome-extension://" + extensionID, "chrome-extension://" + extensionID + "/page.html",
		"https://" + extensionID + "/", "chrome-extension://" + strings.Repeat("b", 32) + "/",
	} {
		if err := validateOrdinaryNativeOrigin(candidate, extensionID); !errors.Is(err, ErrOrdinaryNativeOrigin) {
			t.Fatalf("candidate %q error = %v, want ErrOrdinaryNativeOrigin", candidate, err)
		}
	}
	if err := validateOrdinaryNativeOrigin("chrome-extension://"+extensionID+"/", "*"); !errors.Is(err, ErrOrdinaryNativeOrigin) {
		t.Fatalf("wildcard extension id error = %v, want ErrOrdinaryNativeOrigin", err)
	}
}

func TestOrdinaryNativeHostRelaysValidatedFrames(t *testing.T) {
	extensionID := strings.Repeat("a", 32)
	requestID := strings.Repeat("b", 32)
	request := OrdinaryBridgeRequest{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders}
	response := OrdinaryBridgeResponse{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeOK, Page: &core.OrderPage{Orders: []core.Order{}}}
	responsePayload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var stdin bytes.Buffer
	if err := writeOrdinaryNativeFrame(&stdin, responsePayload); err != nil {
		t.Fatal(err)
	}
	jobs := &fakeOrdinaryNativeJobs{requests: []OrdinaryBridgeRequest{request}}
	var stdout bytes.Buffer
	if err := runOrdinaryNativeHost(context.Background(), "chrome-extension://"+extensionID+"/", extensionID, &stdin, &stdout, jobs); err != nil {
		t.Fatal(err)
	}
	requestPayload, err := readOrdinaryNativeFrame(&stdout)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeOrdinaryBridgeRequestFrame(requestPayload)
	if err != nil || decoded.RequestID != requestID {
		t.Fatalf("relayed request = %#v, %v", decoded, err)
	}
	if stdout.Len() != 0 || len(jobs.completed) != 1 || jobs.completed[0].RequestID != requestID {
		t.Fatalf("host state stdout=%d completed=%#v", stdout.Len(), jobs.completed)
	}
}

func TestOrdinaryNativeHostRejectsMismatchedResponseBeforeCompletion(t *testing.T) {
	extensionID := strings.Repeat("a", 32)
	requestID := strings.Repeat("b", 32)
	wrongID := strings.Repeat("c", 32)
	response := OrdinaryBridgeResponse{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: wrongID, Status: OrdinaryBridgeUnavailable}
	responsePayload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var stdin bytes.Buffer
	if err := writeOrdinaryNativeFrame(&stdin, responsePayload); err != nil {
		t.Fatal(err)
	}
	jobs := &fakeOrdinaryNativeJobs{requests: []OrdinaryBridgeRequest{{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders}}}
	if err := runOrdinaryNativeHost(context.Background(), "chrome-extension://"+extensionID+"/", extensionID, &stdin, io.Discard, jobs); !errors.Is(err, ErrOrdinaryBridgeProtocol) {
		t.Fatalf("host error = %v, want ErrOrdinaryBridgeProtocol", err)
	}
	if len(jobs.completed) != 0 {
		t.Fatalf("mismatched response completed: %#v", jobs.completed)
	}
}

func TestOrdinaryNativeHostStopsWaitingForExtensionOnContextCancellation(t *testing.T) {
	extensionID := strings.Repeat("a", 32)
	requestID := strings.Repeat("b", 32)
	stdin, extensionWriter := io.Pipe()
	defer extensionWriter.Close()
	wroteRequest := make(chan struct{})
	stdout := &notifyingWriter{once: wroteRequest, inner: io.Discard}
	jobs := &fakeOrdinaryNativeJobs{requests: []OrdinaryBridgeRequest{{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Operation:     OrdinaryBridgeReadOrders,
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runOrdinaryNativeHost(ctx, "chrome-extension://"+extensionID+"/", extensionID, stdin, stdout, jobs)
	}()

	select {
	case <-wroteRequest:
	case <-time.After(time.Second):
		t.Fatal("native host did not send the pending request")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("host error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		_ = extensionWriter.Close()
		t.Fatal("native host remained blocked after context cancellation")
	}
}

func nativeFrameHeader(size uint32) []byte {
	header := make([]byte, 4)
	binary.NativeEndian.PutUint32(header, size)
	return header
}
