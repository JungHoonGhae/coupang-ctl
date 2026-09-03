// Command packagemanifests renders release metadata for maintainers. It writes
// only to a new local directory and never publishes to Homebrew or WinGet.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JungHoonGhae/coupang-ctl/internal/packagemanifests"
)

type commandError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	flags := flag.NewFlagSet("packagemanifests", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	tag := flags.String("tag", "", "immutable semantic release tag")
	sourceSHA256 := flags.String("source-sha256", "", "GitHub tagged-source SHA-256")
	windowsAMD64SHA256 := flags.String("windows-amd64-sha256", "", "Windows amd64 release ZIP SHA-256")
	windowsARM64SHA256 := flags.String("windows-arm64-sha256", "", "Windows arm64 release ZIP SHA-256")
	output := flags.String("output", "", "new output directory")
	if err := flags.Parse(os.Args[1:]); err != nil || flags.NArg() != 0 || *output == "" {
		writeError("invalid_request", "required flags: --tag, --source-sha256, --windows-amd64-sha256, --windows-arm64-sha256, --output")
		os.Exit(2)
	}
	bundle, err := packagemanifests.Render(packagemanifests.Request{
		Tag:                *tag,
		SourceSHA256:       *sourceSHA256,
		WindowsAMD64SHA256: *windowsAMD64SHA256,
		WindowsARM64SHA256: *windowsARM64SHA256,
	})
	if err != nil {
		writeError("invalid_release_metadata", "tag or SHA-256 release metadata is invalid")
		os.Exit(1)
	}
	absoluteOutput, err := filepath.Abs(*output)
	if err != nil {
		writeError("invalid_output", "output directory could not be resolved")
		os.Exit(1)
	}
	report, err := packagemanifests.WriteNew(absoluteOutput, bundle)
	if err != nil {
		writeError("output_not_created", "output must be a new writable directory under an existing parent")
		os.Exit(1)
	}
	if err := writeJSON(os.Stdout, report); err != nil {
		fmt.Fprintln(os.Stderr, `{"error":{"code":"output_failed","message":"could not write the structured report"}}`)
		os.Exit(1)
	}
}

func writeError(code, message string) {
	response := commandError{}
	response.Error.Code = code
	response.Error.Message = message
	_ = writeJSON(os.Stderr, response)
}

func writeJSON(file *os.File, value any) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
