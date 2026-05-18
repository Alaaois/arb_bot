# Crypto Arbitrage Bot

Go-first MVP for cross-exchange crypto arbitrage analysis and paper execution.

Supported exchange connectors in the initial skeleton:

- Bybit
- OKX
- HTX
- KuCoin

The current implementation is intentionally safe by default: trading is disabled and execution runs in paper mode.

Market data and history are persisted to Redis when running through Docker Compose.

## Commands

```text
make test
make build
make run
make docker-up
make docker-down
```

## Current Scope

- in-memory order books;
- cross-exchange opportunity estimation;
- risk checks;
- paper execution;
- admin health/status API;
- public WebSocket connectors for Bybit, OKX, HTX and KuCoin;
- Redis-backed history for spreads and approved opportunities.

## Docker

```text
docker compose up --build
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/exchanges
curl http://127.0.0.1:8080/fees
curl http://127.0.0.1:8080/orderbooks
curl http://127.0.0.1:8080/spreads
curl http://127.0.0.1:8080/opportunities
curl "http://127.0.0.1:8080/history/spreads?limit=20"
curl "http://127.0.0.1:8080/history/opportunities?limit=20"
```
