package strategy

import (
	"fmt"
	"strings"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

func NewOpportunityID(symbol string, buyExchange, sellExchange exchange.Name, detectedAt time.Time) string {
	normalizedSymbol := strings.NewReplacer("/", "", "-", "").Replace(symbol)
	return fmt.Sprintf("%s-%s-%s-%d", normalizedSymbol, buyExchange, sellExchange, detectedAt.UnixNano())
}
