package newapi

import (
	"context"
	"testing"
	"time"

	"monitor/internal/storage"
)

type fakeLogLister struct {
	list *LogList
	err  error
}

func (f fakeLogLister) ListLogs(context.Context, string) (*LogList, error) {
	return f.list, f.err
}

func TestSyncLogsUpdatesCursor(t *testing.T) {
	store, err := storage.NewSQLiteStorage(t.TempDir() + "/logs.db")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().Unix()

	metrics, err := SyncLogs(context.Background(), fakeLogLister{list: &LogList{
		Items: []Log{
			{ID: 10, CreatedAt: now - 60, Type: 2, CompletionTokens: 20, UseTime: 4, Other: []byte(`{"frt":11}`)},
			{ID: 11, CreatedAt: now - 30, Type: 5, UseTime: 5, Other: []byte(`{"error_type":"timeout"}`)},
		},
	}}, store)
	if err != nil {
		t.Fatalf("SyncLogs: %v", err)
	}
	if metrics.Windows["24h"].Total != 2 {
		t.Fatalf("unexpected metrics: %+v", metrics.Windows["24h"])
	}

	cur, err := store.GetLogSyncCursor("default")
	if err != nil {
		t.Fatalf("GetLogSyncCursor: %v", err)
	}
	if cur == nil || cur.LastID != 11 || cur.LastTime != now-30 {
		t.Fatalf("unexpected cursor: %+v", cur)
	}

	logs, err := store.ListNewAPILogsSince(now - 120)
	if err != nil {
		t.Fatalf("ListNewAPILogsSince: %v", err)
	}
	if len(logs) != 2 || logs[0].ID != 11 || logs[1].ID != 10 {
		t.Fatalf("unexpected logs: %+v", logs)
	}
}
