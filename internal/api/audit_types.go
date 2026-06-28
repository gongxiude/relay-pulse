package api

type auditSyncStatusResponse struct {
	NewAPIBaseURL string                    `json:"newapi_base_url"`
	Targets       auditTargetSummary        `json:"targets"`
	Channels      any                       `json:"channels"`
	LogCursor     any                       `json:"log_cursor"`
	ProbeRuntime  auditProbeRuntimeResponse `json:"probe_runtime"`
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
	RunStatus          string `json:"run_status,omitempty"`
	RunStatusReason    string `json:"run_status_reason,omitempty"`
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
	Run   auditDiagnosticRunResponse    `json:"run"`
	Score *auditDiagnosticScoreResponse `json:"score,omitempty"`
	Steps []auditDiagnosticStepResponse `json:"steps"`
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
	StepIndex int                          `json:"step_index"`
	StepName  string                       `json:"step_name,omitempty"`
	Candidate auditDiagnosticStepResponse  `json:"candidate"`
	Baseline  *auditDiagnosticStepResponse `json:"baseline,omitempty"`
}

type auditCompareSummaryResponse struct {
	OverallScore      float64  `json:"overall_score"`
	ActiveWeight      int      `json:"active_weight"`
	SkippedDimensions []string `json:"skipped_dimensions,omitempty"`
	Tags              []string `json:"tags,omitempty"`
}

type auditDiagnosticDimensionResponse struct {
	RunID           string  `json:"run_id"`
	DimensionKey    string  `json:"dimension_key"`
	Weight          int     `json:"weight"`
	Score           float64 `json:"score"`
	NormalizedScore float64 `json:"normalized_score"`
	Status          string  `json:"status"`
	Reason          string  `json:"reason"`
	Evidence        any     `json:"evidence,omitempty"`
}

type auditCompareResponse struct {
	Group      auditCompareGroupResponse          `json:"group"`
	Candidate  auditDiagnosticResponse            `json:"candidate"`
	Baseline   *auditDiagnosticResponse           `json:"baseline,omitempty"`
	Dimensions []auditDiagnosticDimensionResponse `json:"dimensions"`
	Steps      []auditCompareStepResponse         `json:"steps"`
	Summary    auditCompareSummaryResponse        `json:"summary"`
}

type auditDiagnosticLatestItemResponse struct {
	Run          auditDiagnosticRunResponse    `json:"run"`
	Score        *auditDiagnosticScoreResponse `json:"score,omitempty"`
	CompareURL   string                        `json:"compare_url,omitempty"`
	Usable       bool                          `json:"usable"`
	FilterReason string                        `json:"filter_reason,omitempty"`
}

type auditDiagnosticLatestResponse struct {
	Items []auditDiagnosticLatestItemResponse `json:"items"`
	Meta  auditDiagnosticLatestMetaResponse   `json:"meta"`
}

type auditDiagnosticLatestMetaResponse struct {
	Limit int `json:"limit"`
	Count int `json:"count"`
}

type auditDiagnosticHistoryResponse struct {
	Items []auditDiagnosticLatestItemResponse `json:"items"`
	Meta  auditDiagnosticHistoryMetaResponse  `json:"meta"`
}

type auditDiagnosticHistoryMetaResponse struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	Count      int    `json:"count"`
	Total      int    `json:"total"`
	Provider   string `json:"provider,omitempty"`
	Service    string `json:"service,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status,omitempty"`
	NextOffset *int   `json:"next_offset,omitempty"`
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

type auditTargetCredentialRequest struct {
	Provider string `json:"provider"`
	Service  string `json:"service"`
	Channel  string `json:"channel"`
	APIKey   string `json:"api_key"`
}

type auditTargetCredentialResponse struct {
	Provider      string `json:"provider"`
	Service       string `json:"service"`
	Channel       string `json:"channel"`
	Updated       int    `json:"updated"`
	KeyConfigured bool   `json:"key_configured"`
	KeyLast4      string `json:"key_last4"`
}

type auditDiagnosticBackfillRequest struct {
	MaxTargets          int `json:"max_targets"`
	MaxModelsPerChannel int `json:"max_models_per_channel"`
	LookbackHours       int `json:"lookback_hours"`
}

