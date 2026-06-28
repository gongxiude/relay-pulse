package audit

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"monitor/internal/config"
	"monitor/internal/storage"
)

type DiagnosticTarget struct {
	Provider         string
	Service          string
	Channel          string
	Model            string
	RequestModel     string
	BaseURL          string
	AccessToken      string
	UserID           string
	CredentialSource string
	Template         *config.ProbeTemplate
	TemplateName     string
}

type DiagnosticRunner struct {
	Client *http.Client
	Now    func() time.Time
}

type diagnosticStepDef struct {
	Name         string
	Prompt       string
	FreshSession bool
}

type diagnosticSessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type diagnosticExecution struct {
	StatusCode      int               `json:"status_code"`
	LatencyMs       int64             `json:"latency_ms"`
	TTFTMs          int64             `json:"ttft_ms"`
	StreamChunks    []string          `json:"stream_chunks,omitempty"`
	ResponseModel   string            `json:"response_model,omitempty"`
	ResponseText    string            `json:"response_text,omitempty"`
	ResponsePreview string            `json:"response_preview,omitempty"`
	FinishReason    string            `json:"finish_reason,omitempty"`
	Usage           map[string]any    `json:"usage,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	RequestURL      string            `json:"request_url,omitempty"`
	RequestBody     json.RawMessage   `json:"request_body,omitempty"`
}

type diagnosticRunResult struct {
	Run   *storage.DiagnosticRun
	Score *storage.DiagnosticScore
	Steps []*storage.DiagnosticStep
}

type streamChunkStats struct {
	StepCount        int
	BufferedSteps    int
	ChunkCounts      []int
	VisibleTextSpans []int
}

type cacheUsageStats struct {
	InputTokens       int
	CacheReadTokens   int
	CacheCreateTokens int
	HasCacheFields    bool
}

type thinkingStats struct {
	Present       bool
	TokenEstimate int
	TextLength    int
	BlockTypes    []string
}

var quickProbeSteps = []diagnosticStepDef{
	{Name: "ping", Prompt: "ping"},
	{Name: "identity", Prompt: "请严格按三行输出，不要解释：vendor: <提供商>; brand: <品牌>; model: <模型名>"},
	{Name: "cutoff", Prompt: "请直接回答你的知识截止日期。"},
	{Name: "identity_free", Prompt: "不要重复上文，直接说出你的身份与版本。"},
	{Name: "knowledge_recall", Prompt: "请简洁回答：地球围绕太阳公转一周大约多少天？"},
	{Name: "digit_count", Prompt: "请从1数到200，只输出数字和空格，不要解释。", FreshSession: true},
	{Name: "world_knowledge_tier", Prompt: "Answer with only one line: RP_WORLD_KNOWLEDGE=<number>. How many permanent members are in the United Nations Security Council?", FreshSession: true},
	{Name: "thinking_probe", Prompt: "Solve mentally and answer only RP_THINKING_CHECK=<final number>: if x=17 and y=23, compute x*y + 19.", FreshSession: true},
	{Name: "cache_seed", Prompt: "Remember this exact marker for the next message: RP_CACHE_MARKER=blue-17-river. Reply only RP_CACHE_SEEDED=1.", FreshSession: true},
	{Name: "cache_recall", Prompt: "Reply only with the marker from the previous message in the format RP_CACHE_MARKER=<marker>."},
}

func NewDiagnosticRunner(client *http.Client) *DiagnosticRunner {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &DiagnosticRunner{
		Client: client,
		Now:    time.Now,
	}
}

func (r *DiagnosticRunner) Run(ctx context.Context, target DiagnosticTarget, store diagnosticStore) (*storage.DiagnosticRun, error) {
	if store == nil {
		return nil, fmt.Errorf("diagnostic store is nil")
	}
	now := r.now()
	runID := "diag-" + uuid.NewString()
	groupID := "diaggrp-" + uuid.NewString()
	methodologyVersion := "quick-probe-v1"
	weightsHash := "legacy-summary-v1"
	run := &storage.DiagnosticRun{
		RunID:     runID,
		Provider:  target.Provider,
		Service:   target.Service,
		Channel:   target.Channel,
		Model:     target.Model,
		Status:    "running",
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
		Input: mustJSON(map[string]any{
			"provider":          target.Provider,
			"service":           target.Service,
			"channel":           target.Channel,
			"model":             target.Model,
			"request_model":     target.RequestModel,
			"base_url":          target.BaseURL,
			"template_name":     target.TemplateName,
			"credential_source": target.CredentialSource,
			"group_id":          groupID,
			"steps":             stepNames(quickProbeSteps),
		}),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		return nil, err
	}

	exec := r.client()
	conversation := make([]diagnosticSessionMessage, 0, 10)
	var (
		steps []*storage.DiagnosticStep
		tags  []string
	)

	for i, stepDef := range quickProbeSteps {
		if stepDef.FreshSession {
			conversation = nil
		}
		conversation = append(conversation, diagnosticSessionMessage{Role: "user", Content: stepDef.Prompt})
		resp, err := exec.executeOpenAIChat(ctx, target, conversation)
		stepNow := r.now()
		step := &storage.DiagnosticStep{
			RunID:     runID,
			StepIndex: i + 1,
			Prompt:    stepDef.Prompt,
			CreatedAt: stepNow.Unix(),
		}
		if stepDef.FreshSession {
			step.ResultSummary = "fresh_session"
		} else {
			step.ResultSummary = "same_session"
		}
		if err != nil {
			step.ErrorMessage = err.Error()
			meta := map[string]any{
				"step_name": stepDef.Name,
				"error":     err.Error(),
			}
			if resp != nil {
				meta["status_code"] = resp.StatusCode
				meta["latency_ms"] = resp.LatencyMs
				meta["ttft_ms"] = resp.TTFTMs
				meta["stream_chunks"] = resp.StreamChunks
				meta["response_model"] = resp.ResponseModel
				meta["finish_reason"] = resp.FinishReason
				meta["usage"] = resp.Usage
				meta["request_url"] = resp.RequestURL
				meta["request_body"] = resp.RequestBody
				meta["response_text"] = resp.ResponseText
				meta["response_headers"] = resp.ResponseHeaders
				step.ResponsePreview = previewText(resp.ResponseText)
			}
			step.ExecutionMeta = mustJSON(meta)
			step.ChannelFingerprint = fingerprintFor(target.Channel, target.Model, stepDef.Name, "", err.Error())
			step.ProviderFingerprint = fingerprintFor(target.Provider, target.Service, stepDef.Name, err.Error())
			if saveErr := store.SaveDiagnosticStep(step); saveErr != nil {
				return nil, saveErr
			}
			steps = append(steps, step)
			tags = append(tags, "request_error")
			continue
		}

		step.ResolvedPrompt = joinConversation(conversation)
		step.ResponsePreview = previewText(resp.ResponseText)
		step.ResultSummary = summarizeStep(stepDef.Name, resp)
		step.ExecutionMeta = mustJSON(map[string]any{
			"step_name":        stepDef.Name,
			"status_code":      resp.StatusCode,
			"latency_ms":       resp.LatencyMs,
			"ttft_ms":          resp.TTFTMs,
			"stream_chunks":    resp.StreamChunks,
			"response_model":   resp.ResponseModel,
			"finish_reason":    resp.FinishReason,
			"usage":            resp.Usage,
			"request_url":      resp.RequestURL,
			"request_body":     resp.RequestBody,
			"response_text":    resp.ResponseText,
			"response_headers": resp.ResponseHeaders,
		})
		step.ChannelFingerprint = fingerprintFor(target.Channel, target.Model, stepDef.Name, resp.ResponseText, strconv.FormatInt(resp.TTFTMs, 10))
		step.ProviderFingerprint = fingerprintFor(target.Provider, target.Service, stepDef.Name, resp.ResponseText, summarizeUsage(resp.Usage))
		if resp.StatusCode >= 400 {
			step.ErrorMessage = fmt.Sprintf("http_%d", resp.StatusCode)
			tags = append(tags, "http_error")
		}
		if len(resp.StreamChunks) <= 1 {
			tags = append(tags, "buffered_stream")
		}
		if strings.Contains(strings.ToLower(resp.ResponseText), "fallback") {
			tags = append(tags, "fallback")
		}
		if err := store.SaveDiagnosticStep(step); err != nil {
			return nil, err
		}
		conversation = append(conversation, diagnosticSessionMessage{Role: "assistant", Content: resp.ResponseText})
		steps = append(steps, step)
	}

	score := scoreDiagnosticRun(steps, tags)
	score.RunID = runID
	score.CreatedAt = now.Unix()
	if err := store.SaveDiagnosticScore(score); err != nil {
		return nil, err
	}
	runStatus, runStatusReason := diagnosticRunStatus(steps)
	modelFamily := diagnosticModelFamily(firstNonEmpty(target.RequestModel, target.Model))
	baselineSource := ""
	baselineRunID := ""
	candidateType := "candidate_only"
	var baselineSteps []*storage.DiagnosticStep
	if runStatus == "done" && isLikelyOfficialBaselineTarget(target) {
		baselineSource = "self_reported_official"
		candidateType = "baseline_seed"
		baselineRecord := &storage.DiagnosticBaselineRun{
			BaselineID:         "baseline-" + uuid.NewString(),
			Service:            strings.TrimSpace(target.Service),
			ModelFamily:        modelFamily,
			RunID:              runID,
			Provider:           target.Provider,
			Channel:            target.Channel,
			Source:             baselineSource,
			MethodologyVersion: methodologyVersion,
			CapturedAt:         now.Unix(),
		}
		if err := store.SaveDiagnosticBaselineRun(baselineRecord); err != nil {
			return nil, err
		}
		baselineRunID = runID
	} else if runStatus == "done" {
		latestBaseline, err := store.GetLatestDiagnosticBaselineRun(strings.TrimSpace(target.Service), modelFamily, methodologyVersion, runID)
		if err != nil {
			return nil, err
		}
		if latestBaseline != nil {
			baselineRunID = latestBaseline.RunID
			baselineSource = latestBaseline.Source
			candidateType = "candidate_with_baseline"
			baselineSteps, err = store.ListDiagnosticSteps(latestBaseline.RunID)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := store.SaveDiagnosticRunGroup(&storage.DiagnosticRunGroup{
		GroupID:            groupID,
		CandidateRunID:     runID,
		BaselineRunID:      baselineRunID,
		BaselineMode:       baselineModeForRun(baselineRunID),
		MethodologyVersion: methodologyVersion,
		WeightsHash:        weightsHash,
		CreatedAt:          now.Unix(),
	}); err != nil {
		return nil, err
	}
	dimensions := buildDimensionsForRun(runID, score, tags, steps, baselineSteps, now.Unix())
	for _, dimension := range dimensions {
		if err := store.SaveDiagnosticDimension(dimension); err != nil {
			return nil, err
		}
	}
	overallScore, activeWeight, skippedDimensions := dimensionScoringSummary(dimensions)
	run.Status = runStatus
	run.UpdatedAt = r.now().Unix()
	run.Output = mustJSON(map[string]any{
		"run_status":          runStatus,
		"run_status_reason":   runStatusReason,
		"baseline_mode":       baselineModeForRun(baselineRunID),
		"baseline_run_id":     baselineRunID,
		"baseline_source":     baselineSource,
		"methodology_version": methodologyVersion,
		"weights_hash":        weightsHash,
		"candidate_type":      candidateType,
		"model_family":        modelFamily,
		"overall_score":       overallScore,
		"active_weight":       activeWeight,
		"skipped_dimensions":  skippedDimensions,
		"tags":                tags,
		"score":               score,
	})
	if err := store.SaveDiagnosticRun(run); err != nil {
		return nil, err
	}
	return run, nil
}

func diagnosticRunStatus(steps []*storage.DiagnosticStep) (status string, reason string) {
	if len(steps) == 0 {
		return "failed_request", "no diagnostic steps recorded"
	}
	errorCount := 0
	authErrorCount := 0
	for _, step := range steps {
		if step == nil || strings.TrimSpace(step.ErrorMessage) == "" {
			continue
		}
		errorCount++
		if strings.Contains(strings.TrimSpace(step.ErrorMessage), "401") {
			authErrorCount++
		}
	}
	if errorCount == 0 {
		return "done", ""
	}
	if errorCount == len(steps) && authErrorCount == len(steps) {
		return "failed_auth", "all diagnostic steps returned 401 unauthorized"
	}
	if errorCount == len(steps) {
		return "failed_request", "all diagnostic steps failed"
	}
	return "done", ""
}

type diagnosticStore interface {
	SaveDiagnosticRun(*storage.DiagnosticRun) error
	SaveDiagnosticStep(*storage.DiagnosticStep) error
	SaveDiagnosticScore(*storage.DiagnosticScore) error
	SaveDiagnosticRunGroup(*storage.DiagnosticRunGroup) error
	SaveDiagnosticDimension(*storage.DiagnosticDimension) error
	SaveDiagnosticBaselineRun(*storage.DiagnosticBaselineRun) error
	GetLatestDiagnosticBaselineRun(service, modelFamily, methodologyVersion, excludeRunID string) (*storage.DiagnosticBaselineRun, error)
	GetDiagnosticRun(string) (*storage.DiagnosticRun, error)
	ListDiagnosticSteps(string) ([]*storage.DiagnosticStep, error)
}

func (r *DiagnosticRunner) client() *diagnosticHTTPClient {
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 60 * time.Second}
	}
	return &diagnosticHTTPClient{client: r.Client}
}

func (r *DiagnosticRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func diagnosticAuthorizationHeader(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	// new-api 的 TokenAuth 直接校验 Authorization 原始值，不会自动剥掉 Bearer 前缀。
	// 这里优先原样发送 access token；仅当调用方显式传入带空格的完整头值时才保留。
	if strings.Contains(token, " ") {
		return token
	}
	return token
}

func readSSE(r io.Reader) (string, time.Duration, []string, string, string, map[string]any, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024), 2<<20)
	var (
		firstChunkAt  time.Time
		start         = time.Now()
		rawLines      []string
		chunks        []string
		finish        string
		responseModel string
		usage         map[string]any
		builder       strings.Builder
	)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		rawLines = append(rawLines, line)
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			}
			if firstChunkAt.IsZero() {
				firstChunkAt = time.Now()
			}
			chunks = append(chunks, payload)
			if responseModel == "" || usage == nil || finish == "" {
				var message map[string]any
				if err := json.Unmarshal([]byte(payload), &message); err == nil {
					if model, ok := message["model"].(string); ok && strings.TrimSpace(model) != "" {
						responseModel = strings.TrimSpace(model)
					}
					if rawUsage, ok := message["usage"].(map[string]any); ok && len(rawUsage) > 0 {
						usage = rawUsage
					}
					if finish == "" {
						finish = extractFinishReason(message)
					}
				}
			}
			builder.WriteString(payload)
			builder.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), 0, chunks, finish, responseModel, usage, err
	}
	if !firstChunkAt.IsZero() {
		if strings.TrimSpace(finish) == "" {
			finish = "sse"
		}
		return builder.String(), firstChunkAt.Sub(start), chunks, finish, responseModel, usage, nil
	}
	return builder.String(), 0, chunks, finish, responseModel, usage, nil
}

func summarizeStep(name string, resp *diagnosticExecution) string {
	switch name {
	case "ping":
		if len(resp.StreamChunks) > 1 {
			return "alive_streaming"
		}
		return "alive"
	case "digit_count":
		return "digit_probe:" + strconv.Itoa(len(resp.ResponseText))
	default:
		if strings.TrimSpace(resp.ResponseText) == "" {
			return "empty_response"
		}
		return "ok"
	}
}

func buildDimensionsForRun(runID string, score *storage.DiagnosticScore, tags []string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) []*storage.DiagnosticDimension {
	dimensions := summaryDimensionsForRun(runID, score, tags, steps, createdAt)
	if len(baselineSteps) == 0 {
		return dimensions
	}
	return append(dimensions, baselineAwareDimensions(runID, steps, baselineSteps, createdAt)...)
}

func summaryDimensionsForRun(runID string, score *storage.DiagnosticScore, tags []string, steps []*storage.DiagnosticStep, createdAt int64) []*storage.DiagnosticDimension {
	if score == nil {
		return nil
	}
	dimensions := []*storage.DiagnosticDimension{
		{
			RunID:           runID,
			DimensionKey:    "authenticity_summary",
			Weight:          1,
			Score:           float64(score.AuthenticityScore) / 10.0,
			NormalizedScore: float64(score.AuthenticityScore),
			Status:          classifySummaryDimension(score.AuthenticityScore),
			Reason:          "legacy summary score from current quick probe runner",
			Evidence: mustJSON(map[string]any{
				"tags":       uniqueSortedStrings(tags),
				"step_count": len(steps),
			}),
			CreatedAt: createdAt,
		},
		{
			RunID:           runID,
			DimensionKey:    "protocol_summary",
			Weight:          1,
			Score:           float64(score.ProtocolScore) / 10.0,
			NormalizedScore: float64(score.ProtocolScore),
			Status:          classifySummaryDimension(score.ProtocolScore),
			Reason:          "legacy protocol summary before baseline-aware scorers land",
			Evidence: mustJSON(map[string]any{
				"tags":       uniqueSortedStrings(tags),
				"step_count": len(steps),
			}),
			CreatedAt: createdAt,
		},
		{
			RunID:           runID,
			DimensionKey:    "streaming_summary",
			Weight:          1,
			Score:           float64(score.SSEScore) / 10.0,
			NormalizedScore: float64(score.SSEScore),
			Status:          classifySummaryDimension(score.SSEScore),
			Reason:          "legacy SSE summary before baseline-aware scorers land",
			Evidence: mustJSON(map[string]any{
				"tags":       uniqueSortedStrings(tags),
				"step_count": len(steps),
			}),
			CreatedAt: createdAt,
		},
	}
	return dimensions
}

func baselineAwareDimensions(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) []*storage.DiagnosticDimension {
	byName := stepMapByName(steps)
	baselineByName := stepMapByName(baselineSteps)
	out := make([]*storage.DiagnosticDimension, 0, 10)
	out = append(out, scoreModelMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreServiceTierPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreAnthropicRequestIDPassthrough(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreStopReasonPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreSelfIdentityConsistency(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreEnvelopeSelfReportMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreAnthropicMsgIDFormat(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreInferenceGeoPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreSystemPromptClean(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreSDKConsistency(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreBufferDumpMatch(runID, steps, baselineSteps, createdAt))
	if candidate, ok := byName["identity"]; ok {
		out = append(out, scoreIdentityStructuredMatch(runID, candidate, baselineByName["identity"], createdAt))
	}
	if candidate, ok := byName["identity_free"]; ok {
		out = append(out, scoreIdentityFreeClean(runID, candidate, baselineByName["identity_free"], createdAt))
		out = append(out, scoreInstructionFollowingLang(runID, candidate, baselineByName["identity_free"], createdAt))
	}
	if candidate, ok := byName["cutoff"]; ok {
		out = append(out, scoreCutoffMatch(runID, candidate, baselineByName["cutoff"], createdAt))
	}
	if candidate, ok := byName["knowledge_recall"]; ok {
		out = append(out, scoreKnowledgeRecallMatch(runID, candidate, baselineByName["knowledge_recall"], createdAt))
	}
	if candidate, ok := byName["world_knowledge_tier"]; ok {
		out = append(out, scoreWorldKnowledgeTierMatch(runID, candidate, baselineByName["world_knowledge_tier"], createdAt))
	}
	out = append(out, scoreThinkingPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreThinkingVolumeMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreCacheHitRatioMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreCacheContinuityIntra(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreCacheSlidingCorrectness(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreCacheTTLConsistency(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreTierThinkingVolumeMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreLatencyBaselineMatch(runID, steps, baselineSteps, createdAt))
	return out
}

func scoreServiceTierPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateValues := collectServiceTierValues(steps)
	baselineValues := collectServiceTierValues(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "service_tier unavailable"
	switch {
	case len(candidateValues) > 0:
		score = 10
		status = "pass"
		if len(baselineValues) > 0 {
			reason = "candidate exposes service_tier like baseline"
		} else {
			reason = "candidate exposes service_tier"
		}
	case len(baselineValues) > 0:
		score = 0
		status = "fail"
		reason = "candidate missing service_tier exposed by baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "service_tier_present",
		Weight:          6,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_values": candidateValues,
			"baseline_values":  baselineValues,
		}),
		CreatedAt: createdAt,
	}
}

func scoreAnthropicRequestIDPassthrough(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateIDs := collectRequestIDChain(steps)
	baselineIDs := collectRequestIDChain(baselineSteps)
	candidateHasReq := containsReqID(candidateIDs)
	baselineHasReq := containsReqID(baselineIDs)
	score := 0.0
	status := "skip"
	reason := "request id chain unavailable"
	switch {
	case candidateHasReq:
		score = 10
		status = "pass"
		if baselineHasReq {
			reason = "candidate preserves upstream req_* request id like baseline"
		} else {
			reason = "candidate exposes upstream req_* request id"
		}
	case baselineHasReq:
		score = 0
		status = "fail"
		reason = "candidate missing upstream req_* request id exposed by baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "anthropic_request_id_passthrough",
		Weight:          4,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_request_ids": candidateIDs,
			"baseline_request_ids":  baselineIDs,
		}),
		CreatedAt: createdAt,
	}
}

func scoreStopReasonPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateReasons := collectFinishReasons(steps)
	baselineReasons := collectFinishReasons(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "stop reason unavailable"
	switch {
	case len(candidateReasons) > 0:
		score = 10
		status = "pass"
		if len(baselineReasons) > 0 {
			reason = "candidate exposes stop reason like baseline"
		} else {
			reason = "candidate exposes stop reason"
		}
	case len(baselineReasons) > 0:
		score = 0
		status = "fail"
		reason = "candidate missing stop reason exposed by baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "stop_reason_present",
		Weight:          3,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_reasons": candidateReasons,
			"baseline_reasons":  baselineReasons,
		}),
		CreatedAt: createdAt,
	}
}

func scoreSelfIdentityConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	byName := stepMapByName(steps)
	baselineByName := stepMapByName(baselineSteps)
	candidateIdentity, candidateOK := extractStructuredIdentity(visibleTextFromStep(byName["identity"]))
	baselineIdentity, baselineOK := extractStructuredIdentity(visibleTextFromStep(baselineByName["identity"]))
	candidateFree := visibleTextFromStep(byName["identity_free"])
	score := 0.0
	status := "skip"
	reason := "identity missing"
	if candidateOK {
		switch {
		case identityTextMatches(candidateIdentity, candidateFree):
			score = 10
			status = "pass"
			reason = "structured and free identity report the same family"
		case baselineOK && identitySameVendorBrand(candidateIdentity, baselineIdentity) && diagnosticModelFamily(candidateIdentity.Model) == diagnosticModelFamily(baselineIdentity.Model):
			score = 6
			status = "partial"
			reason = "candidate matches baseline family but free identity is ambiguous"
		case baselineOK:
			score = 0
			status = "fail"
			reason = "candidate identity contradicts baseline or self report"
		default:
			score = 6
			status = "partial"
			reason = "structured identity present but free identity is ambiguous"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "self_identity_consistency",
		Weight:          8,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_identity":      candidateIdentity,
			"baseline_identity":       baselineIdentity,
			"candidate_identity_free": candidateFree,
		}),
		CreatedAt: createdAt,
	}
}

func scoreEnvelopeSelfReportMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	byName := stepMapByName(steps)
	identity, identityOK := extractStructuredIdentity(visibleTextFromStep(byName["identity"]))
	responseModels := collectResponseModels(steps)
	score := 0.0
	status := "skip"
	reason := "response model missing"
	if identityOK && len(responseModels) > 0 {
		identityFamily := diagnosticModelFamily(identity.Model)
		allFamilyMatch := true
		for _, model := range responseModels {
			if diagnosticModelFamily(model) != identityFamily {
				allFamilyMatch = false
				break
			}
		}
		if allFamilyMatch {
			score = 10
			status = "pass"
			reason = "response envelope model family agrees with text identity"
		} else {
			score = 0
			status = "fail"
			reason = "response envelope model contradicts text identity"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "envelope_self_report_match",
		Weight:          3,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"identity":        identity,
			"response_models": responseModels,
		}),
		CreatedAt: createdAt,
	}
}

func scoreAnthropicMsgIDFormat(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateIDs := collectResponseIDs(steps)
	baselineIDs := collectResponseIDs(baselineSteps)
	candidateMsg := containsPrefix(candidateIDs, "msg_")
	baselineMsg := containsPrefix(baselineIDs, "msg_")
	score := 0.0
	status := "skip"
	reason := "baseline id missing"
	switch {
	case baselineMsg && candidateMsg:
		score = 10
		status = "pass"
		reason = "candidate preserves native msg_ id format like baseline"
	case baselineMsg && len(candidateIDs) > 0:
		score = 6
		status = "partial"
		reason = "candidate exposes stable id but not native msg_ format"
	case baselineMsg:
		score = 0
		status = "fail"
		reason = "candidate missing msg_ id exposed by baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "anthropic_msg_id_format",
		Weight:          8,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_ids": candidateIDs,
			"baseline_ids":  baselineIDs,
		}),
		CreatedAt: createdAt,
	}
}

func scoreInferenceGeoPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateValues := collectInferenceGeoValues(steps)
	baselineValues := collectInferenceGeoValues(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "baseline geo missing"
	switch {
	case len(baselineValues) > 0 && len(candidateValues) > 0:
		score = 10
		status = "pass"
		reason = "candidate exposes geo or trace metadata like baseline"
	case len(baselineValues) > 0:
		score = 0
		status = "fail"
		reason = "candidate missing geo or trace metadata exposed by baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "inference_geo_present",
		Weight:          5,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_values": candidateValues,
			"baseline_values":  baselineValues,
		}),
		CreatedAt: createdAt,
	}
}

func scoreSystemPromptClean(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateSizes := collectRequestBodySizes(steps)
	baselineSizes := collectRequestBodySizes(baselineSteps)
	candidateAvg := averageInts(candidateSizes)
	baselineAvg := averageInts(baselineSizes)
	score := 0.0
	status := "skip"
	reason := "baseline body missing"
	if candidateAvg > 0 && baselineAvg > 0 {
		ratio := candidateAvg / baselineAvg
		switch {
		case ratio <= 2:
			score = 10
			status = "pass"
			reason = "request body size is within 2x baseline"
		case ratio <= 4:
			score = 6
			status = "partial"
			reason = "request body size is within 4x baseline"
		default:
			score = 0
			status = "fail"
			reason = "request body size exceeds 4x baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "system_prompt_clean",
		Weight:          8,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_sizes": candidateSizes,
			"baseline_sizes":  baselineSizes,
			"candidate_avg":   candidateAvg,
			"baseline_avg":    baselineAvg,
		}),
		CreatedAt: createdAt,
	}
}

func scoreSDKConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	values := collectSDKNames(steps)
	score := 0.0
	status := "skip"
	reason := "no sdk markers"
	switch {
	case len(values) == 1:
		score = 10
		status = "pass"
		reason = "sdk marker is stable across observed steps"
	case len(values) > 1:
		score = 0
		status = "fail"
		reason = "multiple conflicting sdk markers observed"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "sdk_consistency",
		Weight:          2,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_values": values,
			"baseline_values":  collectSDKNames(baselineSteps),
		}),
		CreatedAt: createdAt,
	}
}

func scoreBufferDumpMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateStats := collectStreamChunkStats(steps)
	baselineStats := collectStreamChunkStats(baselineSteps)
	candidateRatio := bufferedRatio(candidateStats)
	baselineRatio := bufferedRatio(baselineStats)
	score := 0.0
	status := "skip"
	reason := "stream data missing"
	if candidateStats.StepCount > 0 && baselineStats.StepCount > 0 {
		diff := absFloat(candidateRatio - baselineRatio)
		switch {
		case diff <= 0.20:
			score = 10
			status = "pass"
			reason = "buffered stream ratio is within 20% of baseline"
		case diff <= 0.50:
			score = 6
			status = "partial"
			reason = "buffered stream ratio is within 50% of baseline"
		default:
			score = 0
			status = "fail"
			reason = "candidate stream is far more buffered than baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "buffer_dump_match",
		Weight:          5,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate":       candidateStats,
			"baseline":        baselineStats,
			"candidate_ratio": candidateRatio,
			"baseline_ratio":  baselineRatio,
		}),
		CreatedAt: createdAt,
	}
}

func scoreModelMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	requestModel := ""
	candidateModels := make([]string, 0, len(steps))
	baselineModels := make([]string, 0, len(baselineSteps))
	for _, step := range steps {
		meta := decodeStepMeta(step.ExecutionMeta)
		if requestModel == "" {
			requestModel = requestModelFromMeta(meta)
		}
		if model := responseModelFromMeta(meta); model != "" {
			candidateModels = append(candidateModels, model)
		}
	}
	for _, step := range baselineSteps {
		if model := responseModelFromMeta(decodeStepMeta(step.ExecutionMeta)); model != "" {
			baselineModels = append(baselineModels, model)
		}
	}
	score := 0.0
	status := "skip"
	reason := "response model unavailable"
	if requestModel != "" && len(candidateModels) > 0 {
		allMatch := true
		expectedFamily := diagnosticModelFamily(requestModel)
		for _, model := range candidateModels {
			if diagnosticModelFamily(model) != expectedFamily {
				allMatch = false
				break
			}
		}
		if allMatch {
			score = 10
			status = "pass"
			reason = "all candidate response models match requested model family"
		} else {
			score = 0
			status = "fail"
			reason = "candidate response model differs from requested model family"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "model_match",
		Weight:          14,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"request_model":    requestModel,
			"candidate_models": candidateModels,
			"baseline_models":  baselineModels,
			"request_family":   diagnosticModelFamily(requestModel),
		}),
		CreatedAt: createdAt,
	}
}

func stepMapByName(steps []*storage.DiagnosticStep) map[string]*storage.DiagnosticStep {
	out := make(map[string]*storage.DiagnosticStep, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		name := stepNameForStorageStep(step)
		if name == "" {
			continue
		}
		out[name] = step
	}
	return out
}

type structuredIdentity struct {
	Vendor string `json:"vendor"`
	Brand  string `json:"brand"`
	Model  string `json:"model"`
}

func scoreIdentityStructuredMatch(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	candidateIdentity, candidateOK := extractStructuredIdentity(candidateText)
	baselineIdentity, baselineOK := extractStructuredIdentity(baselineText)
	score := 0.0
	status := "skip"
	reason := "structured identity unavailable"

	if candidateOK && baselineOK {
		candidateModelFamily := diagnosticModelFamily(candidateIdentity.Model)
		baselineModelFamily := diagnosticModelFamily(baselineIdentity.Model)
		switch {
		case equalStructuredIdentity(candidateIdentity, baselineIdentity):
			score = 10
			status = "pass"
			reason = "candidate structured identity matches baseline"
		case strings.EqualFold(strings.TrimSpace(candidateIdentity.Vendor), strings.TrimSpace(baselineIdentity.Vendor)) &&
			strings.EqualFold(strings.TrimSpace(candidateIdentity.Brand), strings.TrimSpace(baselineIdentity.Brand)) &&
			candidateModelFamily != "" && candidateModelFamily == baselineModelFamily:
			score = 8
			status = "partial"
			reason = "candidate structured identity matches baseline family but differs in exact model string"
		default:
			score = 0
			status = "fail"
			reason = "candidate structured identity deviates from baseline"
		}
	}

	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "identity_structured_match",
		Weight:          7,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text":     candidateText,
			"baseline_text":      baselineText,
			"candidate_identity": candidateIdentity,
			"baseline_identity":  baselineIdentity,
		}),
		CreatedAt: createdAt,
	}
}

func stepNameForStorageStep(step *storage.DiagnosticStep) string {
	if step == nil {
		return ""
	}
	meta := decodeStepMeta(step.ExecutionMeta)
	if name, ok := meta["step_name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	switch step.StepIndex {
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
	case 7:
		return "world_knowledge_tier"
	case 8:
		return "thinking_probe"
	case 9:
		return "cache_seed"
	case 10:
		return "cache_recall"
	default:
		return ""
	}
}

func scoreIdentityFreeClean(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	actualText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	wrapperHits := detectWrapperIdentity(actualText)
	score := 10.0
	status := "pass"
	reason := "candidate identity-free response does not expose wrapper identity"
	if len(wrapperHits) > 0 {
		score = 0
		status = "fail"
		reason = "candidate identity-free response exposes wrapper identity"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "identity_free_clean",
		Weight:          7,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text": actualText,
			"baseline_text":  baselineText,
			"wrapper_hits":   wrapperHits,
		}),
		CreatedAt: createdAt,
	}
}

func scoreInstructionFollowingLang(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	actualText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	candidateRatio := cjkRatio(actualText)
	baselineRatio := cjkRatio(baselineText)
	score := 0.0
	status := "fail"
	reason := "candidate response is mostly non-CJK"
	switch {
	case candidateRatio >= 0.30:
		score = 10
		status = "pass"
		reason = "candidate response follows CJK instruction density"
	case candidateRatio <= 0.05:
		score = 0
	default:
		score = (candidateRatio - 0.05) / 0.25 * 10
		status = "partial"
		reason = "candidate response partially follows CJK instruction density"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "instruction_following_lang",
		Weight:          4,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text":  actualText,
			"baseline_text":   baselineText,
			"candidate_ratio": candidateRatio,
			"baseline_ratio":  baselineRatio,
		}),
		CreatedAt: createdAt,
	}
}

func scoreCutoffMatch(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	candidateMonth, candidateOK := extractCutoffMonth(candidateText)
	baselineMonth, baselineOK := extractCutoffMonth(baselineText)
	score := 0.0
	status := "skip"
	reason := "cutoff month unavailable"
	if candidateOK && baselineOK {
		diff := monthDiff(candidateMonth, baselineMonth)
		switch {
		case diff == 0:
			score = 10
			status = "pass"
			reason = "candidate cutoff matches baseline month"
		case diff == 1:
			score = 8
			status = "partial"
			reason = "candidate cutoff differs from baseline by 1 month"
		case diff <= 3:
			score = 5
			status = "partial"
			reason = "candidate cutoff differs from baseline by up to 3 months"
		default:
			score = 0
			status = "fail"
			reason = "candidate cutoff deviates from baseline month"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "cutoff_match",
		Weight:          7,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text":  candidateText,
			"baseline_text":   baselineText,
			"candidate_month": candidateMonth,
			"baseline_month":  baselineMonth,
		}),
		CreatedAt: createdAt,
	}
}

func scoreKnowledgeRecallMatch(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	candidateAnswer, candidateOK := extractKnowledgeRecallValue(candidateText)
	baselineAnswer, baselineOK := extractKnowledgeRecallValue(baselineText)
	score := 0.0
	status := "skip"
	reason := "knowledge recall answer unavailable"
	if candidateOK && baselineOK {
		diff := candidateAnswer - baselineAnswer
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff == 0:
			score = 10
			status = "pass"
			reason = "candidate knowledge recall matches baseline"
		case diff <= 1:
			score = 8
			status = "partial"
			reason = "candidate knowledge recall is close to baseline"
		default:
			score = 0
			status = "fail"
			reason = "candidate knowledge recall deviates from baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "knowledge_recall_match",
		Weight:          12,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text":   candidateText,
			"baseline_text":    baselineText,
			"candidate_answer": candidateAnswer,
			"baseline_answer":  baselineAnswer,
		}),
		CreatedAt: createdAt,
	}
}

func scoreWorldKnowledgeTierMatch(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateText := visibleTextFromStep(candidate)
	baselineText := visibleTextFromStep(baseline)
	candidateAnswer, candidateOK := extractTaggedNumber(candidateText, "RP_WORLD_KNOWLEDGE")
	baselineAnswer, baselineOK := extractTaggedNumber(baselineText, "RP_WORLD_KNOWLEDGE")
	score := 0.0
	status := "skip"
	reason := "world knowledge answer unavailable"
	if candidateOK && baselineOK {
		diff := absInt(candidateAnswer - baselineAnswer)
		switch {
		case diff == 0:
			score = 10
			status = "pass"
			reason = "candidate world knowledge answer matches baseline"
		case diff == 1:
			score = 5
			status = "partial"
			reason = "candidate world knowledge answer differs from baseline by 1"
		default:
			score = 0
			status = "fail"
			reason = "candidate world knowledge answer deviates from baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "world_knowledge_tier_match",
		Weight:          12,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text":   candidateText,
			"baseline_text":    baselineText,
			"candidate_answer": candidateAnswer,
			"baseline_answer":  baselineAnswer,
		}),
		CreatedAt: createdAt,
	}
}

func scoreThinkingPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidate := collectThinkingStats(steps)
	baseline := collectThinkingStats(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "baseline lacks thinking signal"
	if baseline.Present {
		if candidate.Present {
			score = 10
			status = "pass"
			reason = "candidate exposes thinking signal like baseline"
		} else {
			score = 0
			status = "fail"
			reason = "candidate missing thinking signal exposed by baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "thinking_present",
		Weight:          4,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate": candidate,
			"baseline":  baseline,
		}),
		CreatedAt: createdAt,
	}
}

func scoreThinkingVolumeMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidate := collectThinkingStats(steps)
	baseline := collectThinkingStats(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "baseline thinking volume unavailable"
	if baseline.Present && baseline.TokenEstimate > 0 && candidate.Present && candidate.TokenEstimate > 0 {
		score = relativeVolumeScore(float64(candidate.TokenEstimate), float64(baseline.TokenEstimate))
		status = classifyMeasuredDimensionScore(score)
		reason = "candidate thinking volume compared with baseline"
	} else if baseline.Present && baseline.TokenEstimate > 0 {
		score = 0
		status = "fail"
		reason = "candidate missing measurable thinking volume"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "thinking_volume_match",
		Weight:          6,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate": candidate,
			"baseline":  baseline,
		}),
		CreatedAt: createdAt,
	}
}

func scoreCacheHitRatioMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidate := collectCacheUsageStats(steps)
	baseline := collectCacheUsageStats(baselineSteps)
	candidateRatio := cacheReadRatio(candidate)
	baselineRatio := cacheReadRatio(baseline)
	score := 0.0
	status := "skip"
	reason := "baseline has no cache fields"
	if baseline.HasCacheFields {
		diff := absFloat(candidateRatio - baselineRatio)
		switch {
		case candidate.HasCacheFields && diff <= 0.20:
			score = 10
			status = "pass"
			reason = "candidate cache read ratio is within 20% of baseline"
		case candidate.HasCacheFields && diff <= 0.50:
			score = 6
			status = "partial"
			reason = "candidate cache read ratio is within 50% of baseline"
		default:
			score = 0
			status = "fail"
			reason = "candidate cache read ratio deviates from baseline"
		}
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "cache_hit_ratio_match",
		Weight:          20,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate":       candidate,
			"baseline":        baseline,
			"candidate_ratio": candidateRatio,
			"baseline_ratio":  baselineRatio,
		}),
		CreatedAt: createdAt,
	}
}

func scoreCacheContinuityIntra(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	byName := stepMapByName(steps)
	baselineByName := stepMapByName(baselineSteps)
	candidateText := visibleTextFromStep(byName["cache_recall"])
	baselineText := visibleTextFromStep(baselineByName["cache_recall"])
	score := 0.0
	status := "fail"
	reason := "candidate failed to recall same-session cache marker"
	if strings.Contains(candidateText, "RP_CACHE_MARKER=blue-17-river") {
		score = 10
		status = "pass"
		reason = "candidate recalled same-session cache marker"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "cache_continuity_intra",
		Weight:          14,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate_text": candidateText,
			"baseline_text":  baselineText,
		}),
		CreatedAt: createdAt,
	}
}

func scoreCacheSlidingCorrectness(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	return skippedPhase2Dimension(runID, "cache_sliding_correctness", 13, "requires scheduled repeated probes across cache TTL windows", createdAt)
}

func scoreCacheTTLConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	return skippedPhase2Dimension(runID, "cache_ttl_consistency", 15, "requires scheduled repeated probes across cache TTL windows", createdAt)
}

func scoreTierThinkingVolumeMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	return skippedPhase2Dimension(runID, "tier_thinking_volume_match", 8, "requires cross-tier baseline set", createdAt)
}

func skippedPhase2Dimension(runID, key string, weight int, reason string, createdAt int64) *storage.DiagnosticDimension {
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    key,
		Weight:          weight,
		Score:           0,
		NormalizedScore: 0,
		Status:          "skip",
		Reason:          reason,
		Evidence:        mustJSON(map[string]any{"reason": reason}),
		CreatedAt:       createdAt,
	}
}

func scoreLatencyBaselineMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension {
	candidateMedians := diagnosticLatencyStats(steps)
	baselineMedians := diagnosticLatencyStats(baselineSteps)
	score := 0.0
	status := "skip"
	reason := "baseline latency unavailable"
	if baselineMedians.headerMedian > 0 || baselineMedians.ttftMedian > 0 {
		headerScore := relativeLatencyScore(candidateMedians.headerMedian, baselineMedians.headerMedian)
		ttftScore := relativeLatencyScore(candidateMedians.ttftMedian, baselineMedians.ttftMedian)
		score = averageScore(headerScore, ttftScore)
		status = classifyMeasuredDimensionScore(score)
		reason = "candidate latency compared with latest registered baseline"
	}
	return &storage.DiagnosticDimension{
		RunID:           runID,
		DimensionKey:    "latency_baseline_match",
		Weight:          5,
		Score:           score,
		NormalizedScore: score * 10,
		Status:          status,
		Reason:          reason,
		Evidence: mustJSON(map[string]any{
			"candidate": map[string]any{
				"header_median_ms": candidateMedians.headerMedian,
				"ttft_median_ms":   candidateMedians.ttftMedian,
			},
			"baseline": map[string]any{
				"header_median_ms": baselineMedians.headerMedian,
				"ttft_median_ms":   baselineMedians.ttftMedian,
			},
		}),
		CreatedAt: createdAt,
	}
}

type cutoffMonth struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

var (
	cutoffYearMonthPattern = regexp.MustCompile(`(20\d{2})\D{0,3}(0?[1-9]|1[0-2])`)
	dayCountPattern        = regexp.MustCompile(`\d+`)
	identitySeparator      = regexp.MustCompile(`[:：]`)
)

func requestModelFromMeta(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	body, ok := meta["request_body"].(map[string]any)
	if !ok || body == nil {
		return ""
	}
	if model, ok := body["model"].(string); ok {
		return strings.TrimSpace(model)
	}
	return ""
}

func responseModelFromMeta(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	if model, ok := meta["response_model"].(string); ok {
		return strings.TrimSpace(model)
	}
	return ""
}

func extractCutoffMonth(text string) (cutoffMonth, bool) {
	match := cutoffYearMonthPattern.FindStringSubmatch(text)
	if len(match) < 3 {
		return cutoffMonth{}, false
	}
	year, err1 := strconv.Atoi(match[1])
	month, err2 := strconv.Atoi(match[2])
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		return cutoffMonth{}, false
	}
	return cutoffMonth{Year: year, Month: month}, true
}

func monthDiff(a, b cutoffMonth) int {
	av := a.Year*12 + a.Month
	bv := b.Year*12 + b.Month
	if av > bv {
		return av - bv
	}
	return bv - av
}

func extractKnowledgeRecallValue(text string) (int, bool) {
	match := dayCountPattern.FindString(text)
	if strings.TrimSpace(match) == "" {
		return 0, false
	}
	value, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	return value, true
}

func extractTaggedNumber(text, tag string) (int, bool) {
	pattern := regexp.MustCompile(regexp.QuoteMeta(tag) + `\s*=\s*(-?\d+)`)
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return value, true
}

func extractStructuredIdentity(text string) (structuredIdentity, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	identity := structuredIdentity{}
	freeForm := make([]string, 0, 3)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		freeForm = append(freeForm, line)
		lower := strings.ToLower(line)
		value := parseIdentityFieldValue(line)
		switch {
		case strings.HasPrefix(lower, "vendor"):
			identity.Vendor = value
		case strings.HasPrefix(lower, "brand"):
			identity.Brand = value
		case strings.HasPrefix(lower, "model"):
			identity.Model = value
		case strings.HasPrefix(lower, "提供商"):
			identity.Vendor = value
		case strings.HasPrefix(lower, "品牌"):
			identity.Brand = value
		case strings.HasPrefix(lower, "模型"):
			identity.Model = value
		}
	}
	if identity.Vendor == "" && len(freeForm) >= 3 {
		identity.Vendor = parseIdentityFieldValue(freeForm[0])
	}
	if identity.Brand == "" && len(freeForm) >= 3 {
		identity.Brand = parseIdentityFieldValue(freeForm[1])
	}
	if identity.Model == "" && len(freeForm) >= 3 {
		identity.Model = parseIdentityFieldValue(freeForm[2])
	}
	if strings.TrimSpace(identity.Vendor) == "" ||
		strings.TrimSpace(identity.Brand) == "" ||
		strings.TrimSpace(identity.Model) == "" {
		return structuredIdentity{}, false
	}
	return structuredIdentity{
		Vendor: strings.TrimSpace(identity.Vendor),
		Brand:  strings.TrimSpace(identity.Brand),
		Model:  strings.TrimSpace(identity.Model),
	}, true
}

func parseIdentityFieldValue(line string) string {
	parts := identitySeparator.Split(strings.TrimSpace(line), 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(line)
}

func equalStructuredIdentity(a, b structuredIdentity) bool {
	return strings.EqualFold(strings.TrimSpace(a.Vendor), strings.TrimSpace(b.Vendor)) &&
		strings.EqualFold(strings.TrimSpace(a.Brand), strings.TrimSpace(b.Brand)) &&
		strings.EqualFold(strings.TrimSpace(a.Model), strings.TrimSpace(b.Model))
}

func extractFinishReason(message map[string]any) string {
	if len(message) == 0 {
		return ""
	}
	if value, ok := message["stop_reason"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if choices, ok := message["choices"].([]any); ok {
		for _, choice := range choices {
			choiceMap, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			if value, ok := choiceMap["finish_reason"].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
			if value, ok := choiceMap["stop_reason"].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

type latencyStats struct {
	headerMedian float64
	ttftMedian   float64
}

func diagnosticLatencyStats(steps []*storage.DiagnosticStep) latencyStats {
	headers := make([]float64, 0, len(steps))
	ttfts := make([]float64, 0, len(steps))
	for _, step := range steps {
		meta := decodeStepMeta(step.ExecutionMeta)
		header := float64(numberFromMeta(meta, "http_ttfb_ms"))
		if header <= 0 {
			header = float64(numberFromMeta(meta, "latency_ms"))
		}
		ttft := float64(numberFromMeta(meta, "first_text_ms"))
		if ttft <= 0 {
			ttft = float64(numberFromMeta(meta, "ttft_ms"))
		}
		if header > 0 {
			headers = append(headers, header)
		}
		if ttft > 0 {
			ttfts = append(ttfts, ttft)
		}
	}
	return latencyStats{
		headerMedian: medianFloat64(headers),
		ttftMedian:   medianFloat64(ttfts),
	}
}

func relativeLatencyScore(candidate, baseline float64) float64 {
	if baseline <= 0 || candidate <= 0 {
		return 0
	}
	if candidate <= baseline {
		return 10
	}
	ratio := candidate / baseline
	if ratio >= 2 {
		return 0
	}
	return (2 - ratio) * 10
}

func collectServiceTierValues(steps []*storage.DiagnosticStep) []string {
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		meta := decodeStepMeta(step.ExecutionMeta)
		usage, ok := meta["usage"].(map[string]any)
		if !ok || len(usage) == 0 {
			continue
		}
		if value, ok := usage["service_tier"].(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return uniqueSortedStrings(values)
}

func collectRequestIDChain(steps []*storage.DiagnosticStep) []string {
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		meta := decodeStepMeta(step.ExecutionMeta)
		headers, ok := meta["response_headers"].(map[string]any)
		if !ok || len(headers) == 0 {
			if typedHeaders, ok := meta["response_headers"].(map[string]string); ok {
				for key, value := range typedHeaders {
					if isRequestIDHeader(key) && strings.TrimSpace(value) != "" {
						values = append(values, splitHeaderValues(value)...)
					}
				}
			}
			continue
		}
		for key, value := range headers {
			if !isRequestIDHeader(key) {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(value))
			if text == "" {
				continue
			}
			values = append(values, splitHeaderValues(text)...)
		}
	}
	return uniqueSortedStrings(values)
}

func collectResponseIDs(steps []*storage.DiagnosticStep) []string {
	values := make([]string, 0, len(steps))
	idPattern := regexp.MustCompile(`\b(?:msg_|chatcmpl-|gen-)[A-Za-z0-9._:-]+`)
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		for _, key := range []string{"response_text", "response_model", "response_id", "id", "message_id"} {
			if value := strings.TrimSpace(fmt.Sprint(meta[key])); value != "" && !strings.EqualFold(value, "<nil>") {
				values = append(values, idPattern.FindAllString(value, -1)...)
			}
		}
		if headers := mapFromMeta(meta, "response_headers"); len(headers) > 0 {
			for _, value := range headers {
				values = append(values, idPattern.FindAllString(fmt.Sprint(value), -1)...)
			}
		}
		if raw, err := json.Marshal(meta); err == nil {
			values = append(values, idPattern.FindAllString(string(raw), -1)...)
		}
	}
	return uniqueSortedStrings(values)
}

func collectInferenceGeoValues(steps []*storage.DiagnosticStep) []string {
	keys := []string{"cf-ray", "x-vercel-id", "x-amzn-trace-id", "x-request-id", "inference_region", "region", "geo"}
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		if headers := mapFromMeta(meta, "response_headers"); len(headers) > 0 {
			for key, value := range headers {
				if containsAnyFold(key, keys) {
					values = append(values, splitHeaderValues(fmt.Sprint(value))...)
				}
			}
		}
		collectValuesForKeys(meta, keys, &values)
	}
	return uniqueSortedStrings(values)
}

func collectSDKNames(steps []*storage.DiagnosticStep) []string {
	keys := []string{"sdk_name", "x-stainless-lang", "anthropic-version", "user-agent"}
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		if headers := mapFromMeta(meta, "response_headers"); len(headers) > 0 {
			for key, value := range headers {
				if containsAnyFold(key, keys) {
					values = append(values, splitHeaderValues(fmt.Sprint(value))...)
				}
			}
		}
		if requestBody := mapFromMeta(meta, "request_body"); len(requestBody) > 0 {
			collectValuesForKeys(requestBody, keys, &values)
		}
		collectValuesForKeys(meta, keys, &values)
	}
	return uniqueSortedStrings(values)
}

func collectStreamChunkStats(steps []*storage.DiagnosticStep) streamChunkStats {
	var stats streamChunkStats
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		chunks := sliceFromMeta(meta, "stream_chunks")
		if len(chunks) == 0 {
			continue
		}
		stats.StepCount++
		stats.ChunkCounts = append(stats.ChunkCounts, len(chunks))
		if len(chunks) <= 1 {
			stats.BufferedSteps++
		}
		visibleSpans := 0
		for _, chunk := range chunks {
			if strings.TrimSpace(visibleTextFromRaw(fmt.Sprint(chunk))) != "" || strings.TrimSpace(fmt.Sprint(chunk)) != "" {
				visibleSpans++
			}
		}
		stats.VisibleTextSpans = append(stats.VisibleTextSpans, visibleSpans)
	}
	return stats
}

func collectRequestBodySizes(steps []*storage.DiagnosticStep) []int {
	sizes := make([]int, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		body, ok := meta["request_body"]
		if !ok || body == nil {
			continue
		}
		switch v := body.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				sizes = append(sizes, len([]byte(v)))
			}
		default:
			if raw, err := json.Marshal(v); err == nil && len(raw) > 0 && string(raw) != "null" {
				sizes = append(sizes, len(raw))
			}
		}
	}
	return sizes
}

func collectCacheUsageStats(steps []*storage.DiagnosticStep) cacheUsageStats {
	var stats cacheUsageStats
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		usage := mapFromMeta(meta, "usage")
		if len(usage) == 0 {
			continue
		}
		stats.InputTokens += int(firstNumberFromMap(usage, "input_tokens", "prompt_tokens"))
		read := firstNumberFromMap(usage, "cache_read_input_tokens", "cache_read", "cache_tokens")
		create := firstNumberFromMap(usage, "cache_creation_input_tokens", "cache_create_tokens")
		if read > 0 || create > 0 || hasAnyKeyFold(usage, []string{"cache_read_input_tokens", "cache_read", "cache_tokens", "cache_creation_input_tokens", "cache_create_tokens"}) {
			stats.HasCacheFields = true
		}
		stats.CacheReadTokens += int(read)
		stats.CacheCreateTokens += int(create)
	}
	return stats
}

func collectThinkingStats(steps []*storage.DiagnosticStep) thinkingStats {
	var stats thinkingStats
	blockTypes := make([]string, 0)
	for _, step := range steps {
		if step == nil {
			continue
		}
		meta := decodeStepMeta(step.ExecutionMeta)
		collectThinkingFromAny(meta, &stats, &blockTypes)
		if text := strings.TrimSpace(fmt.Sprint(meta["response_text"])); text != "" && !strings.EqualFold(text, "<nil>") {
			collectThinkingFromText(text, &stats)
		}
	}
	stats.BlockTypes = uniqueSortedStrings(blockTypes)
	if stats.TokenEstimate == 0 && stats.TextLength > 0 {
		stats.TokenEstimate = estimateTokensFromTextLength(stats.TextLength)
	}
	return stats
}

func collectFinishReasons(steps []*storage.DiagnosticStep) []string {
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		meta := decodeStepMeta(step.ExecutionMeta)
		value := strings.TrimSpace(fmt.Sprint(meta["finish_reason"]))
		if value == "" || strings.EqualFold(value, "<nil>") || strings.EqualFold(value, "sse") || strings.EqualFold(value, "non_stream") {
			continue
		}
		values = append(values, value)
	}
	return uniqueSortedStrings(values)
}

func isRequestIDHeader(key string) bool {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "request-id", "x-request-id", "request_id", "anthropic-request-id":
		return true
	default:
		return false
	}
}

func splitHeaderValues(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func containsReqID(values []string) bool {
	for _, value := range values {
		if strings.Contains(strings.TrimSpace(strings.ToLower(value)), "req_") {
			return true
		}
	}
	return false
}

func containsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(strings.TrimSpace(value), prefix) {
			return true
		}
	}
	return false
}

func collectResponseModels(steps []*storage.DiagnosticStep) []string {
	values := make([]string, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		if model := responseModelFromMeta(decodeStepMeta(step.ExecutionMeta)); model != "" {
			values = append(values, model)
		}
	}
	return uniqueSortedStrings(values)
}

func identitySameVendorBrand(a, b structuredIdentity) bool {
	return strings.EqualFold(strings.TrimSpace(a.Vendor), strings.TrimSpace(b.Vendor)) &&
		strings.EqualFold(strings.TrimSpace(a.Brand), strings.TrimSpace(b.Brand))
}

func identityTextMatches(identity structuredIdentity, text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	vendor := strings.ToLower(strings.TrimSpace(identity.Vendor))
	brand := strings.ToLower(strings.TrimSpace(identity.Brand))
	family := diagnosticModelFamily(identity.Model)
	if vendor != "" && !strings.Contains(lower, vendor) {
		return false
	}
	if brand != "" && !strings.Contains(lower, brand) {
		return false
	}
	if family != "" && strings.Contains(lower, family) {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(identity.Model))
	return model != "" && strings.Contains(lower, model)
}

func averageInts(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0
	for _, value := range values {
		total += value
	}
	return float64(total) / float64(len(values))
}

func bufferedRatio(stats streamChunkStats) float64 {
	if stats.StepCount <= 0 {
		return 0
	}
	return float64(stats.BufferedSteps) / float64(stats.StepCount)
}

func cacheReadRatio(stats cacheUsageStats) float64 {
	if stats.InputTokens <= 0 {
		return 0
	}
	return float64(stats.CacheReadTokens) / float64(stats.InputTokens)
}

func relativeVolumeScore(candidate, baseline float64) float64 {
	if candidate <= 0 || baseline <= 0 {
		return 0
	}
	ratio := candidate / baseline
	if ratio < 1 {
		ratio = 1 / ratio
	}
	if ratio <= 1.2 {
		return 10
	}
	if ratio <= 2 {
		return 6
	}
	return 0
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func averageNonZero(values ...float64) float64 {
	total := 0.0
	count := 0.0
	for _, v := range values {
		if v <= 0 {
			continue
		}
		total += v
		count++
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func averageScore(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	count := 0.0
	for _, v := range values {
		total += v
		count++
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func classifyDimensionScore(score float64) string {
	switch {
	case score >= 8.5:
		return "pass"
	case score >= 6:
		return "partial"
	case score > 0:
		return "fail"
	default:
		return "skip"
	}
}

func classifyMeasuredDimensionScore(score float64) string {
	switch {
	case score >= 8.5:
		return "pass"
	case score >= 6:
		return "partial"
	default:
		return "fail"
	}
}

func dimensionScoringSummary(dimensions []*storage.DiagnosticDimension) (overallScore float64, activeWeight int, skipped []string) {
	if len(dimensions) == 0 {
		return 0, 0, nil
	}
	scoringDimensions := scoringDimensionSubset(dimensions)
	weighted := 0.0
	for _, dimension := range scoringDimensions {
		if dimension == nil {
			continue
		}
		if strings.EqualFold(dimension.Status, "skip") {
			skipped = append(skipped, dimension.DimensionKey)
			continue
		}
		activeWeight += dimension.Weight
		weighted += float64(dimension.Weight) * dimension.Score
	}
	if activeWeight > 0 {
		overallScore = weighted / float64(activeWeight) * 10
	}
	sort.Strings(skipped)
	return overallScore, activeWeight, skipped
}

func scoringDimensionSubset(dimensions []*storage.DiagnosticDimension) []*storage.DiagnosticDimension {
	real := make([]*storage.DiagnosticDimension, 0, len(dimensions))
	for _, dimension := range dimensions {
		if dimension == nil {
			continue
		}
		if isLegacySummaryDimension(dimension.DimensionKey) {
			continue
		}
		real = append(real, dimension)
	}
	if len(real) > 0 {
		return real
	}
	return dimensions
}

func isLegacySummaryDimension(key string) bool {
	switch strings.TrimSpace(key) {
	case "authenticity_summary", "protocol_summary", "streaming_summary":
		return true
	default:
		return false
	}
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func numberFromMeta(meta map[string]any, key string) int64 {
	if len(meta) == 0 {
		return 0
	}
	if v, ok := numberFromAny(meta[key]); ok {
		return v
	}
	return 0
}

func visibleTextFromStep(step *storage.DiagnosticStep) string {
	if step == nil {
		return ""
	}
	meta := decodeStepMeta(step.ExecutionMeta)
	if chunks, ok := meta["stream_chunks"].([]any); ok && len(chunks) > 0 {
		text := visibleTextFromChunks(chunks)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	if raw, ok := meta["response_text"].(string); ok && strings.TrimSpace(raw) != "" {
		text := visibleTextFromRaw(raw)
		if strings.TrimSpace(text) != "" {
			return text
		}
		return strings.TrimSpace(raw)
	}
	if strings.TrimSpace(step.ResponsePreview) != "" {
		return strings.TrimSpace(step.ResponsePreview)
	}
	return strings.TrimSpace(step.ResultSummary)
}

func visibleTextFromChunks(chunks []any) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		text := visibleTextFromRaw(fmt.Sprint(chunk))
		if strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

func visibleTextFromRaw(raw string) string {
	lines := strings.Split(raw, "\n")
	var parts []string
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
		if line == "" || line == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if choices, ok := payload["choices"].([]any); ok {
			for _, choice := range choices {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if content, ok := delta["content"].(string); ok {
						parts = append(parts, content)
					}
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func detectWrapperIdentity(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lower := strings.ToLower(text)
	keywords := []string{"kiro", "antigravity", "augment", "roocode", "windsurf", "anyclaude", "cline"}
	hits := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			hits = append(hits, keyword)
		}
	}
	return hits
}

func cjkRatio(text string) float64 {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}
	cjk := 0
	alphaNum := 0
	for _, r := range runes {
		switch {
		case isCJKRune(r):
			cjk++
			alphaNum++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			alphaNum++
		}
	}
	if alphaNum == 0 {
		return 0
	}
	return float64(cjk) / float64(alphaNum)
}

func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x3040 && r <= 0x30FF)
}

func baselineModeForRun(baselineRunID string) string {
	if strings.TrimSpace(baselineRunID) == "" {
		return "single_run_only"
	}
	return "latest_registered_baseline"
}

func skippedDimensionsForRun(baselineRunID string) []string {
	if strings.TrimSpace(baselineRunID) == "" {
		return []string{"baseline_compare_pending", "dimension_scorers_pending"}
	}
	return []string{"dimension_scorers_pending"}
}

func isLikelyOfficialBaselineTarget(target DiagnosticTarget) bool {
	parts := []string{
		target.Provider,
		target.Channel,
		target.Service,
		target.Model,
		target.RequestModel,
	}
	text := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(text, "官key直连") ||
		strings.Contains(text, "官方直连") ||
		strings.Contains(text, "official") ||
		strings.Contains(strings.ToUpper(target.Channel), "O-") ||
		strings.Contains(strings.ToUpper(target.Provider), "O-")
}

func diagnosticModelFamily(model string) string {
	value := strings.TrimSpace(strings.ToLower(model))
	if value == "" {
		return ""
	}
	for _, suffix := range []string{"-20251001", "-20250929", "-20251101"} {
		value = strings.TrimSuffix(value, suffix)
	}
	if strings.HasPrefix(value, "claude-") {
		parts := strings.Split(value, "-")
		if len(parts) >= 3 {
			if len(parts) >= 4 && isDigits(parts[3]) {
				return strings.Join(parts[:4], "-")
			}
			return strings.Join(parts[:3], "-")
		}
	}
	if strings.HasPrefix(value, "gpt-") {
		parts := strings.Split(value, "-")
		if len(parts) >= 2 {
			return strings.Join(parts[:2], "-")
		}
	}
	return value
}

func isDigits(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func classifySummaryDimension(score int) string {
	switch {
	case score >= 85:
		return "pass"
	case score >= 60:
		return "partial"
	default:
		return "fail"
	}
}

func scoreDiagnosticRun(steps []*storage.DiagnosticStep, tags []string) *storage.DiagnosticScore {
	var (
		auth  = 100
		proto = 100
		sse   = 100
	)
	uniq := make(map[string]struct{})
	for _, tag := range tags {
		uniq[tag] = struct{}{}
	}
	for _, step := range steps {
		if step.ErrorMessage != "" {
			auth -= 20
			proto -= 10
		}
		if strings.Contains(strings.ToLower(step.ResponsePreview), "fallback") {
			auth -= 15
			uniq["fallback"] = struct{}{}
		}
		if strings.Contains(strings.ToLower(step.ResultSummary), "empty") {
			auth -= 10
		}
		if len(step.ExecutionMeta) == 0 {
			proto -= 10
		}
	}
	if len(steps) > 0 {
		lastMeta := decodeStepMeta(steps[len(steps)-1].ExecutionMeta)
		if chunks, ok := lastMeta["stream_chunks"].([]any); ok && len(chunks) <= 1 {
			sse -= 20
		}
		if ttft, ok := numberFromAny(lastMeta["ttft_ms"]); ok && ttft <= 0 {
			sse -= 10
		}
	}
	if auth < 0 {
		auth = 0
	}
	if proto < 0 {
		proto = 0
	}
	if sse < 0 {
		sse = 0
	}
	tagsOut := make([]string, 0, len(uniq))
	for tag := range uniq {
		tagsOut = append(tagsOut, tag)
	}
	sort.Strings(tagsOut)
	return &storage.DiagnosticScore{
		AuthenticityScore: auth,
		ProtocolScore:     proto,
		SSEScore:          sse,
		Tags:              mustJSON(tagsOut),
	}
}

func uniqueSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	uniq := make(map[string]struct{}, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		uniq[item] = struct{}{}
	}
	out := make([]string, 0, len(uniq))
	for item := range uniq {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func fingerprintFor(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

func joinConversation(messages []diagnosticSessionMessage) string {
	items := make([]string, 0, len(messages))
	for _, msg := range messages {
		items = append(items, msg.Role+":"+msg.Content)
	}
	return strings.Join(items, "\n")
}

func headerMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ",")
	}
	return out
}

func previewText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	runes := []rune(raw)
	if len(runes) > 180 {
		return string(runes[:180]) + "…"
	}
	return raw
}

func summarizeUsage(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(v[k]))
		b.WriteByte(';')
	}
	return b.String()
}

func decodeStepMeta(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func numberFromAny(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func mapFromMeta(meta map[string]any, key string) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	switch v := meta[key].(type) {
	case map[string]any:
		return v
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, value := range v {
			out[key] = value
		}
		return out
	case json.RawMessage:
		var out map[string]any
		if err := json.Unmarshal(v, &out); err == nil {
			return out
		}
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
	}
	return nil
}

func sliceFromMeta(meta map[string]any, key string) []any {
	if len(meta) == 0 {
		return nil
	}
	switch v := meta[key].(type) {
	case []any:
		return v
	case []string:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out
	case json.RawMessage:
		var out []any
		if err := json.Unmarshal(v, &out); err == nil {
			return out
		}
	case string:
		var out []any
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
	}
	return nil
}

func containsAnyFold(value string, needles []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func hasAnyKeyFold(m map[string]any, keys []string) bool {
	for key := range m {
		for _, wanted := range keys {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(wanted)) {
				return true
			}
		}
	}
	return false
}

func firstNumberFromMap(m map[string]any, keys ...string) int64 {
	for _, wanted := range keys {
		for key, value := range m {
			if !strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(wanted)) {
				continue
			}
			if number, ok := numberFromAny(value); ok {
				return number
			}
		}
	}
	return 0
}

func collectValuesForKeys(v any, keys []string, values *[]string) {
	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			if containsAnyFold(key, keys) {
				text := strings.TrimSpace(fmt.Sprint(value))
				if text != "" && !strings.EqualFold(text, "<nil>") {
					*values = append(*values, splitHeaderValues(text)...)
				}
			}
			collectValuesForKeys(value, keys, values)
		}
	case []any:
		for _, item := range typed {
			collectValuesForKeys(item, keys, values)
		}
	}
}

func collectThinkingFromAny(v any, stats *thinkingStats, blockTypes *[]string) {
	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if lowerKey == "type" {
				if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
					*blockTypes = append(*blockTypes, text)
					if strings.EqualFold(text, "thinking") || strings.Contains(strings.ToLower(text), "reasoning") {
						stats.Present = true
					}
				}
			}
			if strings.Contains(lowerKey, "thinking") || strings.Contains(lowerKey, "reasoning") {
				stats.Present = true
				switch value := value.(type) {
				case string:
					stats.TextLength += len([]rune(value))
				default:
					if number, ok := numberFromAny(value); ok {
						stats.TokenEstimate += int(number)
					}
				}
			}
			collectThinkingFromAny(value, stats, blockTypes)
		}
	case []any:
		for _, item := range typed {
			collectThinkingFromAny(item, stats, blockTypes)
		}
	case string:
		collectThinkingFromText(typed, stats)
	}
}

func collectThinkingFromText(text string, stats *thinkingStats) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "thinking") && !strings.Contains(lower, "reasoning") && !strings.Contains(lower, "reasoning_content") {
		return
	}
	stats.Present = true
	stats.TextLength += len([]rune(trimmed))
}

func estimateTokensFromTextLength(length int) int {
	if length <= 0 {
		return 0
	}
	tokens := length / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func stepNames(steps []diagnosticStepDef) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.Name)
	}
	return out
}
