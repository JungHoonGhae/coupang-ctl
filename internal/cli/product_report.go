package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/recommendationreport"
)

func runProductReport(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	const usage = "usage: coupangctl products report --input PATH|- [--output NEW_HTML_PATH]"
	flags := newFlagSet("products report")
	input := flags.String("input", "", "report v2 JSON file or - for standard input; recommendation v5 required")
	output := flags.String("output", "", "explicitly create a new private HTML file; existing files are never overwritten")
	if isFlagHelp(args) {
		return writeCommandFlagHelp(stdout, flags, usage)
	}
	if err := parseFlags(flags, args, usage); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reader := stdin
	if *input != "-" {
		info, err := os.Stat(*input)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("report input must be a readable regular JSON file")
		}
		if info.Size() > core.ProductReportMaxInputBytes {
			return errors.New("report input exceeds byte limit")
		}
		file, err := os.Open(*input)
		if err != nil {
			return errors.New("cannot read report input")
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, core.ProductReportMaxInputBytes+1))
	if err != nil || len(data) > core.ProductReportMaxInputBytes {
		return errors.New("report input is unreadable or exceeds byte limit")
	}
	var report core.ProductRecommendationReport
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return errors.New("invalid report JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("report input must contain exactly one JSON object")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(*output) != "" {
		result, err := recommendationreport.WriteNewFile(report, *output)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	}
	result, err := recommendationreport.RenderResult(ctx, report)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}
