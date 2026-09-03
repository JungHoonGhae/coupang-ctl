package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const localPageRenderTimeout = 20 * time.Second

var ErrLocalPageRenderFailed = errors.New("local page render failed")

type LocalPageRenderer struct{}

func NewLocalPageRenderer() *LocalPageRenderer {
	return &LocalPageRenderer{}
}

func (*LocalPageRenderer) RenderPNG(ctx context.Context, htmlPath, outputPath string, width, height int) error {
	if !filepath.IsAbs(htmlPath) || !filepath.IsAbs(outputPath) || filepath.Ext(outputPath) != ".png" || width < 320 || width > 4096 || height < 320 || height > 4096 {
		return errors.New("invalid local page render request")
	}
	info, err := os.Stat(htmlPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("local page render source unavailable")
	}
	profileDirectory, err := os.MkdirTemp(filepath.Dir(outputPath), "chrome-render-profile-")
	if err != nil {
		return errors.New("create local page browser profile")
	}
	defer os.RemoveAll(profileDirectory)
	if err := os.Chmod(profileDirectory, 0o700); err != nil {
		return errors.New("secure local page browser profile")
	}
	executable, err := NewNative("").discover()
	if err != nil {
		return err
	}
	renderContext, cancel := context.WithTimeout(ctx, localPageRenderTimeout)
	defer cancel()
	command := browserCommand(renderContext, executable, localPageScreenshotArguments(htmlPath, outputPath, profileDirectory, width, height)...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return ErrLocalPageRenderFailed
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastSize := int64(-1)
	stableChecks := 0
	for {
		select {
		case err := <-done:
			if err != nil || !nonEmptyRegularFile(outputPath) {
				return ErrLocalPageRenderFailed
			}
			return nil
		case <-ticker.C:
			info, err := os.Stat(outputPath)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 1 {
				continue
			}
			if info.Size() == lastSize {
				stableChecks++
			} else {
				lastSize = info.Size()
				stableChecks = 0
			}
			if stableChecks < 3 {
				continue
			}
			_ = command.Process.Kill()
			<-done
			return nil
		case <-renderContext.Done():
			_ = command.Process.Kill()
			<-done
			return ErrLocalPageRenderFailed
		}
	}
}

func nonEmptyRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func localPageScreenshotArguments(htmlPath, outputPath, profileDirectory string, width, height int) []string {
	target := (&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String()
	return []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--noerrdialogs",
		"--force-device-scale-factor=1",
		"--run-all-compositor-stages-before-draw",
		"--virtual-time-budget=1500",
		"--user-data-dir=" + profileDirectory,
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--screenshot=" + outputPath,
		target,
	}
}
