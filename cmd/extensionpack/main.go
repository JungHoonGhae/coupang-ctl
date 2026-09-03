// Command extensionpack builds or verifies the reviewed Chrome Web Store ZIP.
// It is a maintainer tool and is not included in coupangctl product archives.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/JungHoonGhae/coupang-ctl/internal/extensionpack"
)

func main() {
	flags := flag.NewFlagSet("extensionpack", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputPath := flags.String("output", "", "build the reviewed extension ZIP at this path")
	verifyPath := flags.String("verify", "", "verify an existing extension ZIP")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 || (*outputPath == "") == (*verifyPath == "") {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/extensionpack (--output PACKAGE.zip | --verify PACKAGE.zip)")
		os.Exit(2)
	}

	var (
		report extensionpack.Report
		err    error
	)
	if *outputPath != "" {
		report, err = extensionpack.Build(*outputPath)
	} else {
		report, err = extensionpack.Verify(*verifyPath)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "extension package failed:", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "encode extension package report:", err)
		os.Exit(1)
	}
}
