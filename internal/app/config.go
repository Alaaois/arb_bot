package app

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App       AppConfig
	Trading   TradingConfig
	Exchanges map[string]ExchangeConfig
	Fees      map[string]FeeConfig
	Risk      RiskConfig
	Strategy  StrategyConfig
	Execution ExecutionConfig
	Storage   StorageConfig
}

type AppConfig struct {
	Environment string
	LogLevel    string
	AdminAddr   string
}

type TradingConfig struct {
	Enabled bool
	Mode    string
	Symbols []string
}

type ExchangeConfig struct {
	Enabled bool
}

type FeeConfig struct {
	MakerPct float64
	TakerPct float64
}

type RiskConfig struct {
	MaxTradeUSD                  float64
	MaxDailyVolumeUSD            float64
	MaxDailyLossUSD              float64
	MinProfitPct                 float64
	MinProfitUSD                 float64
	MaxSlippagePct               float64
	MaxOrderBookAge              time.Duration
	StopAfterConsecutiveFailures int
}

type StrategyConfig struct {
	MaxDepthLevels        int
	MinExpectedCarryPct   float64
	MinExpectedCarryUSD   float64
	BasisRiskPct          float64
	DefaultFundingRatePct float64
	FundingInterval       time.Duration
	MinTimeToFunding      time.Duration
	MaxHoldTime           time.Duration
}

type ExecutionConfig struct {
	OrderType                       string
	RetryCount                      int
	OrderTimeout                    time.Duration
	CancelUnfilledAfter             time.Duration
	ManualInterventionOnPartialFill bool
}

