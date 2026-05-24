package app

import (
	"context"
	"log/slog"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/execution"
	"crypto-arbitrage-bot/internal/funding"
	"crypto-arbitrage-bot/internal/marketdata"
	"crypto-arbitrage-bot/internal/risk"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/storage"
	"crypto-arbitrage-bot/internal/strategy"
)

type ScannerConfig struct {
	Config        Config
	Books         *marketdata.Store
	Funding       *funding.Store
	Estimator     strategy.Estimator
	Risk          risk.Engine
	Execution     execution.Engine
	Opportunities *state.OpportunityLog
	Positions     *state.PositionBook
	Storage       *storage.Store
	Logger        *slog.Logger
}

type Scanner struct {
	cfg           Config
	books         *marketdata.Store
	funding       *funding.Store
	estimator     strategy.Estimator
	risk          risk.Engine
	execution     execution.Engine
	opportunities *state.OpportunityLog
	positions     *state.PositionBook
	storage       *storage.Store
	logger        *slog.Logger
}

func NewScanner(cfg ScannerConfig) *Scanner {
	return &Scanner{
		cfg:           cfg.Config,
		books:         cfg.Books,
		funding:       cfg.Funding,
		estimator:     cfg.Estimator,
		risk:          cfg.Risk,
		execution:     cfg.Execution,
		opportunities: cfg.Opportunities,
		positions:     cfg.Positions,
		storage:       cfg.Storage,
		logger:        cfg.Logger,
	}
}

func (s *Scanner) OnOrderBookUpdate(ctx context.Context, update exchange.OrderBookSnapshot) {
	books := s.books.Snapshot(update.Symbol)
	fundingSnapshots := s.funding.Snapshot(update.Symbol)
	now := update.UpdatedAt

	best, ok := s.bestOpportunity(update.Symbol, books, fundingSnapshots, now)
	if ok {
		if !s.positions.HasOpen(best.Symbol, best.LongExchange, best.ShortExchange) {
			decision := s.risk.Check(ctx, best)
			if decision.Approved {
				result, err := s.execution.Execute(ctx, best)
				if err != nil {
					s.logger.Warn("execution failed", "opportunity", best.ID, "error", err)
				} else {
					position := state.Position{
						ID:                best.ID,
						Symbol:            best.Symbol,
						LongLeg:           state.Leg{Exchange: best.LongExchange, EntryPrice: best.LongEntryPrice},
						ShortLeg:          state.Leg{Exchange: best.ShortExchange, EntryPrice: best.ShortEntryPrice},
						Quantity:          best.Quantity,
						Status:            state.PositionOpen,
						OpenedAt:          result.StartedAt,
						TargetFundingTime: best.TargetFundingTime,
						ExpectedCarryUSD:  best.ExpectedCarryUSD,
						LastUpdatedAt:     result.FinishedAt,
					}
					s.positions.Add(position)
					record := state.OpportunityRecord{
						Opportunity: best,
						Decision:    decision,
						Execution:   result,
					}
					s.opportunities.Add(record)
					s.storage.Enqueue(storage.Event{Type: storage.EventOpportunity, Opportunity: record})
					s.storage.Enqueue(storage.Event{Type: storage.EventPosition, Position: position})
					s.logger.Info("funding opportunity opened",
						"id", best.ID,
						"symbol", best.Symbol,
						"long", best.LongExchange,
						"short", best.ShortExchange,
						"expected_net_pnl_usd", best.ExpectedNetProfitUSD,
						"net_funding_rate_pct", best.NetFundingRatePct,
						"mode", result.Mode,
					)
				}
			}
		}
	}

	s.refreshPositions(update.Symbol, books, fundingSnapshots, now)
}

