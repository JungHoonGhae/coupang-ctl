// Command releasepolicy evaluates the repository's current native-signing
// publication gate. It cannot accept caller-asserted signing state.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("releasepolicy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tag := flags.String("tag", "", "immutable semantic release tag")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *tag == "" {
		writeError(stderr, "invalid_request", "exactly one --tag vX.Y.Z or prerelease tag is required")
		return 2
	}
	policy, err := releasecontract.EvaluateUnsignedPublication(*tag)
	if err != nil {
		writeError(stderr, "invalid_tag", "tag must be an immutable semantic release tag beginning with v")
		return 2
	}
	if err := writeJSON(stdout, policy); err != nil {
		writeError(stderr, "output_failed", "could not write the structured release policy")
		return 1
	}
	if !policy.PublishAllowed {
		return 1
	}
	return 0
}

func writeError(writer io.Writer, code, message string) {
	response := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	response.Error.Code = code
	response.Error.Message = message
	_ = writeJSON(writer, response)
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
