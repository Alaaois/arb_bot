# Design Doc: анализатор и автоматический бот для криптовалютного арбитража

## 1. Цель

Создать систему, которая:

- собирает рыночные данные с нескольких криптовалютных бирж;
- ищет арбитражные возможности с учетом комиссий, ликвидности, проскальзывания и задержек;
- оценивает риск сделки до исполнения;
- автоматически размещает ордера только в рамках заданных лимитов;
- ведет полный аудит решений, сделок, ошибок и финансового результата.

Система не должна торговать по "сырому" спреду. Каждая возможность должна проходить через модель реальной исполнимости: комиссии, глубина стакана, баланс на биржах, минимальные размеры ордеров, сетевые задержки, риск частичного исполнения и риск блокировки вывода.

## 2. Не цели первой версии

В первой версии не реализуются:

- DeFi-арбитраж через смарт-контракты;
- межсетевой арбитраж с автоматическим выводом средств между сетями;
- кредитное плечо и маржинальная торговля;
- высокочастотная торговля на уровне микросекунд;
- самостоятельное принятие решений без заранее заданных лимитов риска.

## 3. Основные сценарии

### 3.1. Аналитический режим

Бот только собирает данные, считает возможности и пишет их в базу:

- где возник спред;
- какой объем можно было исполнить;
- какой была ожидаемая прибыль после комиссий;
- сколько времени возможность оставалась доступной;
- была ли она исполнима с учетом стакана.

Этот режим нужен для калибровки модели до включения реальной торговли.

### 3.2. Paper trading

Бот симулирует сделки:

- использует реальные стаканы и балансы;
- считает частичное исполнение;
- применяет комиссии;
- фиксирует виртуальный PnL;
- сравнивает ожидаемый и фактический результат по рыночным данным после сигнала.

### 3.3. Live trading

Бот размещает реальные ордера только если:

- возможность проходит все фильтры риска;
- оба ордера можно исполнить почти одновременно;
- нужные активы уже находятся на обеих биржах;
- размер сделки не превышает лимиты;
- ожидаемая прибыль выше минимального порога безопасности.

## 4. Типы арбитража

### 4.1. Межбиржевой арбитраж без вывода средств

Пример:

- купить BTC/USDT на Binance;
- продать BTC/USDT на Kraken;
- прибыль возникает из разницы цен.

Условие: BTC и USDT заранее распределены между биржами. После сделок балансы становятся перекошенными, поэтому нужен отдельный процесс ребалансировки.

Это основной тип для MVP.

### 4.2. Треугольный арбитраж внутри одной биржи

Пример:

- USDT -> BTC;
- BTC -> ETH;
- ETH -> USDT.

Плюс: нет межбиржевого риска.

Минусы:

- обычно ниже маржа;
- сильная зависимость от комиссий;
- требуется быстрое исполнение цепочки.

Можно добавить после MVP.

### 4.3. Межбиржевой арбитраж с выводом средств

Покупка на одной бирже, вывод актива и продажа на другой.

Для автоматического live trading в первой версии не рекомендуется из-за:

- времени подтверждения сети;
- риска остановки депозитов/выводов;
- переменной комиссии сети;
- риска изменения цены во время перевода.

## 5. Архитектура

```text
                 +--------------------+
                 |   Admin UI / CLI    |
                 +----------+---------+
                            |
                            v
+----------------+   +------+-------+   +----------------+
| Exchange APIs  |-->| Go Runtime   |-->| Market Storage |
+----------------+   | WebSocket    |   +----------------+
                     | Ingest       |
                     +------+-------+
                            |
                            v
                    +-------+--------+
                    | Opportunity    |
                    | Detector       |
                    +-------+--------+
                            |
                            v
                    +-------+--------+
                    | Risk Engine    |
                    +-------+--------+
                            |
                            v
                    +-------+--------+
                    | Execution      |
                    | Engine         |
                    +-------+--------+
                            |
                            v
                    +-------+--------+
                    | Audit / PnL    |
                    | Monitoring     |
                    +----------------+
```

Базовая реализация: один Go-бинарь с внутренними пакетами. Это проще для MVP, снижает межпроцессные задержки и упрощает согласованность состояния. Разделение на отдельные сервисы стоит делать позже, когда появятся измеренные узкие места.

## 6. Компоненты

### 6.1. Exchange Connector

Отвечает за интеграцию с биржами.

Функции:

