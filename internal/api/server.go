package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/marketdata"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/storage"
	"crypto-arbitrage-bot/internal/strategy"
)

type ServerConfig struct {
	Addr          string
	TradingMode   string
	Connectors    map[exchange.Name]exchange.Connector
	Books         *marketdata.Store
	Opportunities *state.OpportunityLog
	Storage       *storage.Store
	Fees          map[string]FeeView
	Logger        *slog.Logger
}

type FeeView struct {
	MakerPct float64 `json:"maker_pct"`
	TakerPct float64 `json:"taker_pct"`
}

func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"mode":   cfg.TradingMode,
		})
	})

	mux.HandleFunc("GET /exchanges", func(w http.ResponseWriter, r *http.Request) {
		exchanges := make([]string, 0, len(cfg.Connectors))
		for name := range cfg.Connectors {
			exchanges = append(exchanges, string(name))
		}
		writeJSON(w, http.StatusOK, map[string]any{"exchanges": exchanges})
	})

	mux.HandleFunc("GET /fees", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Fees})
	})

	mux.HandleFunc("GET /opportunities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Opportunities.List()})
	})

	mux.HandleFunc("GET /orderbooks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Books.SnapshotAll()})
	})

	mux.HandleFunc("GET /spreads", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": calculateSpreads(cfg.Books, cfg.Fees)})
	})

	mux.HandleFunc("GET /history/spreads", func(w http.ResponseWriter, r *http.Request) {
		items, err := cfg.Storage.RecentSpreads(r.Context(), limitFromRequest(r, 100))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("GET /history/opportunities", func(w http.ResponseWriter, r *http.Request) {
		items, err := cfg.Storage.RecentOpportunities(r.Context(), limitFromRequest(r, 100))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
}

type SpreadView struct {
	Symbol       string        `json:"symbol"`
	BuyExchange  exchange.Name `json:"buy_exchange"`
	SellExchange exchange.Name `json:"sell_exchange"`
	BuyPrice     float64       `json:"buy_price"`
	SellPrice    float64       `json:"sell_price"`
	Quantity     float64       `json:"quantity"`
	NotionalUSD  float64       `json:"notional_usd"`
	GrossProfit  float64       `json:"gross_profit"`
	NetProfit    float64       `json:"net_profit"`
	NetProfitPct float64       `json:"net_profit_pct"`
}

func calculateSpreads(books *marketdata.Store, fees map[string]FeeView) []SpreadView {
	all := books.SnapshotAll()
	estimator := strategy.NewEstimator()
	var spreads []SpreadView
	for symbol, byExchange := range all {
		for buyExchange, buyBook := range byExchange {
			for sellExchange, sellBook := range byExchange {
				if buyExchange == sellExchange {
					continue
				}
				opportunity, ok, _ := estimator.EstimateCrossExchange(strategy.EstimateInput{
					Symbol:       symbol,
					BuyBook:      buyBook,
					SellBook:     sellBook,
					BuyFeePct:    takerFeePct(fees, buyExchange),
					SellFeePct:   takerFeePct(fees, sellExchange),
					MaxNotional:  100,
					SafetyBuffer: 0.10,
				})
				if !ok {
					continue
				}
				spreads = append(spreads, SpreadView{
					Symbol:       opportunity.Symbol,
					BuyExchange:  opportunity.BuyExchange,
					SellExchange: opportunity.SellExchange,
					BuyPrice:     opportunity.BuyPrice,
					SellPrice:    opportunity.SellPrice,
					Quantity:     opportunity.Quantity,
					NotionalUSD:  opportunity.NotionalUSD,
					GrossProfit:  opportunity.GrossProfit,
					NetProfit:    opportunity.NetProfit,
					NetProfitPct: opportunity.NetProfitPct,
				})
			}
		}
	}
	return spreads
}

func takerFeePct(fees map[string]FeeView, exchange exchange.Name) float64 {
	if fee, ok := fees[string(exchange)]; ok {
		return fee.TakerPct
	}
	return 0.10
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func limitFromRequest(r *http.Request, fallback int64) int64 {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return fallback
	}
	limit, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}
