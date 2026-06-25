package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitor/internal/audit"
	"monitor/internal/newapi"
	"monitor/internal/storage"
)

type auditReadStore interface {
	ListAuditTargets() ([]storage.AuditTarget, error)
	GetLatestChannelSnapshotStats() (*storage.ChannelSnapshotStats, error)
	ListLatestChannelSnapshots() ([]storage.ChannelSnapshot, error)
	GetLogSyncCursor(string) (*storage.LogSyncCursor, error)
	ListNewAPILogsSince(int64) ([]storage.NewAPILog, error)
	GetDiagnosticRun(string) (*storage.DiagnosticRun, error)
	ListDiagnosticSteps(string) ([]*storage.DiagnosticStep, error)
	GetDiagnosticScore(string) (*storage.DiagnosticScore, error)
	SaveNewAPILogs([]storage.NewAPILog) error
	ReplaceAuditTargets([]storage.AuditTarget) error
	SaveChannelSnapshot(*storage.ChannelSnapshot) error
	UpsertLogSyncCursor(*storage.LogSyncCursor) error
}

type auditDiagnosticStore interface {
	auditReadStore
	SaveDiagnosticRun(*storage.DiagnosticRun) error
	SaveDiagnosticStep(*storage.DiagnosticStep) error
	SaveDiagnosticScore(*storage.DiagnosticScore) error
}

func (h *Handler) auditStore() (auditReadStore, bool) {
	store, ok := h.storage.(auditReadStore)
	return store, ok
}

func (h *Handler) newAPIClient() *newapi.Client {
	h.cfgMu.RLock()
	cfg := h.config
	h.cfgMu.RUnlock()
	if cfg == nil {
		return nil
	}
	return newapi.NewClient(cfg.NewAPI.BaseURL, cfg.NewAPI.AccessToken, cfg.NewAPI.UserID)
}

func (h *Handler) GetAuditSyncStatus(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	targetSummary := auditTargetSummary{Total: len(targets)}
	for _, target := range targets {
		if target.Enabled {
			targetSummary.Enabled++
		}
	}
	channelStats, err := store.GetLatestChannelSnapshotStats()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	cursor, err := store.GetLogSyncCursor("default")
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	h.cfgMu.RLock()
	baseURL := ""
	if h.config != nil {
		baseURL = h.config.NewAPI.BaseURL
	}
	h.cfgMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditSyncStatusResponse{
			NewAPIBaseURL: baseURL,
			Targets:       targetSummary,
			Channels:      channelStats,
			LogCursor:     cursor,
		},
	})
}

func (h *Handler) PostAuditSyncChannels(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	client := h.newAPIClient()
	if client == nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "new-api 配置缺失")
		return
	}
	items, err := newapi.SyncChannels(c.Request.Context(), client, store)
	if err != nil {
		apiError(c, http.StatusBadGateway, ErrCodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"synced": len(items)}})
}

func (h *Handler) PostAuditSyncLogs(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	client := h.newAPIClient()
	if client == nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "new-api 配置缺失")
		return
	}
	metrics, err := newapi.SyncLogs(c.Request.Context(), client, store)
	if err != nil {
		apiError(c, http.StatusBadGateway, ErrCodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": metrics})
}

func (h *Handler) PostAuditDiagnosticSubmit(c *gin.Context) {
	store, ok := h.storage.(auditDiagnosticStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持诊断提交")
		return
	}
	var req auditDiagnosticSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "请求参数错误")
		return
	}
	target := audit.DiagnosticTarget{
		Provider:     strings.TrimSpace(req.Provider),
		Service:      strings.TrimSpace(req.Service),
		Channel:      strings.TrimSpace(req.Channel),
		Model:        strings.TrimSpace(req.Model),
		RequestModel: strings.TrimSpace(req.RequestModel),
	}
	if target.RequestModel == "" {
		target.RequestModel = target.Model
	}
	h.cfgMu.RLock()
	if h.config != nil {
		target.BaseURL = h.config.NewAPI.BaseURL
		target.AccessToken = h.config.NewAPI.AccessToken
		target.UserID = h.config.NewAPI.UserID
	}
	h.cfgMu.RUnlock()
	if strings.TrimSpace(target.BaseURL) == "" {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "new-api 配置缺失")
		return
	}
	runner := audit.NewDiagnosticRunner(nil)
	run, err := runner.Run(c.Request.Context(), target, store)
	if err != nil {
		apiError(c, http.StatusBadGateway, ErrCodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": auditDiagnosticSubmitResponse{RunID: run.RunID}})
}

func (h *Handler) GetAuditTargets(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": targets})
}

func (h *Handler) GetAuditChannels(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	snapshots, err := store.ListLatestChannelSnapshots()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	for i := range snapshots {
		rawMap := map[string]any{}
		if len(snapshots[i].Raw) > 0 {
			_ = json.Unmarshal(snapshots[i].Raw, &rawMap)
		}
		snapshots[i].ChannelType, snapshots[i].ChannelTypeLabel = resolveAuditChannelType(
			snapshots[i].Service,
			snapshots[i].Channel,
			rawMap,
		)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshots,
		"meta": gin.H{
			"count": len(snapshots),
		},
	})
}

