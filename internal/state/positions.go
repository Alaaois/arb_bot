package state

import (
	"sync"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

type Leg struct {
	Exchange   exchange.Name `json:"exchange"`
	EntryPrice float64       `json:"entry_price"`
	ExitPrice  float64       `json:"exit_price"`
	FeeUSD     float64       `json:"fee_usd"`
}

type PositionStatus string

const (
	PositionOpening PositionStatus = "opening"
	PositionOpen    PositionStatus = "open"
	PositionClosing PositionStatus = "closing"
	PositionClosed  PositionStatus = "closed"
)

type Position struct {
	ID                string         `json:"id"`
	Symbol            string         `json:"symbol"`
	LongLeg           Leg            `json:"long_leg"`
	ShortLeg          Leg            `json:"short_leg"`
	Quantity          float64        `json:"quantity"`
	Status            PositionStatus `json:"status"`
	OpenedAt          time.Time      `json:"opened_at"`
	ClosedAt          time.Time      `json:"closed_at"`
	TargetFundingTime time.Time      `json:"target_funding_time"`
	ExpectedCarryUSD  float64        `json:"expected_carry_usd"`
	AccruedFundingUSD float64        `json:"accrued_funding_usd"`
	RealizedPnLUSD    float64        `json:"realized_pnl_usd"`
	UnrealizedPnLUSD  float64        `json:"unrealized_pnl_usd"`
	CloseReason       string         `json:"close_reason"`
	LastUpdatedAt     time.Time      `json:"last_updated_at"`
}

type PositionBook struct {
	mu      sync.RWMutex
	open    map[string]Position
	history []Position
	limit   int
}

func NewPositionBook(limit int) *PositionBook {
	if limit <= 0 {
		limit = 100
	}
	return &PositionBook{
		open:  make(map[string]Position),
		limit: limit,
	}
}

func (b *PositionBook) Add(position Position) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.open[position.ID] = position
}

func (b *PositionBook) Update(position Position) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.open[position.ID]; ok {
		b.open[position.ID] = position
	}
}

func (b *PositionBook) Close(position Position) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.open, position.ID)
	b.history = append([]Position{position}, b.history...)
	if len(b.history) > b.limit {
		b.history = b.history[:b.limit]
	}
}

func (b *PositionBook) HasOpen(symbol string, longExchange, shortExchange exchange.Name) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, position := range b.open {
		if position.Symbol == symbol &&
			position.LongLeg.Exchange == longExchange &&
			position.ShortLeg.Exchange == shortExchange {
			return true
		}
	}
	return false
}

func (b *PositionBook) OpenCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.open)
}

func (b *PositionBook) OpenPositions() []Position {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Position, 0, len(b.open))
	for _, position := range b.open {
		out = append(out, position)
	}
	return out
}

func (b *PositionBook) OpenPositionsBySymbol(symbol string) []Position {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []Position
	for _, position := range b.open {
		if position.Symbol == symbol {
			out = append(out, position)
		}
	}
	return out
}

func (b *PositionBook) History() []Position {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]Position(nil), b.history...)
}

func (b *PositionBook) Summary() map[string]float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	summary := map[string]float64{
		"open_positions": float64(len(b.open)),
	}
	for _, position := range b.open {
		summary["open_unrealized_pnl_usd"] += position.UnrealizedPnLUSD
		summary["open_accrued_funding_usd"] += position.AccruedFundingUSD
	}
	for _, position := range b.history {
		summary["realized_pnl_usd"] += position.RealizedPnLUSD
		summary["realized_funding_usd"] += position.AccruedFundingUSD
	}
	return summary
}
