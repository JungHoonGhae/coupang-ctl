package recommendationreport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func WriteNewFile(report core.ProductRecommendationReport, outputPath string) (core.ProductRecommendationReportWriteResult, error) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return core.ProductRecommendationReportWriteResult{}, errors.New("report output_path is required")
	}
	html, err := Render(report)
	if err != nil {
		return core.ProductRecommendationReportWriteResult{}, err
	}
	absolute, err := filepath.Abs(outputPath)
	if err != nil {
		return core.ProductRecommendationReportWriteResult{}, err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return core.ProductRecommendationReportWriteResult{}, err
	}
	written, writeErr := file.Write(html)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || written != len(html) {
		_ = os.Remove(absolute)
		if writeErr != nil {
			return core.ProductRecommendationReportWriteResult{}, writeErr
		}
		if closeErr != nil {
			return core.ProductRecommendationReportWriteResult{}, closeErr
		}
		return core.ProductRecommendationReportWriteResult{}, errors.New("incomplete recommendation report write")
	}
	return core.ProductRecommendationReportWriteResult{
		SchemaVersion: core.ProductRecommendationReportSchemaVersion, OutputPath: absolute, BytesWritten: written,
		DiscoveredCount: len(report.Recommendation.Discovered), CandidateCount: len(report.Recommendation.Candidates), VisualEvidenceCount: len(report.VisualEvidence),
	}, nil
}