func (h *Handler) GetAuditRanking(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	window := normalizeAuditWindow(c.DefaultQuery("window", "24h"))
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	logs, err := store.ListNewAPILogsSince(time.Now().Add(-windowDuration(window)).Unix())
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	rows := buildAuditRankingRows(targets, logs, window, time.Now())
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *Handler) GetAuditDiagnostic(c *gin.Context) {
	h.writeAuditDiagnostic(c, false)
}

func (h *Handler) GetAuditCompare(c *gin.Context) {
	h.writeAuditDiagnostic(c, true)
}

func (h *Handler) writeAuditDiagnostic(c *gin.Context, includeCompare bool) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	runID := strings.TrimSpace(c.Param("run_id"))
	if runID == "" {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "run_id 不能为空")
		return
	}
	run, err := store.GetDiagnosticRun(runID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	if run == nil {
		apiError(c, http.StatusNotFound, ErrCodeNotFound, "诊断任务不存在")
		return
	}
	steps, err := store.ListDiagnosticSteps(runID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	score, err := store.GetDiagnosticScore(runID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	resp := auditDiagnosticResponse{Run: run, Score: score}
	respSteps := make([]any, 0, len(steps))
	for _, step := range steps {
		respSteps = append(respSteps, step)
	}
	resp.Steps = respSteps
	if includeCompare {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"run": run, "score": score, "compare": respSteps}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func normalizeAuditWindow(raw string) string {
	switch strings.TrimSpace(raw) {
	case "7d":
		return "7d"
	case "30d":
		return "30d"
	default:
		return "24h"
	}
}

func windowDuration(window string) time.Duration {
	switch window {
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func buildAuditRankingRows(targets []storage.AuditTarget, logs []storage.NewAPILog, window string, now time.Time) []auditRankingRow {
	logMap := make(map[string][]audit.LogSpec)
	for _, log := range logs {
		key := strconv.FormatInt(log.ChannelID, 10) + "|" + strings.TrimSpace(log.ModelName)
		logMap[key] = append(logMap[key], audit.LogSpec{
			ID:               int(log.ID),
			CreatedAt:        log.CreatedAt,
			Type:             log.Type,
			ModelName:        log.ModelName,
			Quota:            log.Quota,
			PromptTokens:     log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			UseTime:          log.UseTime,
			IsStream:         log.IsStream,
			Channel:          int(log.ChannelID),
			Group:            log.Group,
			Other:            log.Other,
		})
	}

	rows := make([]auditRankingRow, 0, len(targets))
	for _, target := range targets {
		channelID := extractChannelID(target.Channel)
		if channelID == "" {
			continue
		}
		modelKey := strings.TrimSpace(target.RequestModel)
		if modelKey == "" {
			modelKey = strings.TrimSpace(target.Model)
		}
		metrics := audit.AggregateProductionMetrics(logMap[channelID+"|"+modelKey], now).Windows[window]
		successRate := 0.0
		if metrics.Total > 0 {
			successRate = float64(metrics.Success) / float64(metrics.Total) * 100
		}
		rows = append(rows, auditRankingRow{
			Provider:        target.Provider,
			Service:         target.Service,
			Channel:         target.Channel,
			Model:           target.Model,
			RequestModel:    target.RequestModel,
			Enabled:         target.Enabled,
			Window:          window,
			Total:           metrics.Total,
			Success:         metrics.Success,
			Error:           metrics.Error,
			Timeout:         metrics.Timeout,
			SuccessRate:     successRate,
			P95:             metrics.P95,
			P99:             metrics.P99,
			TokensPerSecond: metrics.TokensPerSec,
			AvgFRT:          metrics.AvgFRT,
			Score:           successRate,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		if rows[i].Service != rows[j].Service {
			return rows[i].Service < rows[j].Service
		}
		if rows[i].Channel != rows[j].Channel {
			return rows[i].Channel < rows[j].Channel
		}
		return rows[i].Model < rows[j].Model
	})
	return rows
}

func extractChannelID(channel string) string {
	head := strings.TrimSpace(channel)
	if idx := strings.Index(head, ":"); idx >= 0 {
		head = head[:idx]
	}
	head = strings.TrimSpace(head)
	if _, err := strconv.ParseInt(head, 10, 64); err != nil {
		return ""
	}
	return head
}
