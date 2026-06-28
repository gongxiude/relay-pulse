package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitor/internal/audit"
	"monitor/internal/config"
	"monitor/internal/newapi"
	"monitor/internal/storage"
)

type auditReadStore interface {
	ListAuditTargets() ([]storage.AuditTarget, error)
	GetLatestChannelSnapshotStats() (*storage.ChannelSnapshotStats, error)
	ListLatestChannelSnapshots() ([]storage.ChannelSnapshot, error)
	GetLogSyncCursor(string) (*storage.LogSyncCursor, error)
	ListNewAPILogsSince(int64) ([]storage.NewAPILog, error)
	ListDiagnosticRuns(storage.DiagnosticRunFilter) ([]*storage.DiagnosticRun, error)
	CountDiagnosticRuns(string) (int, error)
	CountDiagnosticRunsFiltered(storage.DiagnosticRunFilter) (int, error)
	GetDiagnosticDimensionSummary() (*storage.DiagnosticDimensionSummary, error)
	GetDiagnosticRun(string) (*storage.DiagnosticRun, error)
	ListDiagnosticSteps(string) ([]*storage.DiagnosticStep, error)
	GetDiagnosticScore(string) (*storage.DiagnosticScore, error)
	GetDiagnosticRunGroup(string) (*storage.DiagnosticRunGroup, error)
	GetLatestDiagnosticBaselineRun(service, modelFamily, methodologyVersion, excludeRunID string) (*storage.DiagnosticBaselineRun, error)
	ListDiagnosticDimensions(string) ([]*storage.DiagnosticDimension, error)
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
	SaveDiagnosticRunGroup(*storage.DiagnosticRunGroup) error
	SaveDiagnosticDimension(*storage.DiagnosticDimension) error
	SaveDiagnosticBaselineRun(*storage.DiagnosticBaselineRun) error
}

type auditTemplateProbeStore interface {
	auditReadStore
	SaveRecord(*storage.ProbeRecord) error
}

type auditModelStatusStore interface {
	auditReadStore
	GetLatest(provider, service, channel, model string) (*storage.ProbeRecord, error)
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
	runtime := auditProbeRuntimeResponse{}
	if h.config != nil {
		baseURL = h.config.NewAPI.BaseURL
		runtime = buildAuditProbeRuntime(h.config)
	}
	h.cfgMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditSyncStatusResponse{
			NewAPIBaseURL: baseURL,
			Targets:       targetSummary,
			Channels:      channelStats,
			LogCursor:     cursor,
			ProbeRuntime:  runtime,
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
	appCfg := h.config
	creds, credErr := resolveAuditProbeCredentials(appCfg)
	h.cfgMu.RUnlock()
	if credErr != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, credErr.Error())
		return
	}
	target.BaseURL = creds.BaseURL
	target.AccessToken = creds.AccessToken
	target.UserID = creds.UserID
	if err := attachAuditDiagnosticTemplate(appCfg, h.configDir(), &target); err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, err.Error())
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

func (h *Handler) GetAuditModelStatus(c *gin.Context) {
	store, ok := h.storage.(auditModelStatusStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计模型状态接口")
		return
	}
	window := normalizeAuditWindow(c.DefaultQuery("window", "24h"))
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	targets = filterAuditTargets(targets,
		strings.TrimSpace(c.Query("provider")),
		strings.TrimSpace(c.Query("service")),
		strings.TrimSpace(c.Query("channel")),
		strings.TrimSpace(c.Query("model")),
	)
	logs, err := store.ListNewAPILogsSince(time.Now().Add(-windowDuration(window)).Unix())
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	items, err := buildAuditModelStatusItems(store, targets, logs, window, time.Now())
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditModelStatusResponse{
			Items: items,
			Meta: auditModelStatusMetaResponse{
				Window: window,
				Count:  len(items),
			},
		},
	})
}

func (h *Handler) GetAuditDiagnostic(c *gin.Context) {
	h.writeAuditDiagnostic(c, false)
}

func (h *Handler) GetAuditCompare(c *gin.Context) {
	h.writeAuditDiagnostic(c, true)
}

