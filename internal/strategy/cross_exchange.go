package strategy

import (
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

type EstimateInput struct {
	Symbol          string
	BuyBook         exchange.OrderBookSnapshot
	SellBook        exchange.OrderBookSnapshot
	BuyFeePct       float64
	SellFeePct      float64
	MaxNotional     float64
	SafetyBuffer    float64
	MaxDepthLevels  int
	MinTopSpreadPct float64
	DetectedAt      time.Time
}

type Opportunity struct {
	ID           string
	Symbol       string
	BuyExchange  exchange.Name
	SellExchange exchange.Name
	BuyPrice     float64
	SellPrice    float64
	Quantity     float64
	NotionalUSD  float64
	GrossProfit  float64
	NetProfit    float64
	NetProfitPct float64
	DetectedAt   time.Time
}

type Estimator struct{}

func NewEstimator() Estimator {
	return Estimator{}
}

func (Estimator) EstimateCrossExchange(input EstimateInput) (Opportunity, bool, string) {
	if len(input.BuyBook.Asks) == 0 || len(input.SellBook.Bids) == 0 {
		return Opportunity{}, false, "empty_book"
	}

	ask0 := input.BuyBook.Asks[0].Price
	bid0 := input.SellBook.Bids[0].Price

	if ask0 <= 0 || bid0 <= 0 || input.BuyBook.Asks[0].Quantity <= 0 || input.SellBook.Bids[0].Quantity <= 0 {
		return Opportunity{}, false, "invalid_price_or_quantity"
	}

	if ask0 >= bid0 {
		return Opportunity{}, false, "no_spread"
	}

	// Fast top-of-book rejection
	if input.MinTopSpreadPct > 0 {
		topSpreadPct := (bid0 - ask0) / ask0 * 100
		if topSpreadPct < input.MinTopSpreadPct {
			return Opportunity{}, false, "fast_filter_rejected"
		}
	}

	maxDepth := input.MaxDepthLevels
	if maxDepth <= 0 {
		maxDepth = 5
	}

	buyLevels := input.BuyBook.Asks
	sellLevels := input.SellBook.Bids

	var totalQty, buyCost, sellRevenue float64
	var askRem, bidRem float64
	i, j := 0, 0

	for i < len(buyLevels) && j < len(sellLevels) {
		if i >= maxDepth || j >= maxDepth {
			break
		}

		ask := buyLevels[i]
		bid := sellLevels[j]

		if ask.Price >= bid.Price {
			break
		}

		if askRem == 0 {
			askRem = ask.Quantity
		}
		if bidRem == 0 {
			bidRem = bid.Quantity
		}

		qty := minFloat(askRem, bidRem)
		if input.MaxNotional > 0 {
			remainingNotional := input.MaxNotional - buyCost
			if remainingNotional <= 0 {
				break
			}
			maxQtyByNotional := remainingNotional / ask.Price
			if qty > maxQtyByNotional {
				qty = maxQtyByNotional
			}
		}

		if qty <= 0 {
			break
		}

		// Early exit: if this level is unprofitable after fees/safety, deeper levels will be worse
		levelGross := (bid.Price - ask.Price) * qty
		levelFee := (ask.Price*qty*input.BuyFeePct + bid.Price*qty*input.SellFeePct) / 100
		levelSafety := ask.Price * qty * input.SafetyBuffer / 100
		levelNet := levelGross - levelFee - levelSafety
		if levelNet < 0 {
			break
		}

		totalQty += qty
		buyCost += ask.Price * qty
		sellRevenue += bid.Price * qty

		askRem -= qty
		bidRem -= qty

		if askRem <= 0 {
			i++
			askRem = 0
		}
		if bidRem <= 0 {
			j++
			bidRem = 0
		}
	}

	if totalQty <= 0 || buyCost <= 0 {
		return Opportunity{}, false, "zero_quantity"
	}

	buyFee := buyCost * input.BuyFeePct / 100
	sellFee := sellRevenue * input.SellFeePct / 100
	safety := buyCost * input.SafetyBuffer / 100
	grossProfit := sellRevenue - buyCost
	netProfit := grossProfit - buyFee - sellFee - safety
	netProfitPct := netProfit / buyCost * 100

	detectedAt := input.DetectedAt
	if detectedAt.IsZero() {
		detectedAt = time.Now().UTC()
	}

	return Opportunity{
		ID:           NewOpportunityID(input.Symbol, input.BuyBook.Exchange, input.SellBook.Exchange, detectedAt),
		Symbol:       input.Symbol,
		BuyExchange:  input.BuyBook.Exchange,
		SellExchange: input.SellBook.Exchange,
		BuyPrice:     buyCost / totalQty,
		SellPrice:    sellRevenue / totalQty,
		Quantity:     totalQty,
		NotionalUSD:  buyCost,
		GrossProfit:  grossProfit,
		NetProfit:    netProfit,
		NetProfitPct: netProfitPct,
		DetectedAt:   detectedAt,
	}, true, ""
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