- подключение к WebSocket для стаканов и сделок;
- REST-запросы для балансов, комиссий, правил торговой пары;
- размещение, отмена и проверка ордеров;
- нормализация форматов бирж в единый внутренний формат;
- обработка rate limits;
- восстановление после обрывов соединения.

В Go каждый connector работает как отдельный набор goroutines:

- WebSocket reader читает сообщения из сети;
- parser нормализует сообщения в общие структуры;
- order book updater применяет incremental updates;
- health monitor отслеживает heartbeat, lag и reconnects;
- REST client выполняет торговые и account-запросы с timeout и rate limiter.

Все внешние вызовы должны принимать `context.Context`, иметь deadline и возвращать типизированные ошибки. Нельзя допускать бесконечных сетевых ожиданий в execution path.

Для MVP можно начать с 2-3 бирж, например:

- Binance;
- Kraken;
- OKX;
- Bybit.

Точный список зависит от доступности аккаунтов, API-ключей, региона, KYC и комиссий.

### 6.2. Market Data Service

Собирает:

- best bid / best ask;
- стакан до нужной глубины;
- последние сделки;
- статус торговой пары;
- комиссии maker/taker;
- минимальный размер ордера;
- точность цены и количества.

Для поиска арбитража нужен не только top-of-book, а стакан. Возможность с прибылью на 0.2% может исчезнуть, если доступный объем на лучшей цене слишком мал.

Для минимизации задержек текущий стакан хранится в памяти процесса. PostgreSQL используется для аудита и исторических snapshots, но не должен быть частью hot path при принятии торгового решения.

Hot path:

```text
WebSocket message -> normalize -> in-memory order book -> opportunity detector -> risk engine -> execution
```

Cold path:

```text
snapshots/events -> async batch writer -> PostgreSQL/ClickHouse
```

### 6.3. Opportunity Detector

Ищет потенциальные сделки.

Для межбиржевого арбитража:

```text
buy_exchange.ask < sell_exchange.bid
```

Расчет:

```text
gross_spread = sell_bid - buy_ask
gross_spread_pct = gross_spread / buy_ask

net_profit =
  sell_revenue_after_fee
  - buy_cost_after_fee
  - estimated_slippage
  - safety_buffer
```

Сигнал создается только если:

- `net_profit > min_profit_abs`;
- `net_profit_pct > min_profit_pct`;
- доступный объем выше минимума;
- данные свежие;
- обе биржи доступны;
- есть нужные балансы.

### 6.4. Risk Engine

Центральный компонент защиты.

Проверки перед сделкой:

- максимальный размер одной сделки;
- максимальная дневная потеря;
- максимальная экспозиция на биржу;
- максимальная экспозиция на актив;
- минимальный ожидаемый PnL;
- максимальное допустимое проскальзывание;
- максимальный возраст рыночных данных;
- проверка, что торговая пара не в режиме maintenance;
- проверка, что API биржи отвечает стабильно;
- проверка, что баланс после сделки не станет ниже резервного минимума.

Если любая проверка не проходит, сделка не исполняется.

### 6.5. Execution Engine

Размещает ордера.

Для MVP предпочтительно:

- использовать IOC/FOK limit-ордера, если биржа поддерживает;
- не использовать market-ордера без жестких лимитов;
- размещать обе стороны сделки максимально близко по времени;
- уметь отменять остатки;
- уметь закрывать незакрытую сторону при частичном исполнении.

Проблема: одна сторона может исполниться, а вторая нет. Для этого нужен recovery flow:

1. Проверить фактическое исполнение обеих сторон.
2. Если одна сторона исполнилась частично, пересчитать остаточный риск.
3. Попробовать закрыть остаток в рамках допустимого убытка.
4. Если закрытие невозможно, перевести позицию в manual intervention.
5. Заблокировать новые сделки по этой паре до разбирательства.

Execution Engine должен быть написан без блокировок на долгие операции. Размещение двух сторон сделки выполняется через goroutines с общим `context.Context` и жестким timeout.

Пример поведения:

```text
create execution context, timeout 300-800 ms
send buy order goroutine
send sell order goroutine
wait for both results or timeout
reconcile order statuses
trigger recovery if needed
persist audit asynchronously
```

Для live trading нельзя полагаться только на ответ размещения ордера. После сетевой ошибки обязательно выполняется reconcile по `client_order_id`.

### 6.6. Portfolio / Balance Manager

