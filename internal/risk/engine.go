package risk

import (
	"context"
	"time"

	"crypto-arbitrage-bot/internal/strategy"
)

type Config struct {
	MaxTradeUSD  float64
	MinProfitUSD float64
	MinProfitPct float64
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
	if opportunity.NotionalUSD <= 0 {
		reasons = append(reasons, "notional must be positive")
	}
	if e.cfg.MaxTradeUSD > 0 && opportunity.NotionalUSD > e.cfg.MaxTradeUSD {
		reasons = append(reasons, "notional exceeds max trade limit")
	}
	if opportunity.NetProfit < e.cfg.MinProfitUSD {
		reasons = append(reasons, "net profit below minimum absolute threshold")
	}
	if opportunity.NetProfitPct < e.cfg.MinProfitPct {
		reasons = append(reasons, "net profit below minimum percentage threshold")
	}
	if opportunity.BuyExchange == opportunity.SellExchange {
		reasons = append(reasons, "buy and sell exchange must differ")
	}
	if opportunity.Quantity <= 0 {
		reasons = append(reasons, "quantity must be positive")
	}

	return Decision{
		Approved:  len(reasons) == 0,
		Reasons:   reasons,
		CheckedAt: time.Now().UTC(),
	}
}