type StorageConfig struct {
	RedisEnabled          bool
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	RedisHistoryLimit     int64
	RedisOperationTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		App: AppConfig{
			Environment: "local",
			LogLevel:    "info",
			AdminAddr:   ":8080",
		},
		Trading: TradingConfig{
			Enabled: false,
			Mode:    "paper",
			Symbols: []string{"BTC/USDT", "ETH/USDT"},
		},
		Exchanges: map[string]ExchangeConfig{
			"bybit":  {Enabled: true},
			"okx":    {Enabled: true},
			"htx":    {Enabled: true},
			"kucoin": {Enabled: true},
		},
		Fees: map[string]FeeConfig{
			"bybit":  {MakerPct: 0.02, TakerPct: 0.055},
			"okx":    {MakerPct: 0.02, TakerPct: 0.05},
			"htx":    {MakerPct: 0.02, TakerPct: 0.06},
			"kucoin": {MakerPct: 0.02, TakerPct: 0.06},
		},
		Risk: RiskConfig{
			MaxTradeUSD:                  100,
			MaxDailyVolumeUSD:            1000,
			MaxDailyLossUSD:              50,
			MinProfitPct:                 0.25,
			MinProfitUSD:                 1,
			MaxSlippagePct:               0.10,
			MaxOrderBookAge:              500 * time.Millisecond,
			StopAfterConsecutiveFailures: 3,
		},
		Strategy: StrategyConfig{
			MaxDepthLevels:        5,
			MinExpectedCarryPct:   0.05,
			MinExpectedCarryUSD:   0.25,
			BasisRiskPct:          0.03,
			DefaultFundingRatePct: 0.01,
			FundingInterval:       8 * time.Hour,
			MinTimeToFunding:      15 * time.Minute,
			MaxHoldTime:           9 * time.Hour,
		},
		Execution: ExecutionConfig{
			OrderType:                       "limit_ioc",
			RetryCount:                      1,
			OrderTimeout:                    800 * time.Millisecond,
			CancelUnfilledAfter:             500 * time.Millisecond,
			ManualInterventionOnPartialFill: true,
		},
		Storage: StorageConfig{
			RedisEnabled:          true,
			RedisAddr:             "redis:6379",
			RedisHistoryLimit:     10000,
			RedisOperationTimeout: 500 * time.Millisecond,
		},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	var section string
	var exchange string
	var listKey string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") {
			if listKey == "trading.symbols" {
				cfg.Trading.Symbols = appendIfMissing(cfg.Trading.Symbols, strings.Trim(strings.TrimPrefix(line, "- "), `"`))
			}
			continue
		}

		if !strings.Contains(line, ":") {
			continue
		}

		key, value, _ := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)

		if value == "" {
			listKey = ""
			switch key {
			case "app", "trading", "exchanges", "fees", "risk", "strategy", "execution", "storage":
				section = key
				exchange = ""
			case "symbols":
				listKey = section + ".symbols"
				if section == "trading" {
					cfg.Trading.Symbols = nil
				}
			default:
				if section == "exchanges" {
					exchange = key
					if cfg.Exchanges == nil {
						cfg.Exchanges = make(map[string]ExchangeConfig)
					}
					cfg.Exchanges[exchange] = ExchangeConfig{}
				}
				if section == "fees" {
					exchange = key
					if cfg.Fees == nil {
						cfg.Fees = make(map[string]FeeConfig)
					}
					if _, ok := cfg.Fees[exchange]; !ok {
						cfg.Fees[exchange] = FeeConfig{}
					}
				}
			}
			continue
		}

		if err := applyConfigValue(&cfg, section, exchange, key, value); err != nil {
			return cfg, fmt.Errorf("%s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func applyConfigValue(cfg *Config, section, exchange, key, value string) error {
	switch section {
	case "app":
		switch key {
		case "environment":
			cfg.App.Environment = value
		case "log_level":
			cfg.App.LogLevel = value
		case "admin_addr":
			cfg.App.AdminAddr = value
		}
	case "trading":
		switch key {
		case "enabled":
			cfg.Trading.Enabled = parseBool(value)
		case "mode":
			cfg.Trading.Mode = value
		}
	case "exchanges":
		if exchange != "" && key == "enabled" {
			cfg.Exchanges[exchange] = ExchangeConfig{Enabled: parseBool(value)}
		}
	case "fees":
		return applyFeeValue(cfg, exchange, key, value)
	case "risk":
		return applyRiskValue(&cfg.Risk, key, value)
	case "strategy":
		return applyStrategyValue(&cfg.Strategy, key, value)
	case "execution":
		return applyExecutionValue(&cfg.Execution, key, value)
	case "storage":
		return applyStorageValue(&cfg.Storage, key, value)
	}
	return nil
}

func applyFeeValue(cfg *Config, exchange, key, value string) error {
	if exchange == "" {
		return nil
	}
	fee := cfg.Fees[exchange]
	switch key {
	case "maker_pct":
		if err := parseFloatInto(value, &fee.MakerPct); err != nil {
			return err
		}
	case "taker_pct":
		if err := parseFloatInto(value, &fee.TakerPct); err != nil {
			return err
		}
	}
	cfg.Fees[exchange] = fee
	return nil
}

func applyRiskValue(cfg *RiskConfig, key, value string) error {
	switch key {
	case "max_trade_usd":
		return parseFloatInto(value, &cfg.MaxTradeUSD)
	case "max_daily_volume_usd":
		return parseFloatInto(value, &cfg.MaxDailyVolumeUSD)
	case "max_daily_loss_usd":
		return parseFloatInto(value, &cfg.MaxDailyLossUSD)
	case "min_profit_pct":
		return parseFloatInto(value, &cfg.MinProfitPct)
	case "min_profit_usd":
		return parseFloatInto(value, &cfg.MinProfitUSD)
	case "max_slippage_pct":
		return parseFloatInto(value, &cfg.MaxSlippagePct)
	case "max_orderbook_age_ms":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.MaxOrderBookAge = time.Duration(ms) * time.Millisecond
	case "stop_after_consecutive_failures":
		return parseIntInto(value, &cfg.StopAfterConsecutiveFailures)
	case "max_open_positions":
		return nil
	}
	return nil
}

func applyStrategyValue(cfg *StrategyConfig, key, value string) error {
	switch key {
	case "max_depth_levels":
		return parseIntInto(value, &cfg.MaxDepthLevels)
	case "min_expected_carry_pct":
		return parseFloatInto(value, &cfg.MinExpectedCarryPct)
	case "min_expected_carry_usd":
		return parseFloatInto(value, &cfg.MinExpectedCarryUSD)
	case "basis_risk_pct":
		return parseFloatInto(value, &cfg.BasisRiskPct)
	case "default_funding_rate_pct":
		return parseFloatInto(value, &cfg.DefaultFundingRatePct)
	case "funding_interval_minutes":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.FundingInterval = time.Duration(ms) * time.Minute
	case "min_time_to_funding_minutes":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.MinTimeToFunding = time.Duration(ms) * time.Minute
	case "max_hold_time_minutes":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.MaxHoldTime = time.Duration(ms) * time.Minute
	}
	return nil
}

func applyExecutionValue(cfg *ExecutionConfig, key, value string) error {
	switch key {
	case "order_type":
		cfg.OrderType = value
	case "retry_count":
		return parseIntInto(value, &cfg.RetryCount)
	case "order_timeout_ms":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.OrderTimeout = time.Duration(ms) * time.Millisecond
	case "cancel_unfilled_after_ms":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.CancelUnfilledAfter = time.Duration(ms) * time.Millisecond
	case "manual_intervention_on_partial_fill":
		cfg.ManualInterventionOnPartialFill = parseBool(value)
	}
	return nil
}

func applyStorageValue(cfg *StorageConfig, key, value string) error {
	switch key {
	case "redis_enabled":
		cfg.RedisEnabled = parseBool(value)
	case "redis_addr":
		cfg.RedisAddr = value
	case "redis_password":
		cfg.RedisPassword = value
	case "redis_db":
		return parseIntInto(value, &cfg.RedisDB)
	case "redis_history_limit":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		cfg.RedisHistoryLimit = parsed
	case "redis_operation_timeout_ms":
		ms, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.RedisOperationTimeout = time.Duration(ms) * time.Millisecond
	}
	return nil
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(c.App.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseFloatInto(value string, target *float64) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}

func parseIntInto(value string, target *int) error {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}

func parseBool(value string) bool {
	return value == "true" || value == "yes" || value == "1"
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