Следит за балансами:

- свободный баланс;
- заблокированный в ордерах баланс;
- распределение активов по биржам;
- перекос после арбитражных сделок;
- необходимость ребалансировки.

Для межбиржевого арбитража без вывода средств нужно заранее держать активы на разных биржах.

Пример:

- Binance: USDT для покупки BTC;
- Kraken: BTC для продажи BTC.

После сделки:

- Binance получает BTC;
- Kraken получает USDT.

Чтобы продолжать торговать в том же направлении, понадобится ребалансировка или возможность торговать в обратном направлении.

### 6.7. Audit Log

Каждое решение должно быть записано:

- входные цены;
- глубина стакана;
- комиссии;
- рассчитанная прибыль;
- результат risk checks;
- отправленные ордера;
- ответы бирж;
- фактическое исполнение;
- итоговый PnL;
- ошибки и ретраи.

Аудит нужен для отладки, налогового учета, расследования инцидентов и улучшения модели.

### 6.8. Monitoring

Метрики:

- количество возможностей в минуту;
- количество отклоненных возможностей;
- причины отклонения;
- latency по биржам;
- WebSocket reconnects;
- REST error rate;
- PnL realized / unrealized;
- slippage;
- частичные исполнения;
- доля успешных сделок;
- баланс по биржам;
- дневной объем торгов;
- дневная прибыль/убыток.

Алерты:

- превышен дневной убыток;
- ошибка размещения ордера;
- частичное исполнение;
- расхождение локального баланса с биржей;
- отключение WebSocket;
- stale market data;
- резкое изменение комиссии;
- недоступна биржа;
- превышен лимит API.

## 7. Данные и модели

### 7.1. Основные сущности

```text
Exchange
  id
  name
  status

Market
  exchange_id
  base_asset
  quote_asset
  symbol
  min_order_size
  price_precision
  quantity_precision
  maker_fee
  taker_fee

OrderBookSnapshot
  exchange_id
  symbol
  bids[]
  asks[]
  received_at
  exchange_timestamp

Opportunity
  id
  strategy_type
  buy_exchange
  sell_exchange
  symbol
  expected_volume
  expected_profit
  expected_profit_pct
  detected_at
  status

RiskDecision
  opportunity_id
  approved
  reasons[]
  checked_at

TradeExecution
  id
  opportunity_id
  status
  started_at
  finished_at

Order
  execution_id
  exchange_id
  external_order_id
  side
  price
  quantity
  filled_quantity
  average_price
  status

BalanceSnapshot
  exchange_id
  asset
  free
  locked
  total
  captured_at
```

### 7.2. Хранилище

Рекомендуемая схема:

- PostgreSQL для сделок, балансов, аудита, конфигурации;
- Redis для быстрых актуальных стаканов, локов и transient state;
- TimescaleDB или ClickHouse опционально для большого объема исторических рыночных данных.

Для MVP достаточно PostgreSQL + Redis.

## 8. Алгоритм поиска межбиржевого арбитража

1. Получить актуальные стаканы для пары на всех биржах.
2. Для каждой пары бирж сравнить ask покупателя и bid продавца.
3. Рассчитать доступный объем по стакану, а не только по первой цене.
4. Учесть комиссии обеих бирж.
5. Учесть ожидаемое проскальзывание.
6. Проверить локальные балансы.
7. Добавить safety buffer.
8. Передать возможность в Risk Engine.
9. Если риск одобрен, передать в Execution Engine.

Псевдокод на Go:

```go
func (s *Scanner) OnOrderBookUpdate(ctx context.Context, update marketdata.BookUpdate) {
    s.books.Apply(update)

    books := s.books.Snapshot(update.Symbol)
    for _, buyExchange := range s.exchanges {
        for _, sellExchange := range s.exchanges {
            if buyExchange == sellExchange {
                continue
            }

            quote, ok := s.estimator.EstimateCrossExchange(strategy.EstimateInput{
                Symbol:       update.Symbol,
                BuyBook:      books[buyExchange],
                SellBook:     books[sellExchange],
                Fees:         s.fees,
                Balances:     s.portfolio.Snapshot(),
                MaxNotional:  s.config.Risk.MaxTradeUSD,
                SafetyBuffer: s.config.Strategy.SafetyBufferPct,
            })
            if !ok || quote.NetProfitPct < s.config.Risk.MinProfitPct {
                continue
            }

            opportunity := strategy.NewOpportunity(quote)
            decision := s.risk.Check(ctx, opportunity)
            if !decision.Approved {
                s.audit.RejectedAsync(opportunity, decision)
                continue
            }

            s.execution.Submit(ctx, opportunity)
        }
    }
}
```

