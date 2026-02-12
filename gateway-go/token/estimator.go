package token

import (
	"sync"
	"sync/atomic"
)

type Stats struct {
	mu             sync.RWMutex
	byCredential   map[string]*CounterPair
	byModel        map[string]*CounterPair
	globalInput    atomic.Int64
	globalOutput   atomic.Int64
	globalRequests atomic.Int64
}

type CounterPair struct {
	Input    atomic.Int64
	Output   atomic.Int64
	Requests atomic.Int64
}

func NewStats() *Stats {
	return &Stats{
		byCredential: make(map[string]*CounterPair),
		byModel:      make(map[string]*CounterPair),
	}
}

func (s *Stats) Record(credID, model string, inputTokens, outputTokens int) {
	s.globalInput.Add(int64(inputTokens))
	s.globalOutput.Add(int64(outputTokens))
	s.globalRequests.Add(1)

	s.getOrCreate(credID, model).Input.Add(int64(inputTokens))
	s.getOrCreate(credID, model).Output.Add(int64(outputTokens))
	s.getOrCreate(credID, model).Requests.Add(1)

	s.getModelCounter(model).Input.Add(int64(inputTokens))
	s.getModelCounter(model).Output.Add(int64(outputTokens))
	s.getModelCounter(model).Requests.Add(1)
}

func (s *Stats) getOrCreate(credID, _ string) *CounterPair {
	s.mu.RLock()
	if cp, ok := s.byCredential[credID]; ok {
		s.mu.RUnlock()
		return cp
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if cp, ok := s.byCredential[credID]; ok {
		return cp
	}
	cp := &CounterPair{}
	s.byCredential[credID] = cp
	return cp
}

func (s *Stats) getModelCounter(model string) *CounterPair {
	s.mu.RLock()
	if cp, ok := s.byModel[model]; ok {
		s.mu.RUnlock()
		return cp
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if cp, ok := s.byModel[model]; ok {
		return cp
	}
	cp := &CounterPair{}
	s.byModel[model] = cp
	return cp
}

// EstimateInputTokens estimates input token count: chars/4 + images*300
func EstimateInputTokens(text string, imageCount int) int {
	tokens := len(text) / 4
	tokens += imageCount * 300
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func (s *Stats) GetSummary() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	credStats := make(map[string]map[string]int64)
	for k, v := range s.byCredential {
		credStats[k] = map[string]int64{
			"input_tokens":  v.Input.Load(),
			"output_tokens": v.Output.Load(),
			"requests":      v.Requests.Load(),
		}
	}

	modelStats := make(map[string]map[string]int64)
	for k, v := range s.byModel {
		modelStats[k] = map[string]int64{
			"input_tokens":  v.Input.Load(),
			"output_tokens": v.Output.Load(),
			"requests":      v.Requests.Load(),
		}
	}

	return map[string]any{
		"global": map[string]int64{
			"input_tokens":  s.globalInput.Load(),
			"output_tokens": s.globalOutput.Load(),
			"requests":      s.globalRequests.Load(),
		},
		"by_credential": credStats,
		"by_model":      modelStats,
	}
}
