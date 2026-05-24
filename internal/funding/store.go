package funding

import (
	"hash/fnv"
	"sync"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

type Snapshot struct {
	Exchange            exchange.Name `json:"exchange"`
	Symbol              string        `json:"symbol"`
	MarkPrice           float64       `json:"mark_price"`
	IndexPrice          float64       `json:"index_price"`
	FundingRatePct      float64       `json:"funding_rate_pct"`
	PredictedFundingPct float64       `json:"predicted_funding_pct"`
	NextFundingTime     time.Time     `json:"next_funding_time"`
	ObservedAt          time.Time     `json:"observed_at"`
}

type Config struct {
	DefaultRatePct  float64
	FundingInterval time.Duration
}

type Store struct {
	mu        sync.RWMutex
	snapshots map[string]map[exchange.Name]Snapshot
	cfg       Config
}

func NewStore(cfg Config) *Store {
	if cfg.FundingInterval <= 0 {
		cfg.FundingInterval = 8 * time.Hour
	}
	if cfg.DefaultRatePct == 0 {
		cfg.DefaultRatePct = 0.01
	}
	return &Store{
		snapshots: make(map[string]map[exchange.Name]Snapshot),
		cfg:       cfg,
	}
}

func (s *Store) Apply(snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.snapshots[snapshot.Symbol]; !ok {
		s.snapshots[snapshot.Symbol] = make(map[exchange.Name]Snapshot)
	}
	s.snapshots[snapshot.Symbol][snapshot.Exchange] = snapshot
}

func (s *Store) Snapshot(symbol string) map[exchange.Name]Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byExchange := s.snapshots[symbol]
	out := make(map[exchange.Name]Snapshot, len(byExchange))
	for name, snapshot := range byExchange {
		out[name] = snapshot
	}
	return out
}

func (s *Store) SnapshotAll() map[string]map[exchange.Name]Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]map[exchange.Name]Snapshot, len(s.snapshots))
	for symbol, byExchange := range s.snapshots {
		out[symbol] = make(map[exchange.Name]Snapshot, len(byExchange))
		for name, snapshot := range byExchange {
			out[symbol][name] = snapshot
		}
	}
	return out
}

func (s *Store) ApplySyntheticFromOrderBook(book exchange.OrderBookSnapshot) Snapshot {
	snapshot := SyntheticSnapshot(book, s.cfg)
	s.Apply(snapshot)
	return snapshot
}

func SyntheticSnapshot(book exchange.OrderBookSnapshot, cfg Config) Snapshot {
	mark := midpoint(book)
	if mark == 0 {
		mark = firstPrice(book)
	}
	nextFundingTime := nextFundingWindow(book.UpdatedAt, cfg.FundingInterval)
	rate := syntheticRate(book.Exchange, book.Symbol, cfg.DefaultRatePct)
	return Snapshot{
		Exchange:            book.Exchange,
		Symbol:              book.Symbol,
		MarkPrice:           mark,
		IndexPrice:          mark,
		FundingRatePct:      rate,
		PredictedFundingPct: rate,
		NextFundingTime:     nextFundingTime,
		ObservedAt:          book.UpdatedAt,
	}
}

func nextFundingWindow(now time.Time, interval time.Duration) time.Time {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	windowStart := now.Truncate(interval)
	if windowStart.Equal(now) {
		return now.Add(interval)
	}
	return windowStart.Add(interval)
}

func midpoint(book exchange.OrderBookSnapshot) float64 {
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return 0
	}
	return (book.Bids[0].Price + book.Asks[0].Price) / 2
}

func firstPrice(book exchange.OrderBookSnapshot) float64 {
	switch {
	case len(book.Asks) > 0:
		return book.Asks[0].Price
	case len(book.Bids) > 0:
		return book.Bids[0].Price
	default:
		return 0
	}
}

func syntheticRate(name exchange.Name, symbol string, defaultRate float64) float64 {
	base := map[exchange.Name]float64{
		exchange.Bybit:  1.2,
		exchange.OKX:    -0.6,
		exchange.HTX:    0.8,
		exchange.KuCoin: -0.4,
	}
	scale := base[name]
	if scale == 0 {
		scale = 1
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(symbol))
	jitter := float64(int(hasher.Sum32()%9)-4) * defaultRate * 0.05
	return defaultRate*scale + jitter
}
