package audit

import (
	"testing"
	"time"
)

func TestAggregateProductionMetrics(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	logs := []LogSpec{
		{ID: 1, CreatedAt: now.Add(-1 * time.Hour).Unix(), Type: 2, CompletionTokens: 100, UseTime: 10, Other: []byte(`{"frt":120,"cache_tokens":2,"is_model_mapped":true,"upstream_model_name":"gpt-4o"}`)},
		{ID: 2, CreatedAt: now.Add(-2 * time.Hour).Unix(), Type: 5, CompletionTokens: 0, UseTime: 20, Other: []byte(`{"error_type":"timeout","status_code":504}`)},
		{ID: 3, CreatedAt: now.Add(-8 * 24 * time.Hour).Unix(), Type: 2, CompletionTokens: 50, UseTime: 5},
	}

	got := AggregateProductionMetrics(logs, now)
	w := got.Windows["24h"]
	if w.Total != 2 || w.Success != 1 || w.Error != 1 || w.Timeout != 1 {
		t.Fatalf("unexpected 24h window: %+v", w)
	}
	if w.TokensPerSec <= 0 || w.AvgFRT != 120 {
		t.Fatalf("unexpected metrics: %+v", w)
	}
	if got.Windows["7d"].Total != 2 || got.Windows["30d"].Total != 3 {
		t.Fatalf("unexpected window totals: %+v", got.Windows)
	}
}
