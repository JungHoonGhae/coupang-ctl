package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// PingOrdinaryBrowserNativeHost exercises the authenticated rendezvous,
// Native Messaging framing, exact extension origin, and typed empty-page
// response entirely with synthetic data. It does not launch a browser or read
// any Coupang, profile, cookie, or order data.
func PingOrdinaryBrowserNativeHost(ctx context.Context, stateDir string) error {
	if ctx == nil {
		return errors.New("native-host ping requires a context")
	}
	bridge, err := StartOrdinaryBrowserBridge(stateDir)
	if err != nil {
		return fmt.Errorf("start native-host ping rendezvous: %w", err)
	}
	defer bridge.Close()

	extensionToHostReader, extensionToHostWriter := io.Pipe()
	hostToExtensionReader, hostToExtensionWriter := io.Pipe()
	defer extensionToHostReader.Close()
	defer extensionToHostWriter.Close()
	defer hostToExtensionReader.Close()
	defer hostToExtensionWriter.Close()

	hostDone := make(chan error, 1)
	go func() {
		hostErr := RunOrdinaryBrowserNativeHost(
			ctx,
			stateDir,
			"chrome-extension://"+OrdinaryBrowserExtensionID+"/",
			OrdinaryBrowserExtensionID,
			extensionToHostReader,
			hostToExtensionWriter,
		)
		_ = hostToExtensionWriter.CloseWithError(hostErr)
		hostDone <- hostErr
	}()

	type fetchResult struct {
		page core.OrderPage
		err  error
	}
	fetched := make(chan fetchResult, 1)
	go func() {
		page, fetchErr := bridge.FetchPage(ctx, nil)
		fetched <- fetchResult{page: page, err: fetchErr}
	}()

	requestPayload, err := readOrdinaryNativeFrameContext(ctx, hostToExtensionReader)
	if err != nil {
		return fmt.Errorf("read native-host ping request: %w", err)
	}
	request, err := decodeOrdinaryBridgeRequestFrame(requestPayload)
	if err != nil || request.Operation != OrdinaryBridgeReadOrders || request.Cursor != nil {
		return ErrOrdinaryBridgeProtocol
	}
	responsePayload, err := json.Marshal(OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     request.RequestID,
		Status:        OrdinaryBridgeOK,
		Page:          &core.OrderPage{Orders: []core.Order{}},
	})
	if err != nil {
		return ErrOrdinaryBridgeProtocol
	}
	if err := writeOrdinaryNativeFrame(extensionToHostWriter, responsePayload); err != nil {
		return fmt.Errorf("write native-host ping response: %w", err)
	}

	var result fetchResult
	select {
	case result = <-fetched:
	case <-ctx.Done():
		return ctx.Err()
	}
	if result.err != nil {
		return fmt.Errorf("complete native-host ping: %w", result.err)
	}
	if len(result.page.Orders) != 0 || result.page.Next != nil {
		return ErrOrdinaryBridgeProtocol
	}
	if err := bridge.Close(); err != nil {
		return fmt.Errorf("close native-host ping rendezvous: %w", err)
	}
	select {
	case err := <-hostDone:
		if err != nil {
			return fmt.Errorf("stop native-host ping: %w", err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
