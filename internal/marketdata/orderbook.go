package marketdata

import (
	"sync"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

type Store struct {
	mu    sync.RWMutex
	books map[string]map[exchange.Name]exchange.OrderBookSnapshot
}

func NewStore() *Store {
	return &Store{
		books: make(map[string]map[exchange.Name]exchange.OrderBookSnapshot),
	}
}

func (s *Store) Apply(snapshot exchange.OrderBookSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.books[snapshot.Symbol]; !ok {
		s.books[snapshot.Symbol] = make(map[exchange.Name]exchange.OrderBookSnapshot)
	}
	s.books[snapshot.Symbol][snapshot.Exchange] = snapshot
}

func (s *Store) Snapshot(symbol string) map[exchange.Name]exchange.OrderBookSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byExchange := s.books[symbol]
	out := make(map[exchange.Name]exchange.OrderBookSnapshot, len(byExchange))
	for name, snapshot := range byExchange {
		out[name] = snapshot
	}
	return out
}

func (s *Store) SnapshotAll() map[string]map[exchange.Name]exchange.OrderBookSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]map[exchange.Name]exchange.OrderBookSnapshot, len(s.books))
	for symbol, byExchange := range s.books {
		out[symbol] = make(map[exchange.Name]exchange.OrderBookSnapshot, len(byExchange))
		for name, snapshot := range byExchange {
			out[symbol][name] = snapshot
		}
	}
	return out
}

func (s *Store) IsFresh(snapshot exchange.OrderBookSnapshot, maxAge time.Duration, now time.Time) bool {
	if snapshot.UpdatedAt.IsZero() {
		return false
	}
	return now.Sub(snapshot.UpdatedAt) <= maxAge
}
