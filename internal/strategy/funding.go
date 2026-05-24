package strategy

import (
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/funding"
)

type EstimateInput struct {
	Symbol              string
	LongBook            exchange.OrderBookSnapshot
	ShortBook           exchange.OrderBookSnapshot
	LongFunding         funding.Snapshot
	ShortFunding        funding.Snapshot
	LongFeePct          float64
	ShortFeePct         float64
	MaxNotional         float64
	MaxDepthLevels      int
	BasisRiskPct        float64
	MinExpectedCarryUSD float64
	MinExpectedCarryPct float64
	DetectedAt          time.Time
}

type Opportunity struct {
	ID                   string        `json:"id"`
	Symbol               string        `json:"symbol"`
	LongExchange         exchange.Name `json:"long_exchange"`
	ShortExchange        exchange.Name `json:"short_exchange"`
	Quantity             float64       `json:"quantity"`
	LongEntryPrice       float64       `json:"long_entry_price"`
	ShortEntryPrice      float64       `json:"short_entry_price"`
	ExpectedCarryUSD     float64       `json:"expected_carry_usd"`
	EntryCostUSD         float64       `json:"entry_cost_usd"`
	ExitCostUSD          float64       `json:"exit_cost_usd"`
	BasisRiskBufferUSD   float64       `json:"basis_risk_buffer_usd"`
	ExpectedNetProfitUSD float64       `json:"expected_net_profit_usd"`
	ExpectedNetProfitPct float64       `json:"expected_net_profit_pct"`
	NetFundingRatePct    float64       `json:"net_funding_rate_pct"`
	DetectedAt           time.Time     `json:"detected_at"`
	ValidUntil           time.Time     `json:"valid_until"`
	TargetFundingTime    time.Time     `json:"target_funding_time"`
}

type Estimator struct{}

func NewEstimator() Estimator {
	return Estimator{}
}

func (Estimator) EstimateFunding(input EstimateInput) (Opportunity, bool, string) {
	if len(input.LongBook.Asks) == 0 || len(input.ShortBook.Bids) == 0 {
		return Opportunity{}, false, "empty_book"
	}
	if input.LongFunding.NextFundingTime.IsZero() || input.ShortFunding.NextFundingTime.IsZero() {
		return Opportunity{}, false, "missing_funding_window"
	}

	ask0 := input.LongBook.Asks[0]
	bid0 := input.ShortBook.Bids[0]
	if ask0.Price <= 0 || bid0.Price <= 0 || ask0.Quantity <= 0 || bid0.Quantity <= 0 {
		return Opportunity{}, false, "invalid_price_or_quantity"
	}

	maxDepth := input.MaxDepthLevels
	if maxDepth <= 0 {
		maxDepth = 5
	}

	maxQtyByNotional := input.MaxNotional / maxFloat(ask0.Price, bid0.Price)
	targetQty := minPositive(
		sumQuantity(input.LongBook.Asks, maxDepth),
		sumQuantity(input.ShortBook.Bids, maxDepth),
		maxQtyByNotional,
	)
	if targetQty <= 0 {
		return Opportunity{}, false, "zero_quantity"
	}

	longEntryPrice, ok := vwapForQuantity(input.LongBook.Asks, targetQty, maxDepth)
	if !ok {
		return Opportunity{}, false, "insufficient_long_depth"
	}
	shortEntryPrice, ok := vwapForQuantity(input.ShortBook.Bids, targetQty, maxDepth)
	if !ok {
		return Opportunity{}, false, "insufficient_short_depth"
	}

	notional := targetQty * maxFloat(longEntryPrice, shortEntryPrice)
	netFundingRatePct := input.ShortFunding.PredictedFundingPct - input.LongFunding.PredictedFundingPct
	expectedCarryUSD := targetQty * input.LongFunding.MarkPrice * netFundingRatePct / 100
	entryCostUSD := targetQty*longEntryPrice*input.LongFeePct/100 + targetQty*shortEntryPrice*input.ShortFeePct/100
	exitCostUSD := entryCostUSD
	basisRiskBufferUSD := notional * input.BasisRiskPct / 100
	expectedNetProfitUSD := expectedCarryUSD - entryCostUSD - exitCostUSD - basisRiskBufferUSD
	expectedNetProfitPct := 0.0
	if notional > 0 {
		expectedNetProfitPct = expectedNetProfitUSD / notional * 100
	}

	if expectedCarryUSD < input.MinExpectedCarryUSD {
		return Opportunity{}, false, "carry_below_threshold"
	}
	if expectedNetProfitUSD <= 0 {
		return Opportunity{}, false, "net_profit_not_positive"
	}
	if expectedNetProfitPct < input.MinExpectedCarryPct {
		return Opportunity{}, false, "net_profit_pct_below_threshold"
	}

	detectedAt := input.DetectedAt
	if detectedAt.IsZero() {
		detectedAt = time.Now().UTC()
	}
	targetFundingTime := minTime(input.LongFunding.NextFundingTime, input.ShortFunding.NextFundingTime)
	return Opportunity{
		ID:                   NewOpportunityID(input.Symbol, input.LongBook.Exchange, input.ShortBook.Exchange, detectedAt),
		Symbol:               input.Symbol,
		LongExchange:         input.LongBook.Exchange,
		ShortExchange:        input.ShortBook.Exchange,
		Quantity:             targetQty,
		LongEntryPrice:       longEntryPrice,
		ShortEntryPrice:      shortEntryPrice,
		ExpectedCarryUSD:     expectedCarryUSD,
		EntryCostUSD:         entryCostUSD,
		ExitCostUSD:          exitCostUSD,
		BasisRiskBufferUSD:   basisRiskBufferUSD,
		ExpectedNetProfitUSD: expectedNetProfitUSD,
		ExpectedNetProfitPct: expectedNetProfitPct,
		NetFundingRatePct:    netFundingRatePct,
		DetectedAt:           detectedAt,
		ValidUntil:           targetFundingTime,
		TargetFundingTime:    targetFundingTime,
	}, true, ""
}

func vwapForQuantity(levels []exchange.Level, targetQty float64, maxDepth int) (float64, bool) {
	if targetQty <= 0 {
		return 0, false
	}
	var cost float64
	var filled float64
	for i, level := range levels {
		if i >= maxDepth {
			break
		}
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		qty := level.Quantity
		if remaining := targetQty - filled; qty > remaining {
			qty = remaining
		}
		cost += qty * level.Price
		filled += qty
		if filled >= targetQty {
			return cost / filled, true
		}
	}
	return 0, false
}

func sumQuantity(levels []exchange.Level, maxDepth int) float64 {
	var total float64
	for i, level := range levels {
		if i >= maxDepth {
			break
		}
		if level.Quantity > 0 {
			total += level.Quantity
		}
	}
	return total
}

func minPositive(values ...float64) float64 {
	var out float64
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if out == 0 || value < out {
			out = value
		}
	}
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}
