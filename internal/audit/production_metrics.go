package audit

import (
	"encoding/json"
	"sort"
	"time"
)

type LogSpec struct {
	ID               int
	CreatedAt        int64
	Type             int
	ModelName        string
	Quota            int
	PromptTokens     int
	CompletionTokens int
	UseTime          int
	IsStream         bool
	Channel          int
	Group            string
	Other            json.RawMessage
}

type MetricWindow struct {
	Window         string   `json:"window"`
	Total          int      `json:"total"`
	Success        int      `json:"success"`
	Error          int      `json:"error"`
	Timeout        int      `json:"timeout"`
	P95            int      `json:"p95"`
	P99            int      `json:"p99"`
	TokensPerSec   float64  `json:"tokens_per_sec"`
	AvgFRT         float64  `json:"avg_frt"`
	CacheTokens    int      `json:"cache_tokens"`
	CacheCreate    int      `json:"cache_create_tokens"`
	ModelMapped    int      `json:"model_mapped"`
	UpstreamModels []string `json:"upstream_models,omitempty"`
}

type ProductionMetrics struct {
	Windows map[string]MetricWindow `json:"windows"`
}

func AggregateProductionMetrics(logs []LogSpec, now time.Time) ProductionMetrics {
	windows := map[string]time.Duration{
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}
	out := ProductionMetrics{Windows: make(map[string]MetricWindow, len(windows))}
	for name, dur := range windows {
		cutoff := now.Add(-dur).Unix()
		out.Windows[name] = aggregateWindow(name, filterLogs(logs, cutoff))
	}
	return out
}

func filterLogs(logs []LogSpec, since int64) []LogSpec {
	out := make([]LogSpec, 0, len(logs))
	for _, log := range logs {
		if log.CreatedAt >= since {
			out = append(out, log)
		}
	}
	return out
}

func aggregateWindow(name string, logs []LogSpec) MetricWindow {
	if len(logs) == 0 {
		return MetricWindow{Window: name}
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].UseTime != logs[j].UseTime {
			return logs[i].UseTime < logs[j].UseTime
		}
		return logs[i].ID < logs[j].ID
	})
	var (
		sumUseTime   int
		sumComplete  int
		sumFRT       int
		frtCount     int
		cacheTokens  int
		cacheCreate  int
		modelMapped  int
		upstreamSeen = make(map[string]struct{})
		total        = len(logs)
		success      int
		errorCount   int
		timeout      int
	)
	useTimes := make([]int, 0, len(logs))
	for _, log := range logs {
		useTimes = append(useTimes, log.UseTime)
		sumUseTime += log.UseTime
		sumComplete += log.CompletionTokens
		if log.Type == 2 {
			success++
		}
		if log.Type == 5 {
			errorCount++
		}
		if isTimeoutLog(log) {
			timeout++
		}
		other := parseOther(log.Other)
		if v, ok := other["frt"].(float64); ok {
			sumFRT += int(v)
			frtCount++
		}
		if v, ok := other["cache_tokens"].(float64); ok {
			cacheTokens += int(v)
		}
		if v, ok := other["cache_creation_tokens"].(float64); ok {
			cacheCreate += int(v)
		}
		if v, ok := other["is_model_mapped"].(bool); ok && v {
			modelMapped++
		}
		if v, ok := other["upstream_model_name"].(string); ok && v != "" {
			upstreamSeen[v] = struct{}{}
		}
	}
	upstreamModels := make([]string, 0, len(upstreamSeen))
	for v := range upstreamSeen {
		upstreamModels = append(upstreamModels, v)
	}
	sort.Strings(upstreamModels)
	return MetricWindow{
		Window:         name,
		Total:          total,
		Success:        success,
		Error:          errorCount,
		Timeout:        timeout,
		P95:            percentile(useTimes, 95),
		P99:            percentile(useTimes, 99),
		TokensPerSec:   ratio(sumComplete, sumUseTime),
		AvgFRT:         average(sumFRT, frtCount),
		CacheTokens:    cacheTokens,
		CacheCreate:    cacheCreate,
		ModelMapped:    modelMapped,
		UpstreamModels: upstreamModels,
	}
}

func parseOther(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func isTimeoutLog(log LogSpec) bool {
	other := parseOther(log.Other)
	if v, ok := other["error_type"].(string); ok && v == "timeout" {
		return true
	}
	if v, ok := other["status_code"].(float64); ok && int(v) == 504 {
		return true
	}
	if v, ok := other["error_code"].(string); ok && v == "timeout" {
		return true
	}
	return false
}

func percentile(values []int, p int) int {
	if len(values) == 0 {
		return 0
	}
	sort.Ints(values)
	if p <= 0 {
		return values[0]
	}
	if p >= 100 {
		return values[len(values)-1]
	}
	idx := int(float64(len(values)-1) * float64(p) / 100.0)
	return values[idx]
}

func ratio(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func average(sum, count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(sum) / float64(count)
}
