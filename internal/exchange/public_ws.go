package exchange

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type PublicWSConfig struct {
	Name Name
}

type PublicWSConnector struct {
	name       Name
	httpClient *http.Client
	dialer     *websocket.Dialer
}

func NewPublicWSConnector(cfg PublicWSConfig) *PublicWSConnector {
	return &PublicWSConnector{
		name: cfg.Name,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

func (c *PublicWSConnector) Name() Name {
	return c.name
}

func (c *PublicWSConnector) Start(ctx context.Context, symbols []string, updates chan<- OrderBookSnapshot) error {
	for {
		if err := c.run(ctx, symbols, updates); err != nil && ctx.Err() == nil {
			time.Sleep(2 * time.Second)
			continue
		}
		return ctx.Err()
	}
}

func (c *PublicWSConnector) PlaceOrder(ctx context.Context, req OrderRequest) (OrderResult, error) {
	return OrderResult{
		Exchange:      c.name,
		ClientOrderID: req.ClientOrderID,
		Status:        OrderUnknown,
		RawMessage:    "public market-data connector does not support trading",
	}, ErrLiveTradingNotImplemented
}

func (c *PublicWSConnector) GetOrder(ctx context.Context, clientOrderID string) (OrderResult, error) {
	return OrderResult{
		Exchange:      c.name,
		ClientOrderID: clientOrderID,
		Status:        OrderUnknown,
		RawMessage:    "public market-data connector does not support private order lookup",
	}, ErrLiveTradingNotImplemented
}

func (c *PublicWSConnector) Balances(ctx context.Context) ([]Balance, error) {
	return nil, ErrLiveTradingNotImplemented
}

func (c *PublicWSConnector) run(ctx context.Context, symbols []string, updates chan<- OrderBookSnapshot) error {
	endpoint, err := c.endpoint(ctx)
	if err != nil {
		return err
	}

	conn, _, err := c.dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := c.subscribe(conn, symbols); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		if c.name == HTX && messageType == websocket.BinaryMessage {
			payload, err = ungzip(payload)
			if err != nil {
				return err
			}
		}

		if handled, err := c.handleControl(conn, payload); handled || err != nil {
			if err != nil {
				return err
			}
			continue
		}

		snapshot, ok := c.parseSnapshot(payload)
		if !ok {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case updates <- snapshot:
		}
	}
}

func (c *PublicWSConnector) endpoint(ctx context.Context) (string, error) {
	switch c.name {
	case Bybit:
		return "wss://stream.bybit.com/v5/public/spot", nil
	case OKX:
		return "wss://ws.okx.com:8443/ws/v5/public", nil
	case HTX:
		return "wss://api.huobi.pro/ws", nil
	case KuCoin:
		return c.kucoinEndpoint(ctx)
	default:
		return "", fmt.Errorf("unsupported exchange: %s", c.name)
	}
}

func (c *PublicWSConnector) subscribe(conn *websocket.Conn, symbols []string) error {
	switch c.name {
	case Bybit:
		args := make([]string, 0, len(symbols))
		for _, symbol := range symbols {
			args = append(args, "orderbook.1."+bybitSymbol(symbol))
		}
		return conn.WriteJSON(map[string]any{"op": "subscribe", "args": args})
	case OKX:
		args := make([]map[string]string, 0, len(symbols))
		for _, symbol := range symbols {
			args = append(args, map[string]string{"channel": "books5", "instId": dashSymbol(symbol)})
		}
		return conn.WriteJSON(map[string]any{"op": "subscribe", "args": args})
	case HTX:
		for _, symbol := range symbols {
			req := map[string]string{
				"sub": marketHTXSymbol(symbol) + ".depth.step0",
				"id":  fmt.Sprintf("%d", time.Now().UnixNano()),
			}
			if err := conn.WriteJSON(req); err != nil {
				return err
			}
		}
		return nil
	case KuCoin:
		for _, symbol := range symbols {
			req := map[string]any{
				"id":             fmt.Sprintf("%d", time.Now().UnixNano()),
				"type":           "subscribe",
				"topic":          "/spotMarket/level2Depth5:" + dashSymbol(symbol),
				"privateChannel": false,
				"response":       true,
			}
			if err := conn.WriteJSON(req); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported exchange: %s", c.name)
	}
}

func (c *PublicWSConnector) handleControl(conn *websocket.Conn, payload []byte) (bool, error) {
	switch c.name {
	case HTX:
		var msg struct {
			Ping int64 `json:"ping"`
		}
		if json.Unmarshal(payload, &msg) == nil && msg.Ping > 0 {
			return true, conn.WriteJSON(map[string]int64{"pong": msg.Ping})
		}
	case KuCoin:
		var msg struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(payload, &msg) == nil && msg.Type == "ping" {
			return true, conn.WriteJSON(map[string]string{"id": msg.ID, "type": "pong"})
		}
	}
	return false, nil
}

func (c *PublicWSConnector) parseSnapshot(payload []byte) (OrderBookSnapshot, bool) {
	switch c.name {
	case Bybit:
		return parseBybit(payload)
	case OKX:
		return parseOKX(payload)
	case HTX:
		return parseHTX(payload)
	case KuCoin:
		return parseKuCoin(payload)
	default:
		return OrderBookSnapshot{}, false
	}
}

func parseBybit(payload []byte) (OrderBookSnapshot, bool) {
	var msg struct {
		Topic string `json:"topic"`
		Data  struct {
			Symbol string     `json:"s"`
			Bids   [][]string `json:"b"`
			Asks   [][]string `json:"a"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &msg) != nil || !strings.HasPrefix(msg.Topic, "orderbook.") {
		return OrderBookSnapshot{}, false
	}
	return OrderBookSnapshot{
		Exchange:  Bybit,
		Symbol:    slashSymbol(msg.Data.Symbol),
		Bids:      trimLevels(parseStringLevels(msg.Data.Bids), 20),
		Asks:      trimLevels(parseStringLevels(msg.Data.Asks), 20),
		UpdatedAt: time.Now().UTC(),
	}, true
}

func parseOKX(payload []byte) (OrderBookSnapshot, bool) {
	var msg struct {
		Arg struct {
			Channel string `json:"channel"`
			InstID  string `json:"instId"`
		} `json:"arg"`
		Data []struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &msg) != nil || msg.Arg.Channel != "books5" || len(msg.Data) == 0 {
		return OrderBookSnapshot{}, false
	}
	return OrderBookSnapshot{
		Exchange:  OKX,
		Symbol:    strings.ReplaceAll(msg.Arg.InstID, "-", "/"),
		Bids:      trimLevels(parseStringLevels(msg.Data[0].Bids), 20),
		Asks:      trimLevels(parseStringLevels(msg.Data[0].Asks), 20),
		UpdatedAt: time.Now().UTC(),
	}, true
}

func parseHTX(payload []byte) (OrderBookSnapshot, bool) {
	var msg struct {
		Ch   string `json:"ch"`
		Tick struct {
			Bids [][]float64 `json:"bids"`
			Asks [][]float64 `json:"asks"`
		} `json:"tick"`
	}
	if json.Unmarshal(payload, &msg) != nil || !strings.Contains(msg.Ch, ".depth.") {
		return OrderBookSnapshot{}, false
	}
	parts := strings.Split(msg.Ch, ".")
	if len(parts) < 3 {
		return OrderBookSnapshot{}, false
	}
	return OrderBookSnapshot{
		Exchange:  HTX,
		Symbol:    slashSymbol(strings.ToUpper(parts[1])),
		Bids:      trimLevels(parseFloatLevels(msg.Tick.Bids), 20),
		Asks:      trimLevels(parseFloatLevels(msg.Tick.Asks), 20),
		UpdatedAt: time.Now().UTC(),
	}, true
}

func parseKuCoin(payload []byte) (OrderBookSnapshot, bool) {
	var msg struct {
		Type  string `json:"type"`
		Topic string `json:"topic"`
		Data  struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &msg) != nil || msg.Type != "message" || !strings.HasPrefix(msg.Topic, "/spotMarket/level2Depth5:") {
		return OrderBookSnapshot{}, false
	}
	symbol := strings.TrimPrefix(msg.Topic, "/spotMarket/level2Depth5:")
	return OrderBookSnapshot{
		Exchange:  KuCoin,
		Symbol:    strings.ReplaceAll(symbol, "-", "/"),
		Bids:      trimLevels(parseStringLevels(msg.Data.Bids), 20),
		Asks:      trimLevels(parseStringLevels(msg.Data.Asks), 20),
		UpdatedAt: time.Now().UTC(),
	}, true
}

func (c *PublicWSConnector) kucoinEndpoint(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.kucoin.com/api/v1/bullet-public", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Code string `json:"code"`
		Data struct {
			Token           string `json:"token"`
			InstanceServers []struct {
				Endpoint string `json:"endpoint"`
			} `json:"instanceServers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Data.Token == "" || len(payload.Data.InstanceServers) == 0 {
		return "", fmt.Errorf("kucoin public bullet response missing token or endpoint")
	}
	connectID := fmt.Sprintf("%d", time.Now().UnixNano())
	return payload.Data.InstanceServers[0].Endpoint + "?token=" + payload.Data.Token + "&connectId=" + connectID, nil
}

func parseStringLevels(raw [][]string) []Level {
	levels := make([]Level, 0, len(raw))
	for _, row := range raw {
		if len(row) < 2 {
			continue
		}
		price, err1 := strconv.ParseFloat(row[0], 64)
		qty, err2 := strconv.ParseFloat(row[1], 64)
		if err1 == nil && err2 == nil && price > 0 && qty > 0 {
			levels = append(levels, Level{Price: price, Quantity: qty})
		}
	}
	return levels
}

func parseFloatLevels(raw [][]float64) []Level {
	levels := make([]Level, 0, len(raw))
	for _, row := range raw {
		if len(row) >= 2 && row[0] > 0 && row[1] > 0 {
			levels = append(levels, Level{Price: row[0], Quantity: row[1]})
		}
	}
	return levels
}

func trimLevels(levels []Level, limit int) []Level {
	if len(levels) <= limit {
		return levels
	}
	return levels[:limit]
}

func bybitSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "/", "")
}

func dashSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "/", "-")
}

func marketHTXSymbol(symbol string) string {
	return "market." + strings.ToLower(strings.ReplaceAll(symbol, "/", ""))
}

func slashSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
	if strings.HasSuffix(symbol, "USDT") && len(symbol) > 4 {
		return strings.TrimSuffix(symbol, "USDT") + "/USDT"
	}
	return symbol
}

func ungzip(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