func (s *Scanner) bestOpportunity(
	symbol string,
	books map[exchange.Name]exchange.OrderBookSnapshot,
	fundingSnapshots map[exchange.Name]funding.Snapshot,
	now time.Time,
) (strategy.Opportunity, bool) {
	var best strategy.Opportunity
	var found bool
	for longExchange, longBook := range books {
		if !s.books.IsFresh(longBook, s.cfg.Risk.MaxOrderBookAge, now) {
			continue
		}
		longFunding, ok := fundingSnapshots[longExchange]
		if !ok {
			continue
		}
		for shortExchange, shortBook := range books {
			if longExchange == shortExchange {
				continue
			}
			if !s.books.IsFresh(shortBook, s.cfg.Risk.MaxOrderBookAge, now) {
				continue
			}
			shortFunding, ok := fundingSnapshots[shortExchange]
			if !ok {
				continue
			}

			opportunity, ok, reason := s.estimator.EstimateFunding(strategy.EstimateInput{
				Symbol:              symbol,
				LongBook:            longBook,
				ShortBook:           shortBook,
				LongFunding:         longFunding,
				ShortFunding:        shortFunding,
				LongFeePct:          s.takerFeePct(longExchange),
				ShortFeePct:         s.takerFeePct(shortExchange),
				MaxNotional:         s.cfg.Risk.MaxTradeUSD,
				MaxDepthLevels:      s.cfg.Strategy.MaxDepthLevels,
				BasisRiskPct:        s.cfg.Strategy.BasisRiskPct,
				MinExpectedCarryUSD: s.cfg.Strategy.MinExpectedCarryUSD,
				MinExpectedCarryPct: s.cfg.Strategy.MinExpectedCarryPct,
				DetectedAt:          now,
			})
			if !ok {
				s.logger.Debug("funding estimator rejected",
					"symbol", symbol,
					"long", longExchange,
					"short", shortExchange,
					"reason", reason,
				)
				continue
			}
			if !found || opportunity.ExpectedNetProfitUSD > best.ExpectedNetProfitUSD {
				best = opportunity
				found = true
			}
		}
	}
	return best, found
}

func (s *Scanner) refreshPositions(
	symbol string,
	books map[exchange.Name]exchange.OrderBookSnapshot,
	fundingSnapshots map[exchange.Name]funding.Snapshot,
	now time.Time,
) {
	for _, position := range s.positions.OpenPositionsBySymbol(symbol) {
		longBook, okLong := books[position.LongLeg.Exchange]
		shortBook, okShort := books[position.ShortLeg.Exchange]
		longFunding, okLongFunding := fundingSnapshots[position.LongLeg.Exchange]
		shortFunding, okShortFunding := fundingSnapshots[position.ShortLeg.Exchange]
		if !okLong || !okShort || !okLongFunding || !okShortFunding {
			continue
		}
		if len(longBook.Bids) == 0 || len(shortBook.Asks) == 0 {
			continue
		}

		exitLong := longBook.Bids[0].Price
		exitShort := shortBook.Asks[0].Price
		unrealized := (exitLong-position.LongLeg.EntryPrice)*position.Quantity +
			(position.ShortLeg.EntryPrice-exitShort)*position.Quantity
		position.UnrealizedPnLUSD = unrealized
		position.LastUpdatedAt = now

		currentNetFundingPct := shortFunding.PredictedFundingPct - longFunding.PredictedFundingPct
		if currentNetFundingPct <= 0 {
			position.Status = state.PositionClosed
			position.ClosedAt = now
			position.LongLeg.ExitPrice = exitLong
			position.ShortLeg.ExitPrice = exitShort
			position.RealizedPnLUSD = unrealized
			position.CloseReason = "carry_deteriorated"
			s.positions.Close(position)
			s.storage.Enqueue(storage.Event{Type: storage.EventPosition, Position: position})
			continue
		}

		if now.Before(position.TargetFundingTime) && now.Sub(position.OpenedAt) < s.cfg.Strategy.MaxHoldTime {
			s.positions.Update(position)
			continue
		}

		position.Status = state.PositionClosed
		position.ClosedAt = now
		position.LongLeg.ExitPrice = exitLong
		position.ShortLeg.ExitPrice = exitShort
		position.AccruedFundingUSD = position.ExpectedCarryUSD
		position.RealizedPnLUSD = unrealized + position.AccruedFundingUSD
		if now.Sub(position.OpenedAt) >= s.cfg.Strategy.MaxHoldTime {
			position.CloseReason = "max_hold_time"
		} else {
			position.CloseReason = "funding_captured"
		}
		s.positions.Close(position)
		s.storage.Enqueue(storage.Event{Type: storage.EventPosition, Position: position})
		s.logger.Info("paper position closed",
			"id", position.ID,
			"symbol", position.Symbol,
			"realized_pnl_usd", position.RealizedPnLUSD,
			"accrued_funding_usd", position.AccruedFundingUSD,
			"reason", position.CloseReason,
		)
	}
}

func (s *Scanner) takerFeePct(exchange exchange.Name) float64 {
	if fee, ok := s.cfg.Fees[string(exchange)]; ok {
		return fee.TakerPct
	}
	return 0.10
}