## 9. Исполнение сделки

### 9.1. Предпочтительный порядок

Для Go-first версии предпочтительно конкурентное размещение двух сторон сделки:

1. Сделать финальный refresh стаканов.
2. Повторно пересчитать прибыль.
3. Создать client order id для обеих сторон.
4. Создать `context.Context` с коротким timeout.
5. Запустить две goroutines для buy и sell order.
6. Дождаться результата обеих сторон или timeout.
7. Выполнить reconcile по `client_order_id`.
8. Зафиксировать PnL или запустить recovery flow.

Последовательное размещение допустимо только в paper trading или в live mode с очень консервативными лимитами, потому что оно увеличивает риск исчезновения спреда между первой и второй стороной сделки.

### 9.2. Идемпотентность

Каждая операция должна иметь уникальный `client_order_id`.

Если сеть оборвалась после отправки ордера:

- нельзя сразу повторять ордер;
- нужно сначала запросить статус по `client_order_id`;
- если ордер найден, продолжить обработку;
- если не найден, решить по retry policy.

## 10. Risk Management

Минимальный набор лимитов:

```yaml
trading:
  enabled: false
  mode: paper

risk:
  max_trade_usd: 100
  max_daily_volume_usd: 1000
  max_daily_loss_usd: 50
  min_profit_pct: 0.25
  min_profit_usd: 1
  max_slippage_pct: 0.10
  max_orderbook_age_ms: 500
  max_exchange_exposure_usd: 1000
  max_asset_exposure_usd: 1000
  stop_after_consecutive_failures: 3
```

Live trading должен быть выключен по умолчанию.

## 11. Безопасность

API-ключи:

- хранить только в secret manager или зашифрованном хранилище;
- не хранить в Git;
- включать только нужные permissions;
- для MVP отключить withdrawal permissions;
- использовать IP allowlist, если биржа поддерживает;
- регулярно ротировать ключи.

Операционная безопасность:

- отдельный аккаунт/субаккаунт под бота;
- ограниченный капитал на старте;
- обязательный kill switch;
- ручное подтверждение для увеличения лимитов;
- журнал всех действий администратора.

## 12. Технологический стек

Рекомендуемый стек:

- Go 1.22+ как основной язык runtime;
- goroutines + channels для параллельной обработки WebSocket, сигналов и исполнения;
- `context.Context` для deadline, cancellation и request tracing;
- `net/http` с настроенным transport для REST API бирж;
- `gorilla/websocket` или `nhooyr.io/websocket` для WebSocket;
- `shopspring/decimal` или фиксированная decimal-арифметика для денег и объемов;
- `pgx` для PostgreSQL;
- `go-redis` для Redis;
- `chi`, `gin` или стандартный `net/http` для Admin API;
- `zap` или `zerolog` для структурированных логов;
- `prometheus/client_golang` для метрик;
- `golang-migrate` или `goose` для миграций;
- PostgreSQL;
- Redis;
- Prometheus + Grafana для метрик;
- Docker Compose для локального запуска.

Python можно оставить только для исследовательских notebooks и offline-аналитики, но не для hot path.

### 12.1. Почему Go

Go подходит для этой задачи, потому что:

- простой и быстрый сетевой runtime;
- дешевые goroutines для большого числа WebSocket-соединений;
- строгая типизация для торговой логики и risk checks;
- один статический бинарь для деплоя;
- низкий overhead по сравнению с Python-сервисом;
- хорошая стандартная библиотека для HTTP, TLS, profiling и observability.

### 12.2. Требования к latency

Целевые метрики для MVP:

- обработка WebSocket update внутри процесса: `< 5 ms p95`;
- пересчет opportunity после обновления стакана: `< 2 ms p95` для одной пары;
- pre-trade risk check без обращения к базе: `< 1 ms p95`;
- отправка ордера в REST API биржи: зависит от сети, измеряется отдельно по биржам;
- stale threshold для стакана: `300-500 ms` на старте, затем корректируется по статистике.

Важно: Go уменьшает задержку внутри приложения, но не отменяет внешние задержки бирж, интернета, rate limits и matching engine.

