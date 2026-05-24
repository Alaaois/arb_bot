package strategy

import (
	"math"
	"testing"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/funding"
)

func TestEstimateFundingProfitable(t *testing.T) {
	estimator := NewEstimator()
	now := time.Now().UTC()
	opportunity, ok, reason := estimator.EstimateFunding(EstimateInput{
		Symbol: "BTC/USDT",
		LongBook: exchange.OrderBookSnapshot{
			Exchange:  exchange.OKX,
			Symbol:    "BTC/USDT",
			Asks:      []exchange.Level{{Price: 100, Quantity: 2}},
			UpdatedAt: now,
		},
		ShortBook: exchange.OrderBookSnapshot{
			Exchange:  exchange.Bybit,
			Symbol:    "BTC/USDT",
			Bids:      []exchange.Level{{Price: 100.2, Quantity: 2}},
			UpdatedAt: now,
		},
		LongFunding: funding.Snapshot{
			Exchange:            exchange.OKX,
			Symbol:              "BTC/USDT",
			MarkPrice:           100,
			PredictedFundingPct: -0.08,
			NextFundingTime:     now.Add(2 * time.Hour),
		},
		ShortFunding: funding.Snapshot{
			Exchange:            exchange.Bybit,
			Symbol:              "BTC/USDT",
			MarkPrice:           100,
			PredictedFundingPct: 0.08,
			NextFundingTime:     now.Add(2 * time.Hour),
		},
		LongFeePct:          0.02,
		ShortFeePct:         0.02,
		MaxNotional:         100,
		MaxDepthLevels:      5,
		BasisRiskPct:        0.01,
		MinExpectedCarryUSD: 0.01,
		MinExpectedCarryPct: 0.001,
		DetectedAt:          now,
	})

	if !ok {
		t.Fatalf("expected opportunity, got reason %s", reason)
	}
	if math.Abs(opportunity.Quantity-0.998003992015968) > 1e-12 {
		t.Fatalf("unexpected quantity: %v", opportunity.Quantity)
	}
	if opportunity.ExpectedNetProfitUSD <= 0 {
		t.Fatalf("expected positive net pnl, got %v", opportunity.ExpectedNetProfitUSD)
	}
	if opportunity.LongExchange != exchange.OKX || opportunity.ShortExchange != exchange.Bybit {
		t.Fatalf("unexpected exchanges: %s / %s", opportunity.LongExchange, opportunity.ShortExchange)
	}
}

func TestEstimateFundingRejectsNegativeNetFunding(t *testing.T) {
	estimator := NewEstimator()
	now := time.Now().UTC()
	_, ok, _ := estimator.EstimateFunding(EstimateInput{
		Symbol: "BTC/USDT",
		LongBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks:     []exchange.Level{{Price: 100, Quantity: 2}},
		},
		ShortBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids:     []exchange.Level{{Price: 100.1, Quantity: 2}},
		},
		LongFunding: funding.Snapshot{
			Exchange:            exchange.Bybit,
			MarkPrice:           100,
			PredictedFundingPct: 0.03,
			NextFundingTime:     now.Add(time.Hour),
		},
		ShortFunding: funding.Snapshot{
			Exchange:            exchange.OKX,
			MarkPrice:           100,
			PredictedFundingPct: -0.01,
			NextFundingTime:     now.Add(time.Hour),
		},
		LongFeePct:          0.01,
		ShortFeePct:         0.01,
		MaxNotional:         100,
		MaxDepthLevels:      5,
		BasisRiskPct:        0.01,
		MinExpectedCarryUSD: 0.01,
		MinExpectedCarryPct: 0.001,
	})

	if ok {
		t.Fatal("expected rejection for negative carry")
	}
}

func TestEstimateFundingRespectsDepthLimit(t *testing.T) {
	estimator := NewEstimator()
	now := time.Now().UTC()
	opportunity, ok, _ := estimator.EstimateFunding(EstimateInput{
		Symbol: "ETH/USDT",
		LongBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Asks: []exchange.Level{
				{Price: 100, Quantity: 0.5},
				{Price: 101, Quantity: 1},
			},
		},
		ShortBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Bids: []exchange.Level{
				{Price: 102, Quantity: 0.25},
				{Price: 101.8, Quantity: 1},
			},
		},
		LongFunding: funding.Snapshot{
			Exchange:            exchange.OKX,
			MarkPrice:           100,
			PredictedFundingPct: -0.03,
			NextFundingTime:     now.Add(time.Hour),
		},
		ShortFunding: funding.Snapshot{
			Exchange:            exchange.Bybit,
			MarkPrice:           100,
			PredictedFundingPct: 0.03,
			NextFundingTime:     now.Add(time.Hour),
		},
		LongFeePct:          0.01,
		ShortFeePct:         0.01,
		MaxNotional:         500,
		MaxDepthLevels:      1,
		BasisRiskPct:        0.01,
		MinExpectedCarryUSD: 0.01,
		MinExpectedCarryPct: 0.001,
	})

	if !ok {
		t.Fatal("expected opportunity")
	}
	if math.Abs(opportunity.Quantity-0.25) > 1e-9 {
		t.Fatalf("quantity = %v, want 0.25", opportunity.Quantity)
	}
}
