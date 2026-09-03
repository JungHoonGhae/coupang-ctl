package recap

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"strconv"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const (
	ShareImageWidth  = 1080
	ShareImageHeight = 1350
)

//go:embed share_card.html
var shareCardSource string

var shareCardTemplate = template.Must(template.New("share-card").Funcs(template.FuncMap{
	"axisLabel":       axisLabel,
	"axisReceipt":     axisReceipt,
	"characterColumn": characterColumn,
	"characterRow":    characterRow,
	"comma":           formatInteger,
	"monthRange":      monthRange,
	"pct":             formatPercent,
	"typeTitle":       profileTitle,
}).Parse(shareCardSource))

func RenderPublicShareCard(output io.Writer, summary core.ShoppingInsights) error {
	data := viewData{
		Summary:    summary,
		RosterData: template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(typeRoster)),
		FontData:   template.URL("data:font/ttf;base64," + base64.StdEncoding.EncodeToString(doodleFont)),
	}
	if err := shareCardTemplate.Execute(output, data); err != nil {
		return fmt.Errorf("render public recap share card: %w", err)
	}
	return nil
}

func PublicSharePreview(summary core.ShoppingInsights) core.RecapSharePreview {
	preview := core.RecapSharePreview{
		SchemaVersion: 1,
		Visibility:    "public_safe",
		Format:        "png",
		Width:         ShareImageWidth,
		Height:        ShareImageHeight,
		Ready:         summary.Profile.Ready,
		Fields: []core.RecapShareField{{
			ID: "analysis_period_month", Value: monthRange(summary.FirstOrderDate, summary.LastOrderDate),
			Provenance: "derived_from_observed_order_dates", Rule: "exact order dates are reduced to month precision",
		}},
		Excluded: []string{
			"product_names", "exact_order_dates", "spend_amounts", "order_ids",
			"brand_names", "category_labels_and_counts", "purchase_time_series",
			"delivery_year_series", "cancellation_and_return_rates", "badge_details",
			"payment_methods", "account_identifiers", "credentials_and_session_material",
		},
		ConfirmationFlag: "--confirm-public-safe-image",
		Limitations: []string{
			"the image is a playful summary of this local order history, not a psychological diagnosis or a population comparison",
			"the PNG is intended for public sharing, but the preview must be reviewed before writing it",
		},
	}
	if !summary.Profile.Ready {
		preview.Fields = append(preview.Fields, core.RecapShareField{
			ID: "profile_ready", Value: "false", Provenance: "derived",
			Rule: "one or more shopping-profile axes did not meet their documented minimum sample",
		})
		return preview
	}
	preview.Fields = append(preview.Fields,
		core.RecapShareField{ID: "shopping_profile_code", Value: summary.Profile.Code, Provenance: "derived", Rule: summary.Profile.RuleVersion},
		core.RecapShareField{ID: "shopping_profile_title", Value: profileTitle(summary.Profile.Code), Provenance: "presentation_copy", Rule: "deterministic title for the derived four-axis code"},
	)
	for _, axis := range summary.Profile.Axes {
		provenance := axis.Provenance
		if provenance == "" {
			provenance = "derived"
		}
		preview.Fields = append(preview.Fields, core.RecapShareField{
			ID: "axis." + axis.ID, Value: axis.SelectedCode + " " + axisLabel(axis.SelectedCode),
			Provenance: provenance, SampleSize: axis.SampleSize, Rule: axisReceipt(axis),
		})
	}
	preview.Fields = append(preview.Fields,
		core.RecapShareField{ID: "order_count", Value: strconv.Itoa(summary.OrderCount), Provenance: "derived", SampleSize: summary.OrderCount, Rule: "normalized orders in the selected analysis period"},
		core.RecapShareField{ID: "longest_active_month_streak", Value: strconv.Itoa(summary.LongestActiveMonthStreak), Provenance: "derived", Rule: "longest consecutive run of active product-purchase months"},
		core.RecapShareField{ID: "delivered_within_24_hours_rate", Value: formatPercent(summary.DeliveredWithin24HoursRate) + "%", Provenance: "derived", SampleSize: summary.Samples.DeliveryEvents, Rule: "delivered within 24 hours divided by shipments with usable order and delivery timestamps"},
	)
	return preview
}