func (h *Handler) GetAuditMethodology(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	spec := audit.CurrentMethodologySpec()
	runs, err := store.ListDiagnosticRuns(storage.DiagnosticRunFilter{
		Limit: 1000,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	doneRuns := 0
	dimensionRuns := 0
	dimensionRowCount := 0
	failedAuthRuns := 0
	failedRequestRuns := 0
	for _, run := range runs {
		score, err := store.GetDiagnosticScore(run.RunID)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		usable, reason, _, err := classifyDiagnosticRun(store, run, score)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		if !usable {
			switch reason {
			case "failed_auth":
				failedAuthRuns++
			case "failed_request":
				failedRequestRuns++
			}
			continue
		}
		doneRuns++
		dimensions, err := store.ListDiagnosticDimensions(run.RunID)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		if len(dimensions) > 0 {
			dimensionRuns++
			dimensionRowCount += len(dimensions)
		}
	}
	h.cfgMu.RLock()
	runtime := auditProbeRuntimeResponse{}
	if h.config != nil {
		runtime = buildAuditProbeRuntime(h.config)
	}
	h.cfgMu.RUnlock()
	dimensions := make([]auditMethodologyDimensionResponse, 0, len(spec.Dimensions))
	for _, dimension := range spec.Dimensions {
		dimensions = append(dimensions, auditMethodologyDimensionResponse{
			Key:         dimension.Key,
			Weight:      dimension.Weight,
			Group:       dimension.Group,
			Description: dimension.Description,
			Implemented: dimension.Implemented,
			Active:      dimension.Active,
			Phase:       dimension.Phase,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditMethodologyResponse{
			Summary: auditMethodologySummaryResponse{
				Version:           spec.Version,
				WeightsHash:       spec.WeightsHash,
				TotalDimensions:   len(spec.Dimensions),
				TotalWeight:       spec.TotalWeight,
				ImplementedCount:  spec.ImplementedCount,
				ImplementedWeight: spec.ImplementedWeight,
				ActiveCount:       spec.ActiveCount,
				ActiveWeight:      spec.ActiveWeight,
			},
			Coverage: auditMethodologyCoverageResponse{
				DoneRuns:          doneRuns,
				DimensionRuns:     dimensionRuns,
				DimensionRowCount: dimensionRowCount,
				FailedAuthRuns:    failedAuthRuns,
				FailedRequestRuns: failedRequestRuns,
				FilteredRuns:      failedAuthRuns + failedRequestRuns,
			},
			Runtime:    runtime,
			Dimensions: dimensions,
		},
	})
}

func (h *Handler) GetAuditDiagnosticLatest(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	limit := 1
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "limit 参数错误")
			return
		}
		if value > 20 {
			value = 20
		}
		limit = value
	}
	includeFiltered := parseBoolQuery(c.Query("include_filtered"))
	filter := storage.DiagnosticRunFilter{
		Provider: strings.TrimSpace(c.Query("provider")),
		Service:  strings.TrimSpace(c.Query("service")),
		Channel:  strings.TrimSpace(c.Query("channel")),
		Model:    strings.TrimSpace(c.Query("model")),
		Limit:    limit * 10,
	}
	if !includeFiltered {
		filter.Status = "done"
	}
	runs, err := store.ListDiagnosticRuns(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	items := make([]auditDiagnosticLatestItemResponse, 0, len(runs))
	for _, run := range runs {
		item, err := buildAuditDiagnosticListItem(store, run)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		if !includeFiltered && !item.Usable {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditDiagnosticLatestResponse{
			Items: items,
			Meta: auditDiagnosticLatestMetaResponse{
				Limit: limit,
				Count: len(items),
			},
		},
	})
}

func buildAuditDiagnosticListItem(store auditReadStore, run *storage.DiagnosticRun) (auditDiagnosticLatestItemResponse, error) {
	score, err := store.GetDiagnosticScore(run.RunID)
	if err != nil {
		return auditDiagnosticLatestItemResponse{}, err
	}
	usable, filterReason, classifiedSteps, err := classifyDiagnosticRun(store, run, score)
	if err != nil {
		return auditDiagnosticLatestItemResponse{}, err
	}
	item := auditDiagnosticLatestItemResponse{
		Run:          buildAuditDiagnosticRun(run),
		Score:        buildAuditDiagnosticScore(run, score),
		Usable:       usable,
		FilterReason: filterReason,
		CompareURL:   "/api/audit/compare/" + run.RunID,
	}
	if !usable && strings.TrimSpace(item.Run.RunStatus) == "" {
		item.Run.RunStatus = filterReason
	}
	if !usable && strings.TrimSpace(item.Run.RunStatusReason) == "" {
		item.Run.RunStatusReason = summarizeDiagnosticFailureReason(classifiedSteps, filterReason)
	}
	return item, nil
}

func parseAuditHistoryLimit(c *gin.Context) (int, bool) {
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "limit 参数错误")
			return 0, false
		}
		if value > 200 {
			value = 200
		}
		limit = value
	}
	return limit, true
}

func parseAuditHistoryOffset(c *gin.Context) (int, bool) {
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "offset 参数错误")
			return 0, false
		}
		offset = value
	}
	return offset, true
}

