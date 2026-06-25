package api

type auditSyncStatusResponse struct {
	NewAPIBaseURL string             `json:"newapi_base_url"`
	Targets       auditTargetSummary `json:"targets"`
	Channels      any                `json:"channels"`
	LogCursor     any                `json:"log_cursor"`
}

type auditTargetSummary struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

type auditRankingRow struct {
	Provider        string  `json:"provider"`
	Service         string  `json:"service"`
	Channel         string  `json:"channel"`
	Model           string  `json:"model"`
	RequestModel    string  `json:"request_model"`
	Enabled         bool    `json:"enabled"`
	Window          string  `json:"window"`
	Total           int     `json:"total"`
	Success         int     `json:"success"`
	Error           int     `json:"error"`
	Timeout         int     `json:"timeout"`
	SuccessRate     float64 `json:"success_rate"`
	P95             int     `json:"p95"`
	P99             int     `json:"p99"`
	TokensPerSecond float64 `json:"tokens_per_second"`
	AvgFRT          float64 `json:"avg_frt"`
	Score           float64 `json:"score"`
}

type auditDiagnosticRunResponse struct {
	RunID              string `json:"run_id"`
	Provider           string `json:"provider"`
	Service            string `json:"service"`
	Channel            string `json:"channel"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
	RequestModel       string `json:"request_model,omitempty"`
	BaseURL            string `json:"base_url,omitempty"`
	GroupID            string `json:"group_id,omitempty"`
	BaselineMode       string `json:"baseline_mode,omitempty"`
	MethodologyVersion string `json:"methodology_version,omitempty"`
	WeightsHash        string `json:"weights_hash,omitempty"`
	CandidateType      string `json:"candidate_type,omitempty"`
	Input              any    `json:"input,omitempty"`
	Output             any    `json:"output,omitempty"`
}

type auditDiagnosticScoreResponse struct {
	RunID              string   `json:"run_id"`
	AuthenticityScore  int      `json:"authenticity_score"`
	ProtocolScore      int      `json:"protocol_score"`
	SSEScore           int      `json:"sse_score"`
	OverallScore       float64  `json:"overall_score"`
	ActiveWeight       int      `json:"active_weight"`
	SkippedDimensions  []string `json:"skipped_dimensions,omitempty"`
	MethodologyVersion string   `json:"methodology_version,omitempty"`
	WeightsHash        string   `json:"weights_hash,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

type auditDiagnosticExecutionResponse struct {
	StepName        string            `json:"step_name,omitempty"`
	StatusCode      int               `json:"status_code,omitempty"`
	DurationMs      int64             `json:"duration_ms,omitempty"`
	LatencyMs       int64             `json:"latency_ms,omitempty"`
	HTTPTTFBMs      int64             `json:"http_ttfb_ms,omitempty"`
	FirstTextMs     int64             `json:"first_text_ms,omitempty"`
	TTFTMs          int64             `json:"ttft_ms,omitempty"`
	FinishReason    string            `json:"finish_reason,omitempty"`
	RequestURL      string            `json:"request_url,omitempty"`
	RequestBody     any               `json:"request_body,omitempty"`
	ResponseText    string            `json:"response_text,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	Usage           map[string]any    `json:"usage,omitempty"`
	StreamChunks    []string          `json:"stream_chunks,omitempty"`
	RawMeta         any               `json:"raw_meta,omitempty"`
}

type auditDiagnosticStepResponse struct {
	ID                  int64                            `json:"id"`
	RunID               string                           `json:"run_id"`
	StepIndex           int                              `json:"step_index"`
	StepName            string                           `json:"step_name,omitempty"`
	SessionMode         string                           `json:"session_mode,omitempty"`
	Prompt              string                           `json:"prompt"`
	ResolvedPrompt      string                           `json:"resolved_prompt,omitempty"`
	ResponsePreview     string                           `json:"response_preview,omitempty"`
	ResultSummary       string                           `json:"result_summary,omitempty"`
	Execution           auditDiagnosticExecutionResponse `json:"execution"`
	ChannelFingerprint  string                           `json:"channel_fingerprint,omitempty"`
	ProviderFingerprint string                           `json:"provider_fingerprint,omitempty"`
	ErrorMessage        string                           `json:"error_message,omitempty"`
	CreatedAt           int64                            `json:"created_at"`
}

type auditDiagnosticResponse struct {
	Run   auditDiagnosticRunResponse      `json:"run"`
	Score *auditDiagnosticScoreResponse   `json:"score,omitempty"`
	Steps []auditDiagnosticStepResponse   `json:"steps"`
}

type auditCompareGroupResponse struct {
	GroupID            string `json:"group_id,omitempty"`
	CandidateRunID     string `json:"candidate_run_id"`
	BaselineRunID      string `json:"baseline_run_id,omitempty"`
	BaselineMode       string `json:"baseline_mode,omitempty"`
	MethodologyVersion string `json:"methodology_version,omitempty"`
	WeightsHash        string `json:"weights_hash,omitempty"`
}

type auditCompareStepResponse struct {
	StepIndex  int                           `json:"step_index"`
	StepName   string                        `json:"step_name,omitempty"`
	Candidate  auditDiagnosticStepResponse   `json:"candidate"`
	Baseline   *auditDiagnosticStepResponse  `json:"baseline,omitempty"`
}

type auditCompareSummaryResponse struct {
	OverallScore      float64  `json:"overall_score"`
	ActiveWeight      int      `json:"active_weight"`
	SkippedDimensions []string `json:"skipped_dimensions,omitempty"`
	Tags              []string `json:"tags,omitempty"`
}

type auditCompareResponse struct {
	Group      auditCompareGroupResponse     `json:"group"`
	Candidate  auditDiagnosticResponse       `json:"candidate"`
	Baseline   *auditDiagnosticResponse      `json:"baseline,omitempty"`
	Dimensions []any                         `json:"dimensions"`
	Steps      []auditCompareStepResponse    `json:"steps"`
	Summary    auditCompareSummaryResponse   `json:"summary"`
}

type auditDiagnosticSubmitRequest struct {
	Provider     string `json:"provider"`
	Service      string `json:"service"`
	Channel      string `json:"channel"`
	Model        string `json:"model"`
	RequestModel string `json:"request_model"`
}

type auditDiagnosticSubmitResponse struct {
	RunID string `json:"run_id"`
}
