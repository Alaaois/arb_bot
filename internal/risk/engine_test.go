package risk

import (
	"context"
	"testing"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/strategy"
)

func TestRiskEngineApprovesValidOpportunity(t *testing.T) {
	engine := NewEngine(Config{
		MaxTradeUSD:      100,
		MinProfitUSD:     1,
		MinProfitPct:     0.2,
		MinTimeToFunding: time.Minute,
	})

	decision := engine.Check(context.Background(), strategy.Opportunity{
		Symbol:               "BTC/USDT",
		LongExchange:         exchange.Bybit,
		ShortExchange:        exchange.OKX,
		Quantity:             0.5,
		LongEntryPrice:       100,
		ExpectedNetProfitUSD: 2,
		ExpectedNetProfitPct: 4,
		NetFundingRatePct:    0.02,
		TargetFundingTime:    time.Now().Add(5 * time.Minute),
	})

	if !decision.Approved {
		t.Fatalf("expected approved decision, got reasons: %v", decision.Reasons)
	}
}

func TestRiskEngineRejectsLowProfit(t *testing.T) {
	engine := NewEngine(Config{
		MaxTradeUSD:      100,
		MinProfitUSD:     1,
		MinProfitPct:     0.2,
		MinTimeToFunding: time.Minute,
	})

	decision := engine.Check(context.Background(), strategy.Opportunity{
		Symbol:               "BTC/USDT",
		LongExchange:         exchange.Bybit,
		ShortExchange:        exchange.OKX,
		Quantity:             0.5,
		LongEntryPrice:       100,
		ExpectedNetProfitUSD: 0.5,
		ExpectedNetProfitPct: 1,
		NetFundingRatePct:    0.02,
		TargetFundingTime:    time.Now().Add(5 * time.Minute),
	})

	if decision.Approved {
		t.Fatal("expected rejected decision")
	}
}
