package risk

import (
	"context"
	"time"

	"crypto-arbitrage-bot/internal/strategy"
)

type Config struct {
	MaxTradeUSD      float64
	MinProfitUSD     float64
	MinProfitPct     float64
	MinTimeToFunding time.Duration
}

type Decision struct {
	Approved  bool
	Reasons   []string
	CheckedAt time.Time
}

type Engine struct {
	cfg Config
}

func NewEngine(cfg Config) Engine {
	return Engine{cfg: cfg}
}

func (e Engine) Check(ctx context.Context, opportunity strategy.Opportunity) Decision {
	select {
	case <-ctx.Done():
		return Decision{Approved: false, Reasons: []string{"context canceled"}, CheckedAt: time.Now().UTC()}
	default:
	}

	var reasons []string
	notional := opportunity.Quantity * opportunity.LongEntryPrice
	if notional <= 0 {
		reasons = append(reasons, "notional must be positive")
	}
	if e.cfg.MaxTradeUSD > 0 && notional > e.cfg.MaxTradeUSD {
		reasons = append(reasons, "notional exceeds max trade limit")
	}
	if opportunity.ExpectedNetProfitUSD < e.cfg.MinProfitUSD {
		reasons = append(reasons, "net profit below minimum absolute threshold")
	}
	if opportunity.ExpectedNetProfitPct < e.cfg.MinProfitPct {
		reasons = append(reasons, "net profit below minimum percentage threshold")
	}
	if opportunity.LongExchange == opportunity.ShortExchange {
		reasons = append(reasons, "long and short exchange must differ")
	}
	if opportunity.Quantity <= 0 {
		reasons = append(reasons, "quantity must be positive")
	}
	if opportunity.TargetFundingTime.IsZero() {
		reasons = append(reasons, "target funding time is required")
	}
	if e.cfg.MinTimeToFunding > 0 && time.Until(opportunity.TargetFundingTime) < e.cfg.MinTimeToFunding {
		reasons = append(reasons, "funding window too close")
	}
	if opportunity.NetFundingRatePct <= 0 {
		reasons = append(reasons, "net funding rate must be positive")
	}

	return Decision{
		Approved:  len(reasons) == 0,
		Reasons:   reasons,
		CheckedAt: time.Now().UTC(),
	}
}
