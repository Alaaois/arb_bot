package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/funding"
	"crypto-arbitrage-bot/internal/marketdata"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/storage"
)

type ServerConfig struct {
	Addr          string
	TradingMode   string
	Connectors    map[exchange.Name]exchange.Connector
	Books         *marketdata.Store
	Funding       *funding.Store
	Opportunities *state.OpportunityLog
	Positions     *state.PositionBook
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

	mux.HandleFunc("GET /symbols", func(w http.ResponseWriter, r *http.Request) {
		all := cfg.Books.SnapshotAll()
		symbols := make([]string, 0, len(all))
		for symbol := range all {
			symbols = append(symbols, symbol)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": symbols})
	})

	mux.HandleFunc("GET /funding", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Funding.SnapshotAll()})
	})

	mux.HandleFunc("GET /opportunities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Opportunities.List()})
	})

	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": cfg.Positions.OpenPositions()})
	})

	mux.HandleFunc("GET /positions/history", func(w http.ResponseWriter, r *http.Request) {
		items, err := cfg.Storage.RecentPositions(r.Context(), limitFromRequest(r, 100))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("GET /pnl", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, cfg.Positions.Summary())
	})

	mux.HandleFunc("GET /metrics/latency", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"note": "latency metrics are tracked in-process; detailed histograms are not yet implemented",
		})
	})

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
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
