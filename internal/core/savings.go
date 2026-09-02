package core

import (
	"errors"
	"strings"
	"time"
)

type SavingsMode string

const (
	SavingsModeAuto SavingsMode = "auto"
	SavingsModeOff  SavingsMode = "off"
)

const (
	SavingsProgramCoupangWowCard = "coupang_wow_card"
	SavingsProgramKBPayEvent     = "kbpay_coupang_event"
)

type SavingsPlanRequest struct {
	PurchaseAmountKRW int64       `json:"purchase_amount_krw" jsonschema:"Observed current product amount in KRW"`
	Mode              SavingsMode `json:"mode,omitempty" jsonschema:"auto evaluates legitimate savings routes; off returns direct pricing only"`
	EnabledPrograms   []string    `json:"enabled_programs,omitempty" jsonschema:"Programs the user says they can use,currently coupang_wow_card or kbpay_coupang_event"`
	AllowAffiliate    bool        `json:"allow_affiliate,omitempty" jsonschema:"Explicit opt-in for commission-attributing links; false by default"`
}

func (r SavingsPlanRequest) Validate() error {
	if r.PurchaseAmountKRW <= 0 || r.PurchaseAmountKRW > 1_000_000_000 {
		return errors.New("purchase_amount_krw must be between 1 and 1000000000")
	}
	if r.Mode != "" && r.Mode != SavingsModeAuto && r.Mode != SavingsModeOff {
		return errors.New("unsupported savings mode")
	}
	if len(r.EnabledPrograms) > 10 {
		return errors.New("too many savings programs")
	}
	for _, program := range r.EnabledPrograms {
		switch strings.TrimSpace(program) {
		case SavingsProgramCoupangWowCard, SavingsProgramKBPayEvent:
		default:
			return errors.New("unsupported savings program")
		}
	}
	return nil
}

type SavingsRoute struct {
	ID                      string   `json:"id"`
	Kind                    string   `json:"kind"`
	Status                  string   `json:"status"`
	Eligibility             string   `json:"eligibility"`
	Guaranteed              bool     `json:"guaranteed"`
	Automatic               bool     `json:"automatic"`
	RequiresHumanHandoff    bool     `json:"requires_human_handoff"`
	AffiliateAttribution    bool     `json:"affiliate_attribution"`
	EstimatedRewardKRW      int64    `json:"estimated_reward_krw,omitempty"`
	RewardRangeKRW          []int64  `json:"reward_range_krw,omitempty"`
	BestCaseEffectiveAmount int64    `json:"best_case_effective_amount_krw"`
	WorstCaseEffectiveAmount int64   `json:"worst_case_effective_amount_krw"`
	RewardUnits             int      `json:"reward_units,omitempty"`
	RewardUnitName          string   `json:"reward_unit_name,omitempty"`
	Conditions              []string `json:"conditions"`
	EntryURL                string   `json:"entry_url,omitempty"`
	SourceURL               string   `json:"source_url"`
	LastVerified            string   `json:"last_verified"`
}

type SavingsPlan struct {
	SchemaVersion      int            `json:"schema_version"`
	Mode               SavingsMode    `json:"mode"`
	BaselineAmountKRW  int64          `json:"baseline_amount_krw"`
	Currency           string         `json:"currency"`
	GeneratedAt        time.Time      `json:"generated_at"`
	RecommendedRouteID string         `json:"recommended_route_id"`
	Recommendation     string         `json:"recommendation"`
	Routes             []SavingsRoute `json:"routes"`
	Disclosure         []string       `json:"disclosure"`
}
