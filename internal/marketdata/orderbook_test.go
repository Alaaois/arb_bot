package marketdata

import (
	"testing"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

func BenchmarkStoreSnapshot(b *testing.B) {
	store := NewStore()
	store.Apply(exchange.OrderBookSnapshot{
		Exchange:  exchange.Bybit,
		Symbol:    "BTC/USDT",
		Bids:      makeLevels(20, 100),
		Asks:      makeLevels(20, 101),
		UpdatedAt: time.Now(),
	})
	store.Apply(exchange.OrderBookSnapshot{
		Exchange:  exchange.OKX,
		Symbol:    "BTC/USDT",
		Bids:      makeLevels(20, 100),
		Asks:      makeLevels(20, 101),
		UpdatedAt: time.Now(),
	})
	store.Apply(exchange.OrderBookSnapshot{
		Exchange:  exchange.HTX,
		Symbol:    "BTC/USDT",
		Bids:      makeLevels(20, 100),
		Asks:      makeLevels(20, 101),
		UpdatedAt: time.Now(),
	})
	store.Apply(exchange.OrderBookSnapshot{
		Exchange:  exchange.KuCoin,
		Symbol:    "BTC/USDT",
		Bids:      makeLevels(20, 100),
		Asks:      makeLevels(20, 101),
		UpdatedAt: time.Now(),
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.Snapshot("BTC/USDT")
	}
}

func makeLevels(n int, startPrice float64) []exchange.Level {
	levels := make([]exchange.Level, n)
	for i := 0; i < n; i++ {
		levels[i] = exchange.Level{
			Price:    startPrice + float64(i),
			Quantity: 1,
		}
	}
	return levels
}
