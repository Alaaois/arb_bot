# Crypto Arbitrage Bot

Go-first MVP for funding arbitrage analysis and paper execution on perpetual-style market data.

Supported exchange connectors in the initial skeleton:

- Bybit
- OKX
- HTX
- KuCoin

The current implementation is intentionally safe by default: live trading is not implemented and execution runs in analysis/paper mode.

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
- synthetic in-memory funding state derived from live exchange books;
- funding arbitrage opportunity estimation;
- funding-aware risk checks;
- paper position lifecycle with open/hold/close;
- admin health/status API;
- public WebSocket connectors for Bybit, OKX, HTX and KuCoin;
- Redis-backed history for approved opportunities and position events.

## Docker

```text
docker compose up --build
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/exchanges
curl http://127.0.0.1:8080/symbols
curl http://127.0.0.1:8080/fees
curl http://127.0.0.1:8080/funding
curl http://127.0.0.1:8080/opportunities
curl http://127.0.0.1:8080/positions
curl http://127.0.0.1:8080/pnl
curl "http://127.0.0.1:8080/positions/history?limit=20"
```

## Server Deploy

Минимальный сценарий для VPS/Linux-сервера:

```text
git clone <repo>
cd arb_bot
docker compose up -d --build
docker compose ps
docker compose logs -f arb-bot
```

Проверка после старта:

```text
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/funding
curl http://127.0.0.1:8080/opportunities
curl http://127.0.0.1:8080/positions
curl http://127.0.0.1:8080/pnl
```

Если нужен недельный анализ, контейнеры достаточно держать поднятыми 7 дней:

```text
docker compose up -d --build
docker compose logs -f arb-bot
```

История одобренных возможностей и paper positions сохраняется в Redis volume `redis-data`, поэтому после перезапуска контейнеров данные не теряются, пока volume не удален.
