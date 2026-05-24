package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"crypto-arbitrage-bot/internal/api"
	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/exchange/bybit"
	"crypto-arbitrage-bot/internal/exchange/htx"
	"crypto-arbitrage-bot/internal/exchange/kucoin"
	"crypto-arbitrage-bot/internal/exchange/okx"
	"crypto-arbitrage-bot/internal/execution"
	"crypto-arbitrage-bot/internal/funding"
	"crypto-arbitrage-bot/internal/marketdata"
	"crypto-arbitrage-bot/internal/risk"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/storage"
	"crypto-arbitrage-bot/internal/strategy"
)

type Runtime struct {
	cfg           Config
	logger        *slog.Logger
	connectors    map[exchange.Name]exchange.Connector
	books         *marketdata.Store
	funding       *funding.Store
	opportunities *state.OpportunityLog
	positions     *state.PositionBook
	storage       *storage.Store
	scanner       *Scanner
	apiServer     *http.Server
}

func NewRuntime(cfg Config, logger *slog.Logger) (*Runtime, error) {
	connectors := buildConnectors(cfg)
	books := marketdata.NewStore()
	fundingStore := funding.NewStore(funding.Config{
		DefaultRatePct:  cfg.Strategy.DefaultFundingRatePct,
		FundingInterval: cfg.Strategy.FundingInterval,
	})
	opportunities := state.NewOpportunityLog(100)
	positions := state.NewPositionBook(100)
	storageStore := storage.NewStore(storage.Config{
		Enabled:          cfg.Storage.RedisEnabled,
		Addr:             cfg.Storage.RedisAddr,
		Password:         cfg.Storage.RedisPassword,
		DB:               cfg.Storage.RedisDB,
		HistoryLimit:     cfg.Storage.RedisHistoryLimit,
		OperationTimeout: cfg.Storage.RedisOperationTimeout,
	}, logger)
	executionEngine := execution.NewEngine(execution.Config{
		Mode:      cfg.Trading.Mode,
		Timeout:   cfg.Execution.OrderTimeout,
		OrderType: cfg.Execution.OrderType,
	}, connectors, logger)
	scanner := NewScanner(ScannerConfig{
		Config:    cfg,
		Books:     books,
		Estimator: strategy.NewEstimator(),
		Risk: risk.NewEngine(risk.Config{
			MaxTradeUSD:      cfg.Risk.MaxTradeUSD,
			MinProfitUSD:     cfg.Risk.MinProfitUSD,
			MinProfitPct:     cfg.Risk.MinProfitPct,
			MinTimeToFunding: cfg.Strategy.MinTimeToFunding,
		}),
		Funding:       fundingStore,
		Execution:     executionEngine,
		Opportunities: opportunities,
		Positions:     positions,
		Storage:       storageStore,
		Logger:        logger,
	})

	server := api.NewServer(api.ServerConfig{
		Addr:          cfg.App.AdminAddr,
		TradingMode:   cfg.Trading.Mode,
		Connectors:    connectors,
		Books:         books,
		Funding:       fundingStore,
		Opportunities: opportunities,
		Positions:     positions,
		Storage:       storageStore,
		Fees:          feeView(cfg.Fees),
		Logger:        logger,
	})

	return &Runtime{
		cfg:           cfg,
		logger:        logger,
		connectors:    connectors,
		books:         books,
		funding:       fundingStore,
		opportunities: opportunities,
		positions:     positions,
		storage:       storageStore,
		scanner:       scanner,
		apiServer:     server,
	}, nil
}

func feeView(fees map[string]FeeConfig) map[string]api.FeeView {
	out := make(map[string]api.FeeView, len(fees))
	for exchange, fee := range fees {
		out[exchange] = api.FeeView{
			MakerPct: fee.MakerPct,
			TakerPct: fee.TakerPct,
		}
	}
	return out
}

func (r *Runtime) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	updates := make(chan exchange.OrderBookSnapshot, 1024)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.storage.Run(ctx)
	}()

	for _, connector := range r.connectors {
		connector := connector
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := connector.Start(ctx, r.cfg.Trading.Symbols, updates); err != nil && ctx.Err() == nil {
				r.logger.Warn("connector stopped", "exchange", connector.Name(), "error", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.runScanner(ctx, updates)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.logger.Info("admin api listening", "addr", r.cfg.App.AdminAddr)
		if err := r.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			r.logger.Error("admin api failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = r.apiServer.Shutdown(shutdownCtx)
	wg.Wait()
	return nil
}

func (r *Runtime) runScanner(ctx context.Context, updates <-chan exchange.OrderBookSnapshot) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			r.books.Apply(update)
			r.funding.ApplySyntheticFromOrderBook(update)
			r.storage.Enqueue(storage.Event{Type: storage.EventOrderBook, OrderBook: update})
			r.scanner.OnOrderBookUpdate(ctx, update)
		}
	}
}

func buildConnectors(cfg Config) map[exchange.Name]exchange.Connector {
	all := map[exchange.Name]exchange.Connector{
		exchange.Bybit:  bybit.New(),
		exchange.OKX:    okx.New(),
		exchange.HTX:    htx.New(),
		exchange.KuCoin: kucoin.New(),
	}
	enabled := make(map[exchange.Name]exchange.Connector)
	for name, connector := range all {
		if cfg.Exchanges[string(name)].Enabled {
			enabled[name] = connector
		}
	}
	return enabled
}
