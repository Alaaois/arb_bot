package exchange

import (
	"context"
	"errors"
	"time"
)

var ErrLiveTradingNotImplemented = errors.New("live trading connector is not implemented yet")

type StubConnector struct {
	name Name
}

func NewStubConnector(name Name) StubConnector {
	return StubConnector{name: name}
}

func (c StubConnector) Name() Name {
	return c.name
}

func (c StubConnector) Start(ctx context.Context, symbols []string, updates chan<- OrderBookSnapshot) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c StubConnector) PlaceOrder(ctx context.Context, req OrderRequest) (OrderResult, error) {
	return OrderResult{
		Exchange:      c.name,
		ClientOrderID: req.ClientOrderID,
		Status:        OrderUnknown,
		RawMessage:    "stub connector",
	}, ErrLiveTradingNotImplemented
}

func (c StubConnector) GetOrder(ctx context.Context, clientOrderID string) (OrderResult, error) {
	return OrderResult{
		Exchange:      c.name,
		ClientOrderID: clientOrderID,
		Status:        OrderUnknown,
		RawMessage:    "stub connector",
	}, ErrLiveTradingNotImplemented
}

func (c StubConnector) Balances(ctx context.Context) ([]Balance, error) {
	return nil, ErrLiveTradingNotImplemented
}

func DemoSnapshot(name Name, symbol string, bid, ask float64) OrderBookSnapshot {
	return OrderBookSnapshot{
		Exchange:  name,
		Symbol:    symbol,
		Bids:      []Level{{Price: bid, Quantity: 0.25}},
		Asks:      []Level{{Price: ask, Quantity: 0.25}},
		UpdatedAt: time.Now().UTC(),
	}
}
