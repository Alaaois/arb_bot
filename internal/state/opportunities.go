package state

import (
	"sync"

	"crypto-arbitrage-bot/internal/execution"
	"crypto-arbitrage-bot/internal/risk"
	"crypto-arbitrage-bot/internal/strategy"
)

type OpportunityRecord struct {
	Opportunity strategy.Opportunity `json:"opportunity"`
	Decision    risk.Decision        `json:"decision"`
	Execution   execution.Result     `json:"execution"`
}

type OpportunityLog struct {
	mu      sync.RWMutex
	limit   int
	records []OpportunityRecord
}

func NewOpportunityLog(limit int) *OpportunityLog {
	return &OpportunityLog{limit: limit}
}

func (l *OpportunityLog) Add(record OpportunityRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append([]OpportunityRecord{record}, l.records...)
	if len(l.records) > l.limit {
		l.records = l.records[:l.limit]
	}
}

func (l *OpportunityLog) List() []OpportunityRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]OpportunityRecord(nil), l.records...)
}
