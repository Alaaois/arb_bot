package strategy

import (
	"math"
	"testing"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

func TestEstimateCrossExchangeProfitable(t *testing.T) {
	estimator := NewEstimator()
	opportunity, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange:  exchange.Bybit,
			Symbol:    "BTC/USDT",
			Asks:      []exchange.Level{{Price: 100, Quantity: 2}},
			UpdatedAt: time.Now(),
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange:  exchange.OKX,
			Symbol:    "BTC/USDT",
			Bids:      []exchange.Level{{Price: 101, Quantity: 2}},
			UpdatedAt: time.Now(),
		},
		BuyFeePct:    0.1,
		SellFeePct:   0.1,
		MaxNotional:  100,
		SafetyBuffer: 0.1,
		DetectedAt:   time.Unix(100, 0),
	})

	if !ok {
		t.Fatal("expected opportunity")
	}
	if opportunity.Quantity != 1 {
		t.Fatalf("quantity = %v, want 1", opportunity.Quantity)
	}
	if opportunity.NetProfit <= 0 {
		t.Fatalf("net profit = %v, want positive", opportunity.NetProfit)
	}
	if opportunity.BuyExchange != exchange.Bybit || opportunity.SellExchange != exchange.OKX {
		t.Fatalf("unexpected exchanges: %s -> %s", opportunity.BuyExchange, opportunity.SellExchange)
	}
}

func TestEstimateCrossExchangeRejectsNegativeSpread(t *testing.T) {
	estimator := NewEstimator()
	_, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks:     []exchange.Level{{Price: 101, Quantity: 1}},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids:     []exchange.Level{{Price: 100, Quantity: 1}},
		},
	})

	if ok {
		t.Fatal("expected no opportunity")
	}
}

func TestEstimateCrossExchangeMultiLevelDepth(t *testing.T) {
	estimator := NewEstimator()
	// Asks ascending: 100 (0.5), 101 (1.0)
	// Bids descending: 103 (1.0), 102 (0.3)
	opportunity, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Symbol:   "BTC/USDT",
			Asks: []exchange.Level{
				{Price: 100, Quantity: 0.5},
				{Price: 101, Quantity: 1.0},
			},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Symbol:   "BTC/USDT",
			Bids: []exchange.Level{
				{Price: 103, Quantity: 1.0},
				{Price: 102, Quantity: 0.3},
			},
		},
		BuyFeePct:      0.1,
		SellFeePct:     0.1,
		MaxNotional:    1000,
		SafetyBuffer:   0.1,
		MaxDepthLevels: 5,
	})

	if !ok {
		t.Fatal("expected opportunity")
	}

	// Walk:
	// L1: ask=100 qty=0.5, bid=103 qty=1.0 => match 0.5, ask exhausted, bidRem=0.5
	// L2: ask=101 qty=1.0, bid=103 rem=0.5 => match 0.5, bid exhausted, askRem=0.5
	// L3: ask=101 rem=0.5, bid=102 qty=0.3 => match 0.3, bid exhausted, askRem=0.2
	// Stop: no more bids
	// totalQty = 1.3
	expectedQty := 1.3
	if math.Abs(opportunity.Quantity-expectedQty) > 1e-9 {
		t.Fatalf("quantity = %v, want %v", opportunity.Quantity, expectedQty)
	}

	// VWAP buy: (100*0.5 + 101*0.5 + 101*0.3) / 1.3 = 130.8/1.3 = 100.615384...
	expectedBuyVWAP := 130.8 / 1.3
	if math.Abs(opportunity.BuyPrice-expectedBuyVWAP) > 1e-9 {
		t.Fatalf("buy vwap = %v, want %v", opportunity.BuyPrice, expectedBuyVWAP)
	}

	// VWAP sell: (103*0.5 + 103*0.5 + 102*0.3) / 1.3 = 133.6/1.3 = 102.769230...
	expectedSellVWAP := 133.6 / 1.3
	if math.Abs(opportunity.SellPrice-expectedSellVWAP) > 1e-9 {
		t.Fatalf("sell vwap = %v, want %v", opportunity.SellPrice, expectedSellVWAP)
	}
}

