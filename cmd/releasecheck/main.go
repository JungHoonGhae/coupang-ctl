// Command releasecheck verifies the contents of a GoReleaser dist directory.
// It is a development/release tool and is never included in product archives.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

func main() {
	flags := flag.NewFlagSet("releasecheck", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	requireSBOM := flags.Bool("require-sbom", false, "require one checksummed SBOM per archive")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/releasecheck [--require-sbom] DIST_DIRECTORY")
		os.Exit(2)
	}
	if err := releasecontract.VerifyWithOptions(flags.Arg(0), releasecontract.Options{RequireSBOM: *requireSBOM}); err != nil {
		fmt.Fprintln(os.Stderr, "release contract failed:", err)
		os.Exit(1)
	}
	fmt.Println("release contract verified")
}
