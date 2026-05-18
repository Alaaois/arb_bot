package execution

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"crypto-arbitrage-bot/internal/exchange"
	"crypto-arbitrage-bot/internal/strategy"
)

type Mode string

const (
	ModePaper Mode = "paper"
	ModeLive  Mode = "live"
)

type Config struct {
	Mode      string
	Timeout   time.Duration
	OrderType string
}

type Engine struct {
	mode       Mode
	timeout    time.Duration
	orderType  string
	connectors map[exchange.Name]exchange.Connector
	logger     *slog.Logger
}

type Result struct {
	OpportunityID string
	Mode          Mode
	BuyOrder      exchange.OrderResult
	SellOrder     exchange.OrderResult
	Status        string
	StartedAt     time.Time
	FinishedAt    time.Time
}

func NewEngine(cfg Config, connectors map[exchange.Name]exchange.Connector, logger *slog.Logger) Engine {
	return Engine{
		mode:       Mode(cfg.Mode),
		timeout:    cfg.Timeout,
		orderType:  cfg.OrderType,
		connectors: connectors,
		logger:     logger,
	}
}

func (e Engine) Execute(ctx context.Context, opportunity strategy.Opportunity) (Result, error) {
	startedAt := time.Now().UTC()
	if e.mode != ModeLive {
		return Result{
			OpportunityID: opportunity.ID,
			Mode:          e.mode,
			BuyOrder:      paperOrder(opportunity, exchange.Buy),
			SellOrder:     paperOrder(opportunity, exchange.Sell),
			Status:        "paper_filled",
			StartedAt:     startedAt,
			FinishedAt:    time.Now().UTC(),
		}, nil
	}

	buyConnector, ok := e.connectors[opportunity.BuyExchange]
	if !ok {
		return Result{}, fmt.Errorf("missing buy connector: %s", opportunity.BuyExchange)
	}
	sellConnector, ok := e.connectors[opportunity.SellExchange]
	if !ok {
		return Result{}, fmt.Errorf("missing sell connector: %s", opportunity.SellExchange)
	}

	orderCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	type orderResponse struct {
		side   exchange.OrderSide
		result exchange.OrderResult
		err    error
	}
	responses := make(chan orderResponse, 2)

	go func() {
		result, err := buyConnector.PlaceOrder(orderCtx, e.orderRequest(opportunity, exchange.Buy))
		responses <- orderResponse{side: exchange.Buy, result: result, err: err}
	}()

	go func() {
		result, err := sellConnector.PlaceOrder(orderCtx, e.orderRequest(opportunity, exchange.Sell))
		responses <- orderResponse{side: exchange.Sell, result: result, err: err}
	}()

	result := Result{
		OpportunityID: opportunity.ID,
		Mode:          e.mode,
		Status:        "submitted",
		StartedAt:     startedAt,
	}

	for i := 0; i < 2; i++ {
		response := <-responses
		if response.err != nil {
			e.logger.Warn("order placement failed", "side", response.side, "error", response.err)
			result.Status = "needs_reconcile"
			continue
		}
		if response.side == exchange.Buy {
			result.BuyOrder = response.result
		} else {
			result.SellOrder = response.result
		}
	}

	result.FinishedAt = time.Now().UTC()
	if result.Status == "submitted" {
		result.Status = "placed"
	}
	return result, nil
}

func (e Engine) orderRequest(opportunity strategy.Opportunity, side exchange.OrderSide) exchange.OrderRequest {
	price := opportunity.BuyPrice
	if side == exchange.Sell {
		price = opportunity.SellPrice
	}
	return exchange.OrderRequest{
		ClientOrderID: fmt.Sprintf("%s-%s", opportunity.ID, side),
		Symbol:        opportunity.Symbol,
		Side:          side,
		Price:         price,
		Quantity:      opportunity.Quantity,
		OrderType:     e.orderType,
	}
}

func paperOrder(opportunity strategy.Opportunity, side exchange.OrderSide) exchange.OrderResult {
	price := opportunity.BuyPrice
	exchangeName := opportunity.BuyExchange
	if side == exchange.Sell {
		price = opportunity.SellPrice
		exchangeName = opportunity.SellExchange
	}
	return exchange.OrderResult{
		Exchange:       exchangeName,
		ClientOrderID:  fmt.Sprintf("%s-%s", opportunity.ID, side),
		Status:         exchange.OrderFilled,
		FilledQuantity: opportunity.Quantity,
		AveragePrice:   price,
		RawMessage:     "paper execution",
	}
}