func (h *Handler) GetAuditDiagnosticHistory(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	limit, ok := parseAuditHistoryLimit(c)
	if !ok {
		return
	}
	offset, ok := parseAuditHistoryOffset(c)
	if !ok {
		return
	}

	filter := storage.DiagnosticRunFilter{
		Provider: strings.TrimSpace(c.Query("provider")),
		Service:  strings.TrimSpace(c.Query("service")),
		Channel:  strings.TrimSpace(c.Query("channel")),
		Model:    strings.TrimSpace(c.Query("model")),
		Status:   strings.TrimSpace(c.Query("status")),
		Limit:    limit,
		Offset:   offset,
	}

	total, err := store.CountDiagnosticRunsFiltered(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	runs, err := store.ListDiagnosticRuns(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	items := make([]auditDiagnosticLatestItemResponse, 0, len(runs))
	for _, run := range runs {
		item, err := buildAuditDiagnosticListItem(store, run)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		items = append(items, item)
	}

	var nextOffset *int
	if offset+len(items) < total {
		next := offset + len(items)
		nextOffset = &next
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditDiagnosticHistoryResponse{
			Items: items,
			Meta: auditDiagnosticHistoryMetaResponse{
				Limit:      limit,
				Offset:     offset,
				Count:      len(items),
				Total:      total,
				Provider:   filter.Provider,
				Service:    filter.Service,
				Channel:    filter.Channel,
				Model:      filter.Model,
				Status:     filter.Status,
				NextOffset: nextOffset,
			},
		},
	})
}

func classifyDiagnosticRun(store auditReadStore, run *storage.DiagnosticRun, score *storage.DiagnosticScore) (bool, string, []*storage.DiagnosticStep, error) {
	if run == nil {
		return false, "not_done", nil, nil
	}
	switch strings.TrimSpace(run.Status) {
	case "failed_auth":
		return false, "failed_auth", nil, nil
	case "failed_request":
		return false, "failed_request", nil, nil
	case "", "done":
		// 继续走 output / step 级回溯，兼容旧样本。
	default:
		return false, "not_done", nil, nil
	}
	outputMap := decodeAuditJSONMap(run.Output)
	switch strings.TrimSpace(mapStringValue(outputMap, "run_status")) {
	case "failed_auth":
		return false, "failed_auth", nil, nil
	case "failed_request":
		return false, "failed_request", nil, nil
	}
	needsStepInspection := false
	if skippedTags := decodeAuditJSONStringList(anyJSONValue(outputMap, "tags")); containsString(skippedTags, "request_error") {
		needsStepInspection = true
	}
	if score != nil && containsString(decodeAuditJSONStringList(score.Tags), "request_error") {
		needsStepInspection = true
	}
	if needsStepInspection {
		steps, err := store.ListDiagnosticSteps(run.RunID)
		if err != nil {
			return false, "", nil, err
		}
		if allDiagnosticStepsMatchError(steps, "401") {
			return false, "failed_auth", steps, nil
		}
		return false, "failed_request", steps, nil
	}
	return true, "usable", nil, nil
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func summarizeDiagnosticFailureReason(steps []*storage.DiagnosticStep, fallback string) string {
	if len(steps) == 0 {
		return fallback
	}
	errors := make([]string, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		message := strings.TrimSpace(step.ErrorMessage)
		if message == "" {
			continue
		}
		stepName := inferDiagnosticStepName(step.StepIndex, step.Prompt)
		if stepName != "" {
			errors = append(errors, fmt.Sprintf("%s: %s", stepName, message))
		} else {
			errors = append(errors, message)
		}
		if len(errors) >= 2 {
			break
		}
	}
	if len(errors) == 0 {
		return fallback
	}
	return strings.Join(errors, "; ")
}

func allDiagnosticStepsMatchError(steps []*storage.DiagnosticStep, fragment string) bool {
	if len(steps) == 0 {
		return false
	}
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return false
	}
	for _, step := range steps {
		if step == nil || !strings.Contains(strings.TrimSpace(step.ErrorMessage), fragment) {
			return false
		}
	}
	return true
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func buildAuditProbeRuntime(cfg *config.AppConfig) auditProbeRuntimeResponse {
	creds, err := resolveAuditProbeCredentials(cfg)
	if err != nil {
		mode := "missing"
		authConfigured := false
		userConfigured := false
		if cfg != nil {
			mode = normalizedAuditCredentialMode(cfg)
			authConfigured = strings.TrimSpace(cfg.NewAPI.ProbeAccessToken) != "" || strings.TrimSpace(cfg.NewAPI.AccessToken) != ""
			userConfigured = strings.TrimSpace(cfg.NewAPI.ProbeUserID) != "" || strings.TrimSpace(cfg.NewAPI.UserID) != ""
		}
		return auditProbeRuntimeResponse{
			ProbeCredentialMode: mode,
			ProbeAuthConfigured: authConfigured,
			ProbeUserConfigured: userConfigured,
			ProbeReady:          false,
			Warning:             err.Error(),
		}
	}
	return auditProbeRuntimeResponse{
		ProbeCredentialMode: creds.Mode,
		ProbeAuthConfigured: strings.TrimSpace(creds.AccessToken) != "",
		ProbeUserConfigured: strings.TrimSpace(creds.UserID) != "",
		ProbeReady:          true,
		Warning:             creds.Warning,
	}
}

type auditProbeCredentials struct {
	BaseURL     string
	AccessToken string
	UserID      string
	Mode        string
	Warning     string
}

func normalizedAuditCredentialMode(cfg *config.AppConfig) string {
	if cfg == nil {
		return "missing"
	}
	mode := strings.TrimSpace(cfg.Audit.Diagnostics.CredentialMode)
	if mode == "" {
		return config.ProbeCredentialModeProbeFallback
	}
	return mode
}

func resolveAuditProbeCredentials(cfg *config.AppConfig) (auditProbeCredentials, error) {
	if cfg == nil {
		return auditProbeCredentials{Mode: "missing"}, fmt.Errorf("new-api 配置缺失，无法执行主动诊断")
	}
	if !cfg.Audit.Diagnostics.IsEnabled() {
		return auditProbeCredentials{Mode: normalizedAuditCredentialMode(cfg)}, fmt.Errorf("主动诊断已通过 audit.diagnostics.enabled 关闭")
	}
	baseURL := strings.TrimSpace(cfg.NewAPI.BaseURL)
	if baseURL == "" {
		return auditProbeCredentials{Mode: normalizedAuditCredentialMode(cfg)}, fmt.Errorf("NEWAPI_BASE_URL 未配置，无法执行主动诊断")
	}
	probeToken := strings.TrimSpace(cfg.NewAPI.ProbeAccessToken)
	probeUser := strings.TrimSpace(cfg.NewAPI.ProbeUserID)
	syncToken := strings.TrimSpace(cfg.NewAPI.AccessToken)
	syncUser := strings.TrimSpace(cfg.NewAPI.UserID)
	mode := normalizedAuditCredentialMode(cfg)

	switch mode {
	case config.ProbeCredentialModeProbeOnly:
		if probeToken == "" || probeUser == "" {
			return auditProbeCredentials{BaseURL: baseURL, Mode: mode}, fmt.Errorf("audit.diagnostics.credential_mode=probe_only，但 NEWAPI_PROBE_ACCESS_TOKEN 或 NEWAPI_PROBE_USER_ID 未配置完整")
		}
		return auditProbeCredentials{BaseURL: baseURL, AccessToken: probeToken, UserID: probeUser, Mode: mode}, nil
	case config.ProbeCredentialModeNewAPIOnly:
		if syncToken == "" || syncUser == "" {
			return auditProbeCredentials{BaseURL: baseURL, Mode: mode}, fmt.Errorf("audit.diagnostics.credential_mode=newapi_only，但 NEWAPI_ACCESS_TOKEN 或 NEWAPI_USER_ID 未配置完整")
		}
		return auditProbeCredentials{BaseURL: baseURL, AccessToken: syncToken, UserID: syncUser, Mode: mode}, nil
	case config.ProbeCredentialModeProbeFallback:
		if probeToken != "" && probeUser != "" {
			return auditProbeCredentials{BaseURL: baseURL, AccessToken: probeToken, UserID: probeUser, Mode: mode}, nil
		}
		if syncToken != "" && syncUser != "" {
			warning := "当前未配置完整独立主动探针凭证，诊断会按 probe_fallback 回退使用同步凭证；模板决定请求路径和请求体，probe token 只决定是否有权限发送该模板请求。"
			return auditProbeCredentials{BaseURL: baseURL, AccessToken: syncToken, UserID: syncUser, Mode: mode, Warning: warning}, nil
		}
		return auditProbeCredentials{BaseURL: baseURL, Mode: mode}, fmt.Errorf("主动探针凭证未配置完整，当前只能同步渠道与日志，无法产出有效诊断样本")
	default:
		return auditProbeCredentials{BaseURL: baseURL, Mode: mode}, fmt.Errorf("audit.diagnostics.credential_mode 无效: %s", mode)
	}
}

func attachAuditDiagnosticTemplate(cfg *config.AppConfig, configDir string, target *audit.DiagnosticTarget) error {
	if target == nil {
		return fmt.Errorf("diagnostic target is nil")
	}
	if strings.TrimSpace(configDir) == "" {
		return fmt.Errorf("配置目录不可用，无法加载 diagnostic template")
	}
	templateName, err := audit.ResolveTemplateProbeName(cfg, target.Service, "")
	if err != nil {
		return err
	}
	templatePath := filepath.Join(configDir, "templates", templateName+".json")
	tmpl, err := config.LoadProbeTemplate(templatePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(tmpl.RequestFamily) == "" || len(tmpl.OverridePaths) == 0 || strings.TrimSpace(tmpl.ResponseParser) == "" {
		return fmt.Errorf("模板 %s 未声明完整 diagnostic 契约", templateName)
	}
	target.TemplateName = templateName
	target.Template = tmpl
	return nil
}

func (h *Handler) PostAuditDiagnosticBackfill(c *gin.Context) {
	store, ok := h.storage.(auditDiagnosticStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持诊断提交")
		return
	}
	var req auditDiagnosticBackfillRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "请求参数错误")
			return
		}
	}
	if req.MaxTargets <= 0 {
		req.MaxTargets = 12
	}
	if req.MaxTargets > 50 {
		req.MaxTargets = 50
	}
	if req.MaxModelsPerChannel <= 0 {
		req.MaxModelsPerChannel = 1
	}
	if req.MaxModelsPerChannel > 3 {
		req.MaxModelsPerChannel = 3
	}
	if req.LookbackHours <= 0 {
		req.LookbackHours = 24
	}
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	logs, err := store.ListNewAPILogsSince(time.Now().Add(-time.Duration(req.LookbackHours) * time.Hour).Unix())
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	selected := selectAuditBackfillTargets(targets, logs, req.MaxTargets, req.MaxModelsPerChannel)
	runner := audit.NewDiagnosticRunner(nil)
	items := make([]auditDiagnosticBackfillItemResponse, 0, len(selected))
	started := 0
	failed := 0
	for _, target := range selected {
		runTarget := audit.DiagnosticTarget{
			Provider:     strings.TrimSpace(target.Provider),
			Service:      strings.TrimSpace(target.Service),
			Channel:      strings.TrimSpace(target.Channel),
			Model:        strings.TrimSpace(target.Model),
			RequestModel: strings.TrimSpace(target.RequestModel),
		}
		if runTarget.RequestModel == "" {
			runTarget.RequestModel = runTarget.Model
		}
		h.cfgMu.RLock()
		appCfg := h.config
		creds, credErr := resolveAuditProbeCredentials(appCfg)
		h.cfgMu.RUnlock()
		if credErr != nil {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, credErr.Error())
			return
		}
		runTarget.BaseURL = creds.BaseURL
		runTarget.AccessToken = creds.AccessToken
		runTarget.UserID = creds.UserID
		if err := attachAuditDiagnosticTemplate(appCfg, h.configDir(), &runTarget); err != nil {
			item := auditDiagnosticBackfillItemResponse{
				Provider: runTarget.Provider,
				Service:  runTarget.Service,
				Channel:  runTarget.Channel,
				Model:    runTarget.Model,
				Status:   "failed",
				Error:    err.Error(),
			}
			failed++
			items = append(items, item)
			continue
		}
		run, err := runner.Run(c.Request.Context(), runTarget, store)
		item := auditDiagnosticBackfillItemResponse{
			Provider: runTarget.Provider,
			Service:  runTarget.Service,
			Channel:  runTarget.Channel,
			Model:    runTarget.Model,
		}
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
		} else {
			item.Status = "started"
			item.RunID = run.RunID
			started++
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditDiagnosticBackfillResponse{
			Selected: len(selected),
			Started:  started,
			Failed:   failed,
			Items:    items,
		},
	})
}