## 13. Модули MVP

```text
cmd/
  arb-bot/
    main.go

internal/
  api/
    Admin API

  exchange/
    REST and WebSocket connectors

  marketdata/
    in-memory order books
    collectors
    normalizers

  strategy/
    opportunity detection
    profitability estimation

  risk/
    pre-trade checks
    limits

  execution/
    order placement
    recovery

  portfolio/
    balances
    exposure

  storage/
    PostgreSQL repositories
    async writers

  observability/
    logs
    metrics
    traces
```

Для первого прототипа это один процесс. Внутренние модули разделяются пакетами, а не сетевыми сервисами. Это уменьшает latency, снижает сложность деплоя и упрощает консистентность in-memory состояния.

## 14. Предлагаемая структура репозитория

```text
crypto-arbitrage-bot/
  go.mod
  go.sum
  README.md
  .env.example
  Makefile
  docker-compose.yml

  cmd/
    arb-bot/
      main.go

  configs/
    local.yaml
    paper.yaml

  internal/
    app/
      app.go
      config.go

    api/
      server.go
      handlers.go
      middleware.go

    exchange/
      connector.go
      types.go
      rate_limiter.go
      binance/
        rest.go
        websocket.go
        mapper.go
      kraken/
        rest.go
        websocket.go
        mapper.go

    marketdata/
      orderbook.go
      snapshot.go
      collector.go
      normalizer.go

    strategy/
      cross_exchange.go
      triangular.go
      estimator.go
      opportunity.go

    risk/
      engine.go
      limits.go
      decision.go

    execution/
      engine.go
      orders.go
      recovery.go
      idempotency.go

    portfolio/
      balances.go
      exposure.go
      rebalance.go

    storage/
      postgres.go
      repositories.go
      migrations/

    observability/
      metrics.go
      logging.go
      alerts.go

  pkg/
    decimal/
      money.go

  tests/
    integration/
    simulation/
```

Примечание: `internal/` закрывает прикладную логику от внешнего импорта. `pkg/` стоит использовать только для кода, который действительно может быть переиспользован отдельно от приложения.

## 15. API панели управления

Минимальные endpoints:

```text
GET  /health
GET  /exchanges
GET  /markets
GET  /balances
GET  /opportunities
GET  /executions
GET  /pnl

POST /trading/enable
POST /trading/disable
POST /risk/limits
POST /rebalance/manual-request
```

Кнопка отключения торговли должна быть простой и надежной:

```text
POST /trading/disable
```

Она должна:

- остановить новые сделки;
- отменить открытые ордера, если это безопасно;
- оставить систему мониторинга включенной.

## 16. Тестирование

### 16.1. Unit tests

Покрыть:

- расчет прибыли;
- учет комиссий;
- расчет объема по стакану;
- risk checks;
- округление цены и количества;
- idempotency logic;
- обработку частичных исполнений.

Для Go:

- unit tests размещать рядом с пакетами в файлах `*_test.go`;
- table-driven tests использовать для расчетов прибыли, округлений и risk checks;
- race detector запускать для конкурентных компонентов;
- benchmark tests добавить для order book update и opportunity estimation.

Минимальные команды проверки:

```text
go test ./...
go test -race ./...
go test -bench=. ./internal/marketdata ./internal/strategy
```

### 16.2. Integration tests

Покрыть:

- подключение к sandbox/testnet биржам;
- размещение и отмену ордеров;
- получение балансов;
- reconnect WebSocket;
- rate limit handling.

### 16.3. Simulation tests

Использовать исторические стаканы:

- прогонять стратегию на данных;
- сравнивать ожидаемый и симулированный PnL;
- измерять sensitivity к задержке;
- проверять, как меняется результат при росте комиссии и проскальзывания.

Симулятор должен уметь воспроизводить поток событий детерминированно. Это важно для проверки регрессий: один и тот же набор snapshots должен давать одинаковый набор сигналов и одинаковый paper PnL.

## 17. Этапы реализации

### Этап 1. Research и аналитический сбор данных

Результат:

- подключены 2 биржи;
- собираются стаканы;
- сохраняются snapshots;
- считается сырой и чистый спред;
- есть отчет по возможностям.

Live trading отсутствует.

### Этап 2. Paper trading

Результат:

- симуляция сделок;
- учет комиссий;
- учет стакана;
- виртуальный PnL;
- журнал решений;
- первичные risk limits.

