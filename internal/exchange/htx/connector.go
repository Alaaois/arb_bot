package htx

import "crypto-arbitrage-bot/internal/exchange"

func New() exchange.Connector {
	return exchange.NewPublicWSConnector(exchange.PublicWSConfig{Name: exchange.HTX})
}