func (h *Handler) PostAuditTemplateProbeBackfill(c *gin.Context) {
	store, ok := h.storage.(auditTemplateProbeStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持模板探测补洞")
		return
	}
	if h.inlineProber == nil {
		apiError(c, http.StatusServiceUnavailable, ErrCodeInternalError, "模板探测器未初始化")
		return
	}
	var req auditTemplateProbeBackfillRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "请求参数错误")
			return
		}
	}
	if req.MaxTargets <= 0 {
		req.MaxTargets = 12
	}
	if req.MaxTargets > 50 {
		req.MaxTargets = 50
	}
	configDir := h.configDir()
	if strings.TrimSpace(configDir) == "" {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, "配置目录不可用，无法加载 templates")
		return
	}
	h.cfgMu.RLock()
	appCfg := h.config
	creds, credErr := resolveAuditProbeCredentials(h.config)
	h.cfgMu.RUnlock()
	if credErr != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, credErr.Error())
		return
	}
	targets, err := store.ListAuditTargets()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	selected := selectAuditTemplateProbeTargets(targets, req.MaxTargets)
	items := make([]auditTemplateProbeBackfillItemResponse, 0, len(selected))
	probed := 0
	failed := 0
	for _, target := range selected {
		item := auditTemplateProbeBackfillItemResponse{
			Provider: strings.TrimSpace(target.Provider),
			Service:  strings.TrimSpace(target.Service),
			Channel:  strings.TrimSpace(target.Channel),
			Model:    strings.TrimSpace(target.Model),
		}
		templateName, err := audit.ResolveTemplateProbeName(appCfg, target.Service, req.TemplateName)
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
			items = append(items, item)
			continue
		}
		item.Template = templateName
		probeCfg, err := audit.BuildTemplateProbeConfig(appCfg, target, audit.TemplateProbeCredentials{
			BaseURL:     creds.BaseURL,
			AccessToken: creds.AccessToken,
			UserID:      creds.UserID,
		}, templateName, configDir)
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
			items = append(items, item)
			continue
		}
		result := h.inlineProber.ProbeConfig(c.Request.Context(), probeCfg)
		record, err := audit.ProbeRecordFromTemplateProbeResult(target, result, time.Now())
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
			items = append(items, item)
			continue
		}
		if err := store.SaveRecord(record); err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
			items = append(items, item)
			continue
		}
		item.Status = "probed"
		item.ProbeStatus = record.Status
		item.SubStatus = string(record.SubStatus)
		item.HTTPCode = record.HttpCode
		item.Latency = record.Latency
		probed++
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditTemplateProbeBackfillResponse{
			Selected: len(selected),
			Probed:   probed,
			Failed:   failed,
			Items:    items,
		},
	})
}