type auditDiagnosticBackfillItemResponse struct {
	Provider string `json:"provider"`
	Service  string `json:"service"`
	Channel  string `json:"channel"`
	Model    string `json:"model"`
	RunID    string `json:"run_id,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type auditDiagnosticBackfillResponse struct {
	Selected int                                   `json:"selected"`
	Started  int                                   `json:"started"`
	Failed   int                                   `json:"failed"`
	Items    []auditDiagnosticBackfillItemResponse `json:"items"`
}

type auditTemplateProbeBackfillRequest struct {
	MaxTargets   int    `json:"max_targets"`
	TemplateName string `json:"template_name"`
}

type auditTemplateProbeBackfillItemResponse struct {
	Provider    string `json:"provider"`
	Service     string `json:"service"`
	Channel     string `json:"channel"`
	Model       string `json:"model"`
	Template    string `json:"template,omitempty"`
	Status      string `json:"status"`
	ProbeStatus int    `json:"probe_status,omitempty"`
	SubStatus   string `json:"sub_status,omitempty"`
	HTTPCode    int    `json:"http_code,omitempty"`
	Latency     int    `json:"latency,omitempty"`
	Error       string `json:"error,omitempty"`
}

type auditTemplateProbeBackfillResponse struct {
	Selected int                                      `json:"selected"`
	Probed   int                                      `json:"probed"`
	Failed   int                                      `json:"failed"`
	Items    []auditTemplateProbeBackfillItemResponse `json:"items"`
}

type auditModelStatusResponse struct {
	Items []auditModelStatusItemResponse `json:"items"`
	Meta  auditModelStatusMetaResponse   `json:"meta"`
}

type auditModelStatusMetaResponse struct {
	Window string `json:"window"`
	Count  int    `json:"count"`
}

type auditModelStatusItemResponse struct {
	Provider             string                           `json:"provider"`
	Service              string                           `json:"service"`
	Channel              string                           `json:"channel"`
	Model                string                           `json:"model"`
	RequestModel         string                           `json:"request_model"`
	Enabled              bool                             `json:"enabled"`
	CredentialConfigured bool                             `json:"credential_configured"`
	CredentialLast4      string                           `json:"credential_last4,omitempty"`
	Production           auditProductionStatusResponse    `json:"production"`
	TemplateProbe        auditTemplateProbeStatusResponse `json:"template_probe"`
	QuickProbe           auditQuickProbeStatusResponse    `json:"quick_probe"`
}

type auditProductionStatusResponse struct {
	Source      string  `json:"source"`
	Status      string  `json:"status"`
	Total       int     `json:"total"`
	Success     int     `json:"success"`
	Error       int     `json:"error"`
	Timeout     int     `json:"timeout"`
	SuccessRate float64 `json:"success_rate"`
	P95         int     `json:"p95"`
	P99         int     `json:"p99"`
	UpdatedAt   int64   `json:"updated_at,omitempty"`
}

type auditTemplateProbeStatusResponse struct {
	Source    string `json:"source"`
	Status    string `json:"status"`
	SubStatus string `json:"sub_status,omitempty"`
	HTTPCode  int    `json:"http_code,omitempty"`
	Latency   int    `json:"latency,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

type auditQuickProbeStatusResponse struct {
	Source      string  `json:"source"`
	Status      string  `json:"status"`
	RunID       string  `json:"run_id,omitempty"`
	CompareURL  string  `json:"compare_url,omitempty"`
	Usable      bool    `json:"usable"`
	Reason      string  `json:"reason,omitempty"`
	Score       float64 `json:"score,omitempty"`
	UpdatedAt   int64   `json:"updated_at,omitempty"`
	Methodology string  `json:"methodology,omitempty"`
}

type auditMethodologyDimensionResponse struct {
	Key         string `json:"key"`
	Weight      int    `json:"weight"`
	Group       string `json:"group"`
	Description string `json:"description"`
	Implemented bool   `json:"implemented"`
	Active      bool   `json:"active"`
	Phase       string `json:"phase"`
}

type auditMethodologyCoverageResponse struct {
	DoneRuns          int `json:"done_runs"`
	DimensionRuns     int `json:"dimension_runs"`
	DimensionRowCount int `json:"dimension_row_count"`
	FailedAuthRuns    int `json:"failed_auth_runs"`
	FailedRequestRuns int `json:"failed_request_runs"`
	FilteredRuns      int `json:"filtered_runs"`
}

type auditProbeRuntimeResponse struct {
	ProbeCredentialMode string `json:"probe_credential_mode"`
	ProbeAuthConfigured bool   `json:"probe_auth_configured"`
	ProbeUserConfigured bool   `json:"probe_user_configured"`
	ProbeReady          bool   `json:"probe_ready"`
	Warning             string `json:"warning,omitempty"`
}

type auditMethodologySummaryResponse struct {
	Version           string `json:"version"`
	WeightsHash       string `json:"weights_hash"`
	TotalDimensions   int    `json:"total_dimensions"`
	TotalWeight       int    `json:"total_weight"`
	ImplementedCount  int    `json:"implemented_count"`
	ImplementedWeight int    `json:"implemented_weight"`
	ActiveCount       int    `json:"active_count"`
	ActiveWeight      int    `json:"active_weight"`
}

type auditMethodologyResponse struct {
	Summary    auditMethodologySummaryResponse     `json:"summary"`
	Coverage   auditMethodologyCoverageResponse    `json:"coverage"`
	Runtime    auditProbeRuntimeResponse           `json:"runtime"`
	Dimensions []auditMethodologyDimensionResponse `json:"dimensions"`
}
