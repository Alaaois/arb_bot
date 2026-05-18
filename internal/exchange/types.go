package exchange

import (
	"context"
	"time"
)

type Name string

const (
	Bybit  Name = "bybit"
	OKX    Name = "okx"
	HTX    Name = "htx"
	KuCoin Name = "kucoin"
)

type Level struct {
	Price    float64
	Quantity float64
}

type OrderBookSnapshot struct {
	Exchange  Name
	Symbol    string
	Bids      []Level
	Asks      []Level
	UpdatedAt time.Time
}

type OrderSide string

const (
	Buy  OrderSide = "buy"
	Sell OrderSide = "sell"
)

type OrderRequest struct {
	ClientOrderID string
	Symbol        string
	Side          OrderSide
	Price         float64
	Quantity      float64
	OrderType     string
}

type OrderStatus string

const (
	OrderAccepted        OrderStatus = "accepted"
	OrderRejected        OrderStatus = "rejected"
	OrderFilled          OrderStatus = "filled"
	OrderPartiallyFilled OrderStatus = "partially_filled"
	OrderCanceled        OrderStatus = "canceled"
	OrderUnknown         OrderStatus = "unknown"
)

type OrderResult struct {
	Exchange       Name
	ClientOrderID  string
	ExternalID     string
	Status         OrderStatus
	FilledQuantity float64
	AveragePrice   float64
	RawMessage     string
}

type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

type Connector interface {
	Name() Name
	Start(ctx context.Context, symbols []string, updates chan<- OrderBookSnapshot) error
	PlaceOrder(ctx context.Context, req OrderRequest) (OrderResult, error)
	GetOrder(ctx context.Context, clientOrderID string) (OrderResult, error)
	Balances(ctx context.Context) ([]Balance, error)
}
