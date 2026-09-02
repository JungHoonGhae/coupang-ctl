package account

import (
	"context"
	"errors"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const defaultCashTransactionPages = 50

var ErrSourceUnavailable = errors.New("account benefits source unavailable")

type Source interface {
	Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error)
}

type Service struct {
	source Source
	now    func() time.Time
}

func New(source Source) *Service {
	return &Service{source: source, now: time.Now}
}

func (s *Service) Snapshot(ctx context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	if request.MaxCashTransactionPages == 0 {
		request.MaxCashTransactionPages = defaultCashTransactionPages
	}
	if err := request.Validate(); err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	if s.source == nil {
		return core.AccountBenefitsSnapshot{}, ErrSourceUnavailable
	}
	result, err := s.source.Snapshot(ctx, request)
	if err != nil {
		return core.AccountBenefitsSnapshot{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.AccountBenefitsSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = s.now().UTC()
	result.CardProgram = wowCardProgramTerms(s.now())
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	return result, nil
}

func wowCardProgramTerms(now time.Time) core.WowCardProgramTerms {
	status := "active"
	date := now.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly)
	if date < "2026-04-15" {
		status = "scheduled"
	} else if date > "2026-10-15" {
		status = "ended_or_unverified"
	}
	return core.WowCardProgramTerms{
		ProductName: "쿠팡 와우 카드", PublishedAnnualFeeKRW: 20_000,
		CoupangBaseRewardRate: 0.02, CoupangPromotionRewardRate: 0.02, CoupangMaximumRewardRate: 0.04,
		CoupangBaseMonthlyCapKRW: 20_000, CoupangPromotionMonthlyCapKRW: 20_000,
		PromotionFrom: "2026-04-15", PromotionTo: "2026-10-15", PromotionStatus: status,
		InterestFreeInstallmentRewardEligible: false,
		SourceURL:                             "https://m.coupang.com/vm/wowcard", LastVerified: "2026-09-02",
	}
}
