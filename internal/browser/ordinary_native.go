package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrOrdinaryNativeProtocol = errors.New("invalid ordinary browser native message")
var ErrOrdinaryNativeOrigin = errors.New("ordinary browser extension origin rejected")

type ordinaryNativeJobs interface {
	Next(context.Context) (OrdinaryBridgeRequest, error)
	Complete(context.Context, OrdinaryBridgeResponse) error
}

func runOrdinaryNativeHost(ctx context.Context, origin, allowedExtensionID string, stdin io.Reader, stdout io.Writer, jobs ordinaryNativeJobs) error {
	if err := validateOrdinaryNativeOrigin(origin, allowedExtensionID); err != nil {
		return err
	}
	if stdin == nil || stdout == nil || jobs == nil {
		return ErrOrdinaryNativeProtocol
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		request, err := jobs.Next(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read ordinary browser pending job: %w", err)
		}
		if err := request.Validate(); err != nil {
			return ErrOrdinaryBridgeProtocol
		}
		payload, err := json.Marshal(request)
		if err != nil {
			return ErrOrdinaryBridgeProtocol
		}
		if err := writeOrdinaryNativeFrame(stdout, payload); err != nil {
			return err
		}
		responsePayload, err := readOrdinaryNativeFrame(stdin)
		if err != nil {
			return err
		}
		response, err := decodeOrdinaryBridgeResponseFrame(responsePayload, request.RequestID)
		if err != nil {
			return err
		}
		if err := jobs.Complete(ctx, response); err != nil {
			return fmt.Errorf("complete ordinary browser pending job: %w", err)
		}
	}
}

func readOrdinaryNativeFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("%w: frame header", ErrOrdinaryNativeProtocol)
	}
	size := binary.NativeEndian.Uint32(header)
	if size == 0 || size > ordinaryBridgeMaxFrameBytes {
		return nil, fmt.Errorf("%w: frame size", ErrOrdinaryNativeProtocol)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fmt.Errorf("%w: frame payload", ErrOrdinaryNativeProtocol)
	}
	if !validOrdinaryNativeJSON(payload) {
		return nil, fmt.Errorf("%w: frame JSON", ErrOrdinaryNativeProtocol)
	}
	return payload, nil
}

func writeOrdinaryNativeFrame(writer io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > ordinaryBridgeMaxFrameBytes || !validOrdinaryNativeJSON(payload) {
		return fmt.Errorf("%w: outbound frame", ErrOrdinaryNativeProtocol)
	}
	header := make([]byte, 4)
	binary.NativeEndian.PutUint32(header, uint32(len(payload)))
	if err := writeOrdinaryNativeBytes(writer, header); err != nil {
		return fmt.Errorf("write ordinary browser frame header: %w", err)
	}
	if err := writeOrdinaryNativeBytes(writer, payload); err != nil {
		return fmt.Errorf("write ordinary browser frame payload: %w", err)
	}
	return nil
}

func validOrdinaryNativeJSON(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func writeOrdinaryNativeBytes(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func validateOrdinaryNativeOrigin(origin, allowedExtensionID string) error {
	if !validChromeExtensionID(allowedExtensionID) || origin != "chrome-extension://"+allowedExtensionID+"/" {
		return ErrOrdinaryNativeOrigin
	}
	return nil
}

func validChromeExtensionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if character < 'a' || character > 'p' {
			return false
		}
	}
	return true
}