func selectAuditTemplateProbeTargets(targets []storage.AuditTarget, maxTargets int) []storage.AuditTarget {
	if maxTargets <= 0 {
		return nil
	}
	out := make([]storage.AuditTarget, 0, maxTargets)
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		if strings.TrimSpace(target.Provider) == "" ||
			strings.TrimSpace(target.Service) == "" ||
			strings.TrimSpace(target.Channel) == "" ||
			strings.TrimSpace(target.Model) == "" {
			continue
		}
		out = append(out, target)
		if len(out) >= maxTargets {
			break
		}
	}
	return out
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
	group, err := loadAuditDiagnosticGroup(store, run)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	baselineResp, err := loadAuditBaselineResponse(store, group)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	dimensions, err := store.ListDiagnosticDimensions(runID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	resp := buildAuditDiagnosticResponse(run, score, steps)
	if includeCompare {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": buildAuditCompareResponse(resp, baselineResp, group, dimensions)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func loadAuditDiagnosticGroup(store auditReadStore, run *storage.DiagnosticRun) (*storage.DiagnosticRunGroup, error) {
	if run == nil {
		return nil, nil
	}
	input := decodeAuditJSONMap(run.Input)
	groupID := mapStringValue(input, "group_id")
	if groupID == "" {
		return nil, nil
	}
	return store.GetDiagnosticRunGroup(groupID)
}

func loadAuditBaselineResponse(store auditReadStore, group *storage.DiagnosticRunGroup) (*auditDiagnosticResponse, error) {
	if group == nil || strings.TrimSpace(group.BaselineRunID) == "" {
		return nil, nil
	}
	run, err := store.GetDiagnosticRun(group.BaselineRunID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, nil
	}
	steps, err := store.ListDiagnosticSteps(group.BaselineRunID)
	if err != nil {
		return nil, err
	}
	score, err := store.GetDiagnosticScore(group.BaselineRunID)
	if err != nil {
		return nil, err
	}
	resp := buildAuditDiagnosticResponse(run, score, steps)
	return &resp, nil
}

func buildAuditDiagnosticResponse(run *storage.DiagnosticRun, score *storage.DiagnosticScore, steps []*storage.DiagnosticStep) auditDiagnosticResponse {
	resp := auditDiagnosticResponse{
		Run:   buildAuditDiagnosticRun(run),
		Score: buildAuditDiagnosticScore(run, score),
		Steps: make([]auditDiagnosticStepResponse, 0, len(steps)),
	}
	resp.Run = normalizeAuditDiagnosticRunResponse(resp.Run, steps, score)
	for _, step := range steps {
		resp.Steps = append(resp.Steps, buildAuditDiagnosticStep(step))
	}
	return resp
}

func buildAuditCompareResponse(candidate auditDiagnosticResponse, baseline *auditDiagnosticResponse, group *storage.DiagnosticRunGroup, dimensions []*storage.DiagnosticDimension) auditCompareResponse {
	steps := make([]auditCompareStepResponse, 0, len(candidate.Steps))
	baselineStepMap := make(map[int]auditDiagnosticStepResponse)
	if baseline != nil {
		for _, step := range baseline.Steps {
			baselineStepMap[step.StepIndex] = step
		}
	}
	for _, step := range candidate.Steps {
		var baselineStep *auditDiagnosticStepResponse
		if item, ok := baselineStepMap[step.StepIndex]; ok {
			copyItem := item
			baselineStep = &copyItem
		}
		steps = append(steps, auditCompareStepResponse{
			StepIndex: step.StepIndex,
			StepName:  step.StepName,
			Candidate: step,
			Baseline:  baselineStep,
		})
	}
	summary := auditCompareSummaryResponse{}
	if candidate.Score != nil {
		summary.OverallScore = candidate.Score.OverallScore
		summary.ActiveWeight = candidate.Score.ActiveWeight
		summary.SkippedDimensions = candidate.Score.SkippedDimensions
		summary.Tags = candidate.Score.Tags
	}
	dimensionPayload := make([]auditDiagnosticDimensionResponse, 0, len(dimensions))
	for _, dimension := range dimensions {
		dimensionPayload = append(dimensionPayload, buildAuditDiagnosticDimension(dimension))
	}
	groupResp := auditCompareGroupResponse{
		GroupID:            candidate.Run.GroupID,
		CandidateRunID:     candidate.Run.RunID,
		BaselineMode:       firstNonEmptyString(candidate.Run.BaselineMode, "single_run_only"),
		MethodologyVersion: firstNonEmptyString(candidate.Run.MethodologyVersion, "quick-probe-v1"),
		WeightsHash:        candidate.Run.WeightsHash,
	}
	if group != nil {
		groupResp.GroupID = group.GroupID
		groupResp.CandidateRunID = group.CandidateRunID
		groupResp.BaselineRunID = group.BaselineRunID
		groupResp.BaselineMode = firstNonEmptyString(group.BaselineMode, groupResp.BaselineMode)
		groupResp.MethodologyVersion = firstNonEmptyString(group.MethodologyVersion, groupResp.MethodologyVersion)
		groupResp.WeightsHash = firstNonEmptyString(group.WeightsHash, groupResp.WeightsHash)
	}
	return auditCompareResponse{
		Group:      groupResp,
		Candidate:  candidate,
		Baseline:   baseline,
		Dimensions: dimensionPayload,
		Steps:      steps,
		Summary:    summary,
	}
}

func buildAuditDiagnosticDimension(dimension *storage.DiagnosticDimension) auditDiagnosticDimensionResponse {
	if dimension == nil {
		return auditDiagnosticDimensionResponse{}
	}
	return auditDiagnosticDimensionResponse{
		RunID:           dimension.RunID,
		DimensionKey:    dimension.DimensionKey,
		Weight:          dimension.Weight,
		Score:           dimension.Score,
		NormalizedScore: dimension.NormalizedScore,
		Status:          dimension.Status,
		Reason:          dimension.Reason,
		Evidence:        decodeAuditJSONValue(dimension.Evidence),
	}
}

func buildAuditDiagnosticRun(run *storage.DiagnosticRun) auditDiagnosticRunResponse {
	if run == nil {
		return auditDiagnosticRunResponse{}
	}
	inputMap := decodeAuditJSONMap(run.Input)
	outputMap := decodeAuditJSONMap(run.Output)
	return auditDiagnosticRunResponse{
		RunID:              run.RunID,
		Provider:           run.Provider,
		Service:            run.Service,
		Channel:            run.Channel,
		Model:              run.Model,
		Status:             run.Status,
		RunStatus:          mapStringValue(outputMap, "run_status"),
		RunStatusReason:    mapStringValue(outputMap, "run_status_reason"),
		CreatedAt:          run.CreatedAt,
		UpdatedAt:          run.UpdatedAt,
		RequestModel:       mapStringValue(inputMap, "request_model"),
		BaseURL:            mapStringValue(inputMap, "base_url"),
		GroupID:            mapStringValue(inputMap, "group_id"),
		BaselineMode:       mapStringValue(outputMap, "baseline_mode"),
		MethodologyVersion: firstNonEmptyString(mapStringValue(outputMap, "methodology_version"), "quick-probe-v1"),
		WeightsHash:        mapStringValue(outputMap, "weights_hash"),
		CandidateType:      mapStringValue(outputMap, "candidate_type"),
		Input:              decodeAuditJSONValue(run.Input),
		Output:             decodeAuditJSONValue(run.Output),
	}
}

func normalizeAuditDiagnosticRunResponse(run auditDiagnosticRunResponse, steps []*storage.DiagnosticStep, score *storage.DiagnosticScore) auditDiagnosticRunResponse {
	if strings.TrimSpace(run.RunStatus) != "" {
		if strings.TrimSpace(run.RunStatusReason) == "" && strings.TrimSpace(run.RunStatus) != "done" {
			run.RunStatusReason = summarizeDiagnosticFailureReason(steps, strings.TrimSpace(run.RunStatus))
		}
		return run
	}
	if strings.TrimSpace(run.Status) != "done" {
		return run
	}
	hasRequestError := score != nil && containsString(decodeAuditJSONStringList(score.Tags), "request_error")
	if !hasRequestError {
		return run
	}
	if allDiagnosticStepsMatchError(steps, "401") {
		run.RunStatus = "failed_auth"
		run.RunStatusReason = summarizeDiagnosticFailureReason(steps, "failed_auth")
		return run
	}
	run.RunStatus = "failed_request"
	run.RunStatusReason = summarizeDiagnosticFailureReason(steps, "failed_request")
	return run
}

func buildAuditDiagnosticScore(run *storage.DiagnosticRun, score *storage.DiagnosticScore) *auditDiagnosticScoreResponse {
	if score == nil {
		return nil
	}
	outputMap := map[string]any{}
	if run != nil {
		outputMap = decodeAuditJSONMap(run.Output)
	}
	resp := &auditDiagnosticScoreResponse{
		RunID:              score.RunID,
		AuthenticityScore:  score.AuthenticityScore,
		ProtocolScore:      score.ProtocolScore,
		SSEScore:           score.SSEScore,
		OverallScore:       numberValueAsFloat(outputMap, "overall_score", averageFloat64(float64(score.AuthenticityScore), float64(score.ProtocolScore), float64(score.SSEScore))),
		ActiveWeight:       int(numberValue(outputMap, "active_weight")),
		MethodologyVersion: firstNonEmptyString(mapStringValue(outputMap, "methodology_version"), "quick-probe-v1"),
		WeightsHash:        mapStringValue(outputMap, "weights_hash"),
		Tags:               decodeAuditJSONStringList(score.Tags),
	}
	if resp.ActiveWeight == 0 {
		resp.ActiveWeight = 3
	}
	if skipped := decodeAuditJSONStringList(anyJSONValue(outputMap, "skipped_dimensions")); len(skipped) > 0 {
		resp.SkippedDimensions = skipped
	}
	return resp
}

func buildAuditDiagnosticStep(step *storage.DiagnosticStep) auditDiagnosticStepResponse {
	if step == nil {
		return auditDiagnosticStepResponse{}
	}
	meta := decodeAuditJSONMap(step.ExecutionMeta)
	stepName := mapStringValue(meta, "step_name")
	if stepName == "" {
		stepName = inferDiagnosticStepName(step.StepIndex, step.Prompt)
	}
	sessionMode := "same_session"
	if strings.Contains(strings.ToLower(step.ResultSummary), "fresh_session") {
		sessionMode = "fresh_session"
	}
	return auditDiagnosticStepResponse{
		ID:                  step.ID,
		RunID:               step.RunID,
		StepIndex:           step.StepIndex,
		StepName:            stepName,
		SessionMode:         sessionMode,
		Prompt:              step.Prompt,
		ResolvedPrompt:      step.ResolvedPrompt,
		ResponsePreview:     step.ResponsePreview,
		ResultSummary:       step.ResultSummary,
		Execution:           buildAuditDiagnosticExecution(meta),
		ChannelFingerprint:  step.ChannelFingerprint,
		ProviderFingerprint: step.ProviderFingerprint,
		ErrorMessage:        step.ErrorMessage,
		CreatedAt:           step.CreatedAt,
	}
}

func buildAuditDiagnosticExecution(meta map[string]any) auditDiagnosticExecutionResponse {
	headers := mapString(meta["response_headers"])
	usage := mapAny(meta["usage"])
	rawMeta := any(meta)
	if len(meta) == 0 {
		rawMeta = nil
	}
	return auditDiagnosticExecutionResponse{
		StepName:        mapStringValue(meta, "step_name"),
		StatusCode:      int(numberValue(meta, "status_code")),
		DurationMs:      firstPositive(numberValue(meta, "duration_ms"), numberValue(meta, "latency_ms")),
		LatencyMs:       numberValue(meta, "latency_ms"),
		HTTPTTFBMs:      numberValue(meta, "http_ttfb_ms"),
		FirstTextMs:     numberValue(meta, "first_text_ms"),
		TTFTMs:          numberValue(meta, "ttft_ms"),
		FinishReason:    mapStringValue(meta, "finish_reason"),
		RequestURL:      mapStringValue(meta, "request_url"),
		RequestBody:     anyJSONValue(meta, "request_body"),
		ResponseText:    mapStringValue(meta, "response_text"),
		ResponseHeaders: headers,
		Usage:           usage,
		StreamChunks:    stringList(meta["stream_chunks"]),
		RawMeta:         rawMeta,
	}
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

func buildAuditModelStatusItems(store auditModelStatusStore, targets []storage.AuditTarget, logs []storage.NewAPILog, window string, now time.Time) ([]auditModelStatusItemResponse, error) {
	logMap := make(map[string][]audit.LogSpec)
	logUpdatedAt := make(map[string]int64)
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
		if log.CreatedAt > logUpdatedAt[key] {
			logUpdatedAt[key] = log.CreatedAt
		}
	}
	items := make([]auditModelStatusItemResponse, 0, len(targets))
	for _, target := range targets {
		channelID := extractChannelID(target.Channel)
		modelKey := strings.TrimSpace(target.RequestModel)
		if modelKey == "" {
			modelKey = strings.TrimSpace(target.Model)
		}
		logKey := channelID + "|" + modelKey
		production := buildAuditProductionStatus(logMap[logKey], window, now, logUpdatedAt[logKey])
		templateProbe, err := buildAuditTemplateProbeStatus(store, target)
		if err != nil {
			return nil, err
		}
		quickProbe, err := buildAuditQuickProbeStatus(store, target)
		if err != nil {
			return nil, err
		}
		items = append(items, auditModelStatusItemResponse{
			Provider:      target.Provider,
			Service:       target.Service,
			Channel:       target.Channel,
			Model:         target.Model,
			RequestModel:  target.RequestModel,
			Enabled:       target.Enabled,
			Production:    production,
			TemplateProbe: templateProbe,
			QuickProbe:    quickProbe,
		})
	}
	return items, nil
}

func buildAuditProductionStatus(logs []audit.LogSpec, window string, now time.Time, updatedAt int64) auditProductionStatusResponse {
	metrics := audit.AggregateProductionMetrics(logs, now).Windows[window]
	status := "no_data"
	successRate := 0.0
	if metrics.Total > 0 {
		successRate = float64(metrics.Success) / float64(metrics.Total) * 100
		switch {
		case metrics.Error == 0:
			status = "ok"
		case metrics.Success > 0:
			status = "degraded"
		default:
			status = "error"
		}
	}
	return auditProductionStatusResponse{
		Source:      "production_logs",
		Status:      status,
		Total:       metrics.Total,
		Success:     metrics.Success,
		Error:       metrics.Error,
		Timeout:     metrics.Timeout,
		SuccessRate: successRate,
		P95:         metrics.P95,
		P99:         metrics.P99,
		UpdatedAt:   updatedAt,
	}
}

func buildAuditTemplateProbeStatus(store auditModelStatusStore, target storage.AuditTarget) (auditTemplateProbeStatusResponse, error) {
	record, err := store.GetLatest(target.Provider, target.Service, target.Channel, target.Model)
	if err != nil {
		return auditTemplateProbeStatusResponse{}, err
	}
	if record == nil {
		return auditTemplateProbeStatusResponse{Source: "template_probe", Status: "missing"}, nil
	}
	status := "unavailable"
	switch record.Status {
	case 1:
		status = "available"
	case 2:
		status = "degraded"
	}
	return auditTemplateProbeStatusResponse{
		Source:    "template_probe",
		Status:    status,
		SubStatus: string(record.SubStatus),
		HTTPCode:  record.HttpCode,
		Latency:   record.Latency,
		UpdatedAt: record.Timestamp,
		Error:     record.ErrorDetail,
	}, nil
}

func buildAuditQuickProbeStatus(store auditModelStatusStore, target storage.AuditTarget) (auditQuickProbeStatusResponse, error) {
	runs, err := store.ListDiagnosticRuns(storage.DiagnosticRunFilter{
		Provider: target.Provider,
		Service:  target.Service,
		Channel:  target.Channel,
		Model:    target.Model,
		Limit:    5,
	})
	if err != nil {
		return auditQuickProbeStatusResponse{}, err
	}
	if len(runs) == 0 {
		return auditQuickProbeStatusResponse{Source: "quick_probe", Status: "missing"}, nil
	}
	run := runs[0]
	score, err := store.GetDiagnosticScore(run.RunID)
	if err != nil {
		return auditQuickProbeStatusResponse{}, err
	}
	usable, reason, classifiedSteps, err := classifyDiagnosticRun(store, run, score)
	if err != nil {
		return auditQuickProbeStatusResponse{}, err
	}
	runResp := buildAuditDiagnosticRun(run)
	status := runResp.RunStatus
	if status == "" {
		status = run.Status
	}
	if usable {
		status = "done"
		reason = "usable"
	} else if reason != "" {
		status = reason
	}
	if !usable && runResp.RunStatusReason == "" {
		runResp.RunStatusReason = summarizeDiagnosticFailureReason(classifiedSteps, reason)
	}
	resp := auditQuickProbeStatusResponse{
		Source:      "quick_probe",
		Status:      status,
		RunID:       run.RunID,
		Usable:      usable,
		Reason:      firstNonEmptyString(runResp.RunStatusReason, reason),
		UpdatedAt:   run.UpdatedAt,
		Methodology: runResp.MethodologyVersion,
	}
	resp.CompareURL = "/api/audit/compare/" + run.RunID
	if scoreResp := buildAuditDiagnosticScore(run, score); scoreResp != nil {
		resp.Score = scoreResp.OverallScore
	}
	return resp, nil
}

func filterAuditTargets(targets []storage.AuditTarget, provider, service, channel, model string) []storage.AuditTarget {
	out := make([]storage.AuditTarget, 0, len(targets))
	for _, target := range targets {
		if provider != "" && target.Provider != provider {
			continue
		}
		if service != "" && target.Service != service {
			continue
		}
		if channel != "" && target.Channel != channel {
			continue
		}
		if model != "" && target.Model != model {
			continue
		}
		out = append(out, target)
	}
	return out
}

func selectAuditBackfillTargets(targets []storage.AuditTarget, logs []storage.NewAPILog, maxTargets, maxModelsPerChannel int) []storage.AuditTarget {
	if maxTargets <= 0 || maxModelsPerChannel <= 0 {
		return nil
	}
	type weightedTarget struct {
		target  storage.AuditTarget
		hasLogs bool
	}
	logHit := make(map[string]struct{}, len(logs))
	for _, log := range logs {
		logHit[strconv.FormatInt(log.ChannelID, 10)+"|"+strings.TrimSpace(log.ModelName)] = struct{}{}
	}
	grouped := make(map[string][]weightedTarget)
	groupOrder := make([]string, 0)
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		channelID := extractChannelID(target.Channel)
		modelKey := strings.TrimSpace(target.RequestModel)
		if modelKey == "" {
			modelKey = strings.TrimSpace(target.Model)
		}
		_, hasLogs := logHit[channelID+"|"+modelKey]
		groupKey := strings.Join([]string{
			strings.TrimSpace(target.Provider),
			strings.TrimSpace(target.Service),
			strings.TrimSpace(target.Channel),
		}, "|")
		if _, ok := grouped[groupKey]; !ok {
			groupOrder = append(groupOrder, groupKey)
		}
		grouped[groupKey] = append(grouped[groupKey], weightedTarget{target: target, hasLogs: hasLogs})
	}
	sort.SliceStable(groupOrder, func(i, j int) bool {
		leftHasLogs := false
		rightHasLogs := false
		for _, item := range grouped[groupOrder[i]] {
			if item.hasLogs {
				leftHasLogs = true
				break
			}
		}
		for _, item := range grouped[groupOrder[j]] {
			if item.hasLogs {
				rightHasLogs = true
				break
			}
		}
		if leftHasLogs != rightHasLogs {
			return leftHasLogs
		}
		return groupOrder[i] < groupOrder[j]
	})
	selected := make([]storage.AuditTarget, 0, maxTargets)
	for _, groupKey := range groupOrder {
		items := grouped[groupKey]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].hasLogs != items[j].hasLogs {
				return items[i].hasLogs
			}
			if items[i].target.Priority != items[j].target.Priority {
				return items[i].target.Priority > items[j].target.Priority
			}
			if items[i].target.Weight != items[j].target.Weight {
				return items[i].target.Weight > items[j].target.Weight
			}
			return items[i].target.Model < items[j].target.Model
		})
		picked := 0
		for _, item := range items {
			if picked >= maxModelsPerChannel || len(selected) >= maxTargets {
				break
			}
			selected = append(selected, item.target)
			picked++
		}
		if len(selected) >= maxTargets {
			break
		}
	}
	return selected
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

func decodeAuditJSONMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err == nil && out != nil {
		return out
	}
	return map[string]any{}
}

func decodeAuditJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func decodeAuditJSONStringList(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case json.RawMessage:
		var items []string
		if err := json.Unmarshal(v, &items); err == nil {
			return items
		}
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func mapStringValue(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.Trim(fmt.Sprint(raw), `"`), "\n", " "))
}

func anyJSONValue(m map[string]any, key string) any {
	if len(m) == 0 {
		return nil
	}
	return m[key]
}

func numberValue(m map[string]any, key string) int64 {
	if len(m) == 0 {
		return 0
	}
	switch n := m[key].(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case json.Number:
		v, err := n.Int64()
		if err == nil {
			return v
		}
	}
	return 0
}

func numberValueAsFloat(m map[string]any, key string, fallback float64) float64 {
	if len(m) == 0 {
		return fallback
	}
	switch n := m[key].(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		v, err := n.Float64()
		if err == nil {
			return v
		}
	}
	return fallback
}

func mapString(v any) map[string]string {
	if v == nil {
		return nil
	}
	switch raw := v.(type) {
	case map[string]string:
		return raw
	case map[string]any:
		out := make(map[string]string, len(raw))
		for k, item := range raw {
			if s, ok := item.(string); ok {
				out[k] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func mapAny(v any) map[string]any {
	if v == nil {
		return nil
	}
	if out, ok := v.(map[string]any); ok && len(out) > 0 {
		return out
	}
	return nil
}

func stringList(v any) []string {
	switch raw := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), raw...)
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func averageFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func firstPositive(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func inferDiagnosticStepName(stepIndex int, prompt string) string {
	switch stepIndex {
	case 1:
		return "ping"
	case 2:
		return "identity"
	case 3:
		return "cutoff"
	case 4:
		return "identity_free"
	case 5:
		return "knowledge_recall"
	case 6:
		return "digit_count"
	default:
		if strings.Contains(prompt, "ping") {
			return "ping"
		}
		return ""
	}
}
