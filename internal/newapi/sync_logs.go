package newapi

import (
	"context"
	"fmt"
	"time"

	"monitor/internal/audit"
	"monitor/internal/storage"
)

type LogLister interface {
	ListLogs(context.Context, string) (*LogList, error)
}

type LogCursorStore interface {
	GetLogSyncCursor(string) (*storage.LogSyncCursor, error)
	UpsertLogSyncCursor(*storage.LogSyncCursor) error
}

type LogStore interface {
	LogCursorStore
	SaveNewAPILogs([]storage.NewAPILog) error
}

func SyncLogs(ctx context.Context, client LogLister, store LogCursorStore) (audit.ProductionMetrics, error) {
	if client == nil {
		return audit.ProductionMetrics{}, fmt.Errorf("log lister is nil")
	}
	if store == nil {
		return audit.ProductionMetrics{}, fmt.Errorf("log cursor store is nil")
	}
	cur, err := store.GetLogSyncCursor("default")
	if err != nil {
		return audit.ProductionMetrics{}, err
	}
	cursor := ""
	if cur != nil && cur.LastID > 0 {
		cursor = fmt.Sprintf("%d", cur.LastID)
	}
	res, err := client.ListLogs(ctx, cursor)
	if err != nil {
		return audit.ProductionMetrics{}, err
	}
	specs := make([]audit.LogSpec, 0, len(res.Items))
	logs := make([]storage.NewAPILog, 0, len(res.Items))
	for _, log := range res.Items {
		specs = append(specs, audit.LogSpec{
			ID:               log.ID,
			CreatedAt:        log.CreatedAt,
			Type:             log.Type,
			ModelName:        log.ModelName,
			Quota:            log.Quota,
			PromptTokens:     log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			UseTime:          log.UseTime,
			IsStream:         log.IsStream,
			Channel:          log.Channel,
			Group:            log.Group,
			Other:            log.Other,
		})
		logs = append(logs, storage.NewAPILog{
			ID:                int64(log.ID),
			CreatedAt:         log.CreatedAt,
			Type:              log.Type,
			Content:           log.Content,
			ModelName:         log.ModelName,
			Quota:             log.Quota,
			PromptTokens:      log.PromptTokens,
			CompletionTokens:  log.CompletionTokens,
			UseTime:           log.UseTime,
			IsStream:          log.IsStream,
			ChannelID:         int64(log.Channel),
			Group:             log.Group,
			RequestID:         log.RequestID,
			UpstreamRequestID: log.UpstreamRequestID,
			Other:             log.Other,
		})
	}
	metrics := audit.AggregateProductionMetrics(specs, time.Now())
	if logStore, ok := store.(interface {
		SaveNewAPILogs([]storage.NewAPILog) error
	}); ok && len(logs) > 0 {
		if err := logStore.SaveNewAPILogs(logs); err != nil {
			return metrics, err
		}
	}
	if len(res.Items) > 0 {
		last := res.Items[len(res.Items)-1]
		if err := store.UpsertLogSyncCursor(&storage.LogSyncCursor{
			Name:      "default",
			LastID:    int64(last.ID),
			LastTime:  last.CreatedAt,
			UpdatedAt: time.Now().Unix(),
		}); err != nil {
			return metrics, err
		}
	}
	return metrics, nil
}
