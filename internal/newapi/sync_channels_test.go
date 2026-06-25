package newapi

import (
	"context"
	"testing"

	"monitor/internal/storage"
)

type fakeLister struct {
	list *ChannelList
	err  error
}

func (f fakeLister) ListChannels(context.Context) (*ChannelList, error) {
	return f.list, f.err
}

func TestSyncChannelsWritesTargets(t *testing.T) {
	store, err := storage.NewSQLiteStorage(t.TempDir() + "/audit.db")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	mapJSON := `{"gpt-4o":"gpt-4o"}`
	got, err := SyncChannels(context.Background(), fakeLister{list: &ChannelList{
		Items: []Channel{{
			ID:           11,
			Type:         1,
			Status:       1,
			Name:         "demo",
			Models:       "gpt-4o",
			Group:        "default",
			ModelMapping: &mapJSON,
			Other:        []byte(`{"provider":"Anthropic","service":"cc"}`),
		}},
	}}, store)
	if err != nil {
		t.Fatalf("SyncChannels: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got len = %d, want 1", len(got))
	}

	targets, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].Provider != "Anthropic" || targets[0].Model != "gpt-4o" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}
