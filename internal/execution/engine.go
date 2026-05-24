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
	LongOrder     exchange.OrderResult
	ShortOrder    exchange.OrderResult
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
			LongOrder:     paperOrder(opportunity, exchange.Buy),
			ShortOrder:    paperOrder(opportunity, exchange.Sell),
			Status:        "paper_open",
			StartedAt:     startedAt,
			FinishedAt:    time.Now().UTC(),
		}, nil
	}

	longConnector, ok := e.connectors[opportunity.LongExchange]
	if !ok {
		return Result{}, fmt.Errorf("missing long connector: %s", opportunity.LongExchange)
	}
	shortConnector, ok := e.connectors[opportunity.ShortExchange]
	if !ok {
		return Result{}, fmt.Errorf("missing short connector: %s", opportunity.ShortExchange)
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
		result, err := longConnector.PlaceOrder(orderCtx, e.orderRequest(opportunity, exchange.Buy))
		responses <- orderResponse{side: exchange.Buy, result: result, err: err}
	}()

	go func() {
		result, err := shortConnector.PlaceOrder(orderCtx, e.orderRequest(opportunity, exchange.Sell))
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
			result.LongOrder = response.result
		} else {
			result.ShortOrder = response.result
		}
	}

	result.FinishedAt = time.Now().UTC()
	if result.Status == "submitted" {
		result.Status = "live_open"
	}
	return result, nil
}

func (e Engine) orderRequest(opportunity strategy.Opportunity, side exchange.OrderSide) exchange.OrderRequest {
	price := opportunity.LongEntryPrice
	if side == exchange.Sell {
		price = opportunity.ShortEntryPrice
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
	price := opportunity.LongEntryPrice
	exchangeName := opportunity.LongExchange
	if side == exchange.Sell {
		price = opportunity.ShortEntryPrice
		exchangeName = opportunity.ShortExchange
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
