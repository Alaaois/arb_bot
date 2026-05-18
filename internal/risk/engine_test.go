package risk

import (
	"context"
	"testing"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/strategy"
)

func TestRiskEngineApprovesValidOpportunity(t *testing.T) {
	engine := NewEngine(Config{
		MaxTradeUSD:  100,
		MinProfitUSD: 1,
		MinProfitPct: 0.2,
	})

	decision := engine.Check(context.Background(), strategy.Opportunity{
		Symbol:       "BTC/USDT",
		BuyExchange:  exchange.Bybit,
		SellExchange: exchange.OKX,
		Quantity:     1,
		NotionalUSD:  50,
		NetProfit:    2,
		NetProfitPct: 4,
	})

	if !decision.Approved {
		t.Fatalf("expected approved decision, got reasons: %v", decision.Reasons)
	}
}

func TestRiskEngineRejectsLowProfit(t *testing.T) {
	engine := NewEngine(Config{
		MaxTradeUSD:  100,
		MinProfitUSD: 1,
		MinProfitPct: 0.2,
	})

	decision := engine.Check(context.Background(), strategy.Opportunity{
		Symbol:       "BTC/USDT",
		BuyExchange:  exchange.Bybit,
		SellExchange: exchange.OKX,
		Quantity:     1,
		NotionalUSD:  50,
		NetProfit:    0.5,
		NetProfitPct: 1,
	})

	if decision.Approved {
		t.Fatal("expected rejected decision")
	}
}