### Этап 3. Минимальный live trading

Результат:

- live trading только для одной пары, например BTC/USDT;
- маленький лимит на сделку;
- withdrawal permissions отключены;
- IOC/FOK limit-ордера;
- kill switch;
- алерты.

### Этап 4. Расширение

Результат:

- больше пар;
- больше бирж;
- автоматическая ребалансировка;
- треугольный арбитраж;
- улучшенный execution engine;
- dashboard.

## 18. Главные риски

### 18.1. Спред не означает прибыль

Видимый спред может исчезнуть после:

- комиссий;
- проскальзывания;
- задержки;
- частичного исполнения;
- недостаточного объема.

### 18.2. Риск одной исполненной стороны

Если покупка прошла, а продажа нет, бот получает открытую позицию. Это главный риск live trading.

### 18.3. Биржевые ограничения

Биржа может:

- изменить правила торговой пары;
- остановить ввод/вывод;
- отклонить ордер;
- ограничить API;
- вернуть устаревшие или неполные данные.

### 18.4. Операционный риск

Ошибки в конфигурации могут быть опаснее ошибок стратегии. Поэтому live trading должен быть выключен по умолчанию, а лимиты должны быть жесткими.

## 19. Критерии готовности MVP

MVP можно считать готовым, если:

- система стабильно собирает стаканы минимум с двух бирж;
- расчет прибыли учитывает комиссии и глубину стакана;
- paper trading работает минимум 7 дней без критических ошибок;
- все сделки и решения пишутся в аудит;
- есть kill switch;
- есть лимиты риска;
- live trading можно включить только явным действием;
- минимальные тесты покрывают расчет прибыли, risk checks и execution recovery.

## 20. Рекомендуемый первый MVP

Самый прагматичный первый вариант:

- 2 биржи;
- 1 торговая пара: BTC/USDT или ETH/USDT;
- только spot;
- только аналитика и paper trading первые 1-2 недели;
- live trading с лимитом `max_trade_usd = 25-100`;
- без автоматических выводов между биржами;
- без маржи;
- без leverage;
- без market-ордеров.

## 21. Пример конфигурации

```yaml
app:
  environment: local
  log_level: info
  metrics_addr: ":9090"
  admin_addr: ":8080"

trading:
  mode: paper
  enabled: false
  symbols:
    - BTC/USDT
    - ETH/USDT

exchanges:
  binance:
    enabled: true
    api_key_env: BINANCE_API_KEY
    api_secret_env: BINANCE_API_SECRET
  kraken:
    enabled: true
    api_key_env: KRAKEN_API_KEY
    api_secret_env: KRAKEN_API_SECRET

risk:
  max_trade_usd: 100
  max_daily_volume_usd: 1000
  max_daily_loss_usd: 50
  min_profit_pct: 0.25
  min_profit_usd: 1
  max_slippage_pct: 0.10
  max_orderbook_age_ms: 500
  stop_after_consecutive_failures: 3

execution:
  order_type: limit_ioc
  retry_count: 1
  order_timeout_ms: 800
  cancel_unfilled_after_ms: 500
  manual_intervention_on_partial_fill: true

latency:
  max_ws_update_lag_ms: 300
  max_orderbook_age_ms: 500
  opportunity_recheck_before_execution: true

storage:
  postgres_dsn_env: DATABASE_URL
  redis_dsn_env: REDIS_URL
```

## 22. Открытые вопросы

- Какие биржи доступны с учетом региона, KYC и API permissions?
- Какие комиссии будут у конкретного аккаунта?
- Нужен ли только spot или также derivatives?
- Какой стартовый капитал планируется держать на каждой бирже?
- Нужна ли web-панель или достаточно CLI и Grafana?
- Будет ли бот работать на VPS, локальной машине или в облаке?
- Нужна ли интеграция с Telegram/Slack для алертов?

## 23. Практическая рекомендация

Начинать нужно не с автоматической торговли, а с анализатора:

1. Подключить биржи.
2. Собирать стаканы.
3. Считать реальные net opportunities.
4. Несколько дней измерять, сколько возможностей остается после комиссий и проскальзывания.
5. Включить paper trading.
6. Только после стабильной статистики включать live trading на минимальных лимитах.

Арбитражный бот должен быть спроектирован как risk-management система с торговой функцией, а не как скрипт, который покупает дешевле и продает дороже.
