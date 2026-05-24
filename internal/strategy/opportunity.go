package strategy

import (
	"fmt"
	"strings"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
)

func NewOpportunityID(symbol string, longExchange, shortExchange exchange.Name, detectedAt time.Time) string {
	normalizedSymbol := strings.NewReplacer("/", "", "-", "").Replace(symbol)
	return fmt.Sprintf("%s-%s-%s-%d", normalizedSymbol, longExchange, shortExchange, detectedAt.UnixNano())
}
