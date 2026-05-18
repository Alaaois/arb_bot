package app

import (
	"context"
	"log/slog"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/execution"
	"crypto-arbitrage-bot/internal/marketdata"
	"crypto-arbitrage-bot/internal/risk"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/storage"
	"crypto-arbitrage-bot/internal/strategy"
)

type ScannerConfig struct {
	Config        Config
	Books         *marketdata.Store
	Estimator     strategy.Estimator
	Risk          risk.Engine
	Execution     execution.Engine
	Opportunities *state.OpportunityLog
	Storage       *storage.Store
	Logger        *slog.Logger
}

type Scanner struct {
	cfg           Config
	books         *marketdata.Store
	estimator     strategy.Estimator
	risk          risk.Engine
	execution     execution.Engine
	opportunities *state.OpportunityLog
	storage       *storage.Store
	logger        *slog.Logger
}

func NewScanner(cfg ScannerConfig) *Scanner {
	return &Scanner{
		cfg:           cfg.Config,
		books:         cfg.Books,
		estimator:     cfg.Estimator,
		risk:          cfg.Risk,
		execution:     cfg.Execution,
		opportunities: cfg.Opportunities,
		storage:       cfg.Storage,
		logger:        cfg.Logger,
	}
}

func (s *Scanner) OnOrderBookUpdate(ctx context.Context, update exchange.OrderBookSnapshot) {
	books := s.books.Snapshot(update.Symbol)
	now := update.UpdatedAt
	for buyExchange, buyBook := range books {
		if !s.books.IsFresh(buyBook, s.cfg.Risk.MaxOrderBookAge, now) {
			continue
		}
		for sellExchange, sellBook := range books {
			if buyExchange == sellExchange {
				continue
			}
			if !s.books.IsFresh(sellBook, s.cfg.Risk.MaxOrderBookAge, now) {
				continue
			}

			opportunity, ok, reason := s.estimator.EstimateCrossExchange(strategy.EstimateInput{
				Symbol:          update.Symbol,
				BuyBook:         buyBook,
				SellBook:        sellBook,
				BuyFeePct:       s.takerFeePct(buyExchange),
				SellFeePct:      s.takerFeePct(sellExchange),
				MaxNotional:     s.cfg.Risk.MaxTradeUSD,
				SafetyBuffer:    s.cfg.Risk.MaxSlippagePct,
				MaxDepthLevels:  s.cfg.Strategy.MaxDepthLevels,
				MinTopSpreadPct: s.cfg.Strategy.MinTopSpreadPct,
				DetectedAt:      now,
			})
			if !ok {
				s.logger.Debug("estimator rejected",
					"symbol", update.Symbol,
					"buy", buyExchange,
					"sell", sellExchange,
					"reason", reason,
					"buy_ask", buyBook.Asks[0].Price,
					"sell_bid", sellBook.Bids[0].Price,
				)
				continue
			}
			s.storage.Enqueue(storage.Event{
				Type:   storage.EventSpread,
				Spread: storage.SpreadFromOpportunity(opportunity),
			})

			decision := s.risk.Check(ctx, opportunity)
			if !decision.Approved {
				s.logger.Debug("risk rejected",
					"symbol", update.Symbol,
					"buy", buyExchange,
					"sell", sellExchange,
					"reasons", decision.Reasons,
					"net_profit", opportunity.NetProfit,
					"net_profit_pct", opportunity.NetProfitPct,
				)
				continue
			}

			result, err := s.execution.Execute(ctx, opportunity)
			if err != nil {
				s.logger.Warn("execution failed", "opportunity", opportunity.ID, "error", err)
				continue
			}

			record := state.OpportunityRecord{
				Opportunity: opportunity,
				Decision:    decision,
				Execution:   result,
			}
			s.opportunities.Add(record)
			s.storage.Enqueue(storage.Event{Type: storage.EventOpportunity, Opportunity: record})
			s.logger.Info("opportunity executed",
				"id", opportunity.ID,
				"symbol", opportunity.Symbol,
				"buy", opportunity.BuyExchange,
				"sell", opportunity.SellExchange,
				"net_profit", opportunity.NetProfit,
				"net_profit_pct", opportunity.NetProfitPct,
				"mode", result.Mode,
			)
		}
	}
}

func (s *Scanner) takerFeePct(exchange exchange.Name) float64 {
	if fee, ok := s.cfg.Fees[string(exchange)]; ok {
		return fee.TakerPct
	}
	return 0.10
}