func TestEstimateCrossExchangeEarlyExitOnUnprofitableLevel(t *testing.T) {
	estimator := NewEstimator()
	// Level 1: ask=100, bid=101.05 => spread ~1.05%, after 0.2% cost => profitable
	// Level 2: ask=101, bid=100.5 => ask > bid, spread closed => break
	// Bids must be descending: 101.05 first, then 100.5
	opportunity, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks: []exchange.Level{
				{Price: 100, Quantity: 1},
				{Price: 101, Quantity: 1},
			},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids: []exchange.Level{
				{Price: 101.05, Quantity: 1},
				{Price: 100.5, Quantity: 1},
			},
		},
		BuyFeePct:      0.1,
		SellFeePct:     0.1,
		MaxNotional:    10000,
		SafetyBuffer:   0.1,
		MaxDepthLevels: 5,
	})

	if !ok {
		t.Fatal("expected opportunity")
	}

	// Should only consume level 1
	if opportunity.Quantity != 1 {
		t.Fatalf("quantity = %v, want 1 (early exit should stop at level 1)", opportunity.Quantity)
	}
}

func TestEstimateCrossExchangeFastFilter(t *testing.T) {
	estimator := NewEstimator()
	// Top spread is 0.1% (100 -> 100.1), filter requires 0.2%
	_, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks:     []exchange.Level{{Price: 100, Quantity: 1}},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids:     []exchange.Level{{Price: 100.1, Quantity: 1}},
		},
		MinTopSpreadPct: 0.2,
	})

	if ok {
		t.Fatal("expected no opportunity due to fast filter")
	}
}

func TestEstimateCrossExchangeMaxDepthLevels(t *testing.T) {
	estimator := NewEstimator()
	// 3 levels all profitable, but max depth = 2
	// Bids descending: 105, 104, 103
	// Asks ascending: 100, 101, 102
	opportunity, ok, _ := estimator.EstimateCrossExchange(EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks: []exchange.Level{
				{Price: 100, Quantity: 1},
				{Price: 101, Quantity: 1},
				{Price: 102, Quantity: 1},
			},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids: []exchange.Level{
				{Price: 105, Quantity: 1},
				{Price: 104, Quantity: 1},
				{Price: 103, Quantity: 1},
			},
		},
		BuyFeePct:      0.01,
		SellFeePct:     0.01,
		MaxNotional:    10000,
		SafetyBuffer:   0.01,
		MaxDepthLevels: 2,
	})

	if !ok {
		t.Fatal("expected opportunity")
	}
	if opportunity.Quantity != 2 {
		t.Fatalf("quantity = %v, want 2 (max depth limit)", opportunity.Quantity)
	}
}

func BenchmarkEstimateCrossExchangeLevel0(b *testing.B) {
	estimator := NewEstimator()
	input := EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks:     []exchange.Level{{Price: 100, Quantity: 1}},
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids:     []exchange.Level{{Price: 101, Quantity: 1}},
		},
		BuyFeePct:    0.1,
		SellFeePct:   0.1,
		MaxNotional:  1000,
		SafetyBuffer: 0.1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = estimator.EstimateCrossExchange(input)
	}
}

func BenchmarkEstimateCrossExchangeDepth5(b *testing.B) {
	estimator := NewEstimator()
	buyLevels := make([]exchange.Level, 5)
	sellLevels := make([]exchange.Level, 5)
	for i := 0; i < 5; i++ {
		buyLevels[i] = exchange.Level{Price: 100 + float64(i), Quantity: 1}
		sellLevels[i] = exchange.Level{Price: 106 - float64(i), Quantity: 1}
	}

	input := EstimateInput{
		Symbol: "BTC/USDT",
		BuyBook: exchange.OrderBookSnapshot{
			Exchange: exchange.Bybit,
			Asks:     buyLevels,
		},
		SellBook: exchange.OrderBookSnapshot{
			Exchange: exchange.OKX,
			Bids:     sellLevels,
		},
		BuyFeePct:      0.1,
		SellFeePct:     0.1,
		MaxNotional:    10000,
		SafetyBuffer:   0.1,
		MaxDepthLevels: 5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = estimator.EstimateCrossExchange(input)
	}
}
