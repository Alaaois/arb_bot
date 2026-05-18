package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/state"
	"crypto-arbitrage-bot/internal/strategy"

	"github.com/redis/go-redis/v9"
)

const (
	keyOrderBookLatest = "arb:orderbooks:latest"
	keySpreadHistory   = "arb:history:spreads"
	keyOpportunityHist = "arb:history:opportunities"
)

type Config struct {
	Addr             string
	Password         string
	DB               int
	HistoryLimit     int64
	OperationTimeout time.Duration
	Enabled          bool
}

type Store struct {
	client *redis.Client
	cfg    Config
	logger *slog.Logger
	events chan Event
}

type EventType string

const (
	EventOrderBook   EventType = "orderbook"
	EventSpread      EventType = "spread"
	EventOpportunity EventType = "opportunity"
)

type Event struct {
	Type        EventType
	OrderBook   exchange.OrderBookSnapshot
	Spread      SpreadRecord
	Opportunity state.OpportunityRecord
}

type SpreadRecord struct {
	ID           string        `json:"id"`
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
	DetectedAt   time.Time     `json:"detected_at"`
}

func NewStore(cfg Config, logger *slog.Logger) *Store {
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 1000
	}
	if cfg.OperationTimeout == 0 {
		cfg.OperationTimeout = 500 * time.Millisecond
	}
	return &Store{
		cfg:    cfg,
		logger: logger,
		events: make(chan Event, 4096),
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

func DisabledStore() *Store {
	return &Store{cfg: Config{Enabled: false}, events: make(chan Event)}
}

func (s *Store) Run(ctx context.Context) {
	if s == nil || !s.cfg.Enabled {
		<-ctx.Done()
		return
	}

	pingCtx, cancel := context.WithTimeout(ctx, s.cfg.OperationTimeout)
	err := s.client.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		s.logger.Warn("redis ping failed; storage writer will keep retrying per event", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			_ = s.client.Close()
			return
		case event := <-s.events:
			if err := s.writeEvent(ctx, event); err != nil {
				s.logger.Warn("redis write failed", "type", event.Type, "error", err)
			}
		}
	}
}

func (s *Store) Enqueue(event Event) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	select {
	case s.events <- event:
	default:
		s.logger.Warn("redis event queue full; dropping event", "type", event.Type)
	}
}

func (s *Store) RecentSpreads(ctx context.Context, limit int64) ([]SpreadRecord, error) {
	if s == nil || !s.cfg.Enabled {
		return nil, errors.New("redis storage disabled")
	}
	if limit <= 0 {
		limit = 100
	}
	values, err := s.lrange(ctx, keySpreadHistory, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SpreadRecord, 0, len(values))
	for _, value := range values {
		var record SpreadRecord
		if json.Unmarshal([]byte(value), &record) == nil {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s *Store) RecentOpportunities(ctx context.Context, limit int64) ([]state.OpportunityRecord, error) {
	if s == nil || !s.cfg.Enabled {
		return nil, errors.New("redis storage disabled")
	}
	if limit <= 0 {
		limit = 100
	}
	values, err := s.lrange(ctx, keyOpportunityHist, limit)
	if err != nil {
		return nil, err
	}
	out := make([]state.OpportunityRecord, 0, len(values))
	for _, value := range values {
		var record state.OpportunityRecord
		if json.Unmarshal([]byte(value), &record) == nil {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s *Store) writeEvent(ctx context.Context, event Event) error {
	eventCtx, cancel := context.WithTimeout(ctx, s.cfg.OperationTimeout)
	defer cancel()

	switch event.Type {
	case EventOrderBook:
		return s.writeOrderBook(eventCtx, event.OrderBook)
	case EventSpread:
		return s.pushJSON(eventCtx, keySpreadHistory, event.Spread)
	case EventOpportunity:
		return s.pushJSON(eventCtx, keyOpportunityHist, event.Opportunity)
	default:
		return fmt.Errorf("unknown storage event type: %s", event.Type)
	}
}

func (s *Store) writeOrderBook(ctx context.Context, snapshot exchange.OrderBookSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	field := string(snapshot.Exchange) + ":" + snapshot.Symbol
	return s.client.HSet(ctx, keyOrderBookLatest, field, payload).Err()
}

func (s *Store) pushJSON(ctx context.Context, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	pipe := s.client.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.LTrim(ctx, key, 0, s.cfg.HistoryLimit-1)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) lrange(ctx context.Context, key string, limit int64) ([]string, error) {
	opCtx, cancel := context.WithTimeout(ctx, s.cfg.OperationTimeout)
	defer cancel()
	return s.client.LRange(opCtx, key, 0, limit-1).Result()
}

func SpreadFromOpportunity(opportunity strategy.Opportunity) SpreadRecord {
	return SpreadRecord{
		ID:           opportunity.ID,
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
		DetectedAt:   opportunity.DetectedAt,
	}
}
