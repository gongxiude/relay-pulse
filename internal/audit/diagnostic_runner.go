package audit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"monitor/internal/storage"
)

type DiagnosticTarget struct {
	Provider     string
	Service      string
	Channel      string
	Model        string
	RequestModel string
	BaseURL      string
	AccessToken  string
	UserID       string
}

type DiagnosticRunner struct {
	Client *http.Client
	Now    func() time.Time
}

type diagnosticStepDef struct {
	Name        string
	Prompt      string
	FreshSession bool
}

type diagnosticSessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type diagnosticRequest struct {
	Model       string                    `json:"model"`
	Messages    []diagnosticSessionMessage `json:"messages"`
	Stream      bool                      `json:"stream"`
	Temperature float64                   `json:"temperature,omitempty"`
}

type diagnosticExecution struct {
	StatusCode        int               `json:"status_code"`
	LatencyMs         int64             `json:"latency_ms"`
	TTFTMs            int64             `json:"ttft_ms"`
	StreamChunks      []string          `json:"stream_chunks,omitempty"`
	ResponseText      string            `json:"response_text,omitempty"`
	ResponsePreview   string            `json:"response_preview,omitempty"`
	FinishReason      string            `json:"finish_reason,omitempty"`
	Usage             map[string]any    `json:"usage,omitempty"`
	ResponseHeaders   map[string]string `json:"response_headers,omitempty"`
	RequestURL        string            `json:"request_url,omitempty"`
	RequestBody       json.RawMessage   `json:"request_body,omitempty"`
}

type diagnosticRunResult struct {
	Run   *storage.DiagnosticRun
	Score *storage.DiagnosticScore
	Steps []*storage.DiagnosticStep
}

var quickProbeSteps = []diagnosticStepDef{
	{Name: "ping", Prompt: "ping"},
	{Name: "identity", Prompt: "请用一句话说明你是谁，你的模型名称是什么？"},
	{Name: "cutoff", Prompt: "请直接回答你的知识截止日期。"},
	{Name: "identity_free", Prompt: "不要重复上文，直接说出你的身份与版本。"},
	{Name: "knowledge_recall", Prompt: "请简洁回答：地球围绕太阳公转一周大约多少天？"},
	{Name: "digit_count", Prompt: "请从1数到200，只输出数字和空格，不要解释。", FreshSession: true},
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
			"provider":      target.Provider,
			"service":       target.Service,
			"channel":       target.Channel,
			"model":         target.Model,
			"request_model": target.RequestModel,
			"base_url":      target.BaseURL,
			"steps":         stepNames(quickProbeSteps),
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
		resp, err := exec.chat(ctx, target, conversation)
		stepNow := r.now()
		step := &storage.DiagnosticStep{
			RunID:      runID,
			StepIndex:  i + 1,
			Prompt:     stepDef.Prompt,
			CreatedAt:  stepNow.Unix(),
		}
		if stepDef.FreshSession {
			step.ResultSummary = "fresh_session"
		} else {
			step.ResultSummary = "same_session"
		}
		if err != nil {
			step.ErrorMessage = err.Error()
			step.ExecutionMeta = mustJSON(map[string]any{
				"step_name": stepDef.Name,
				"error":     err.Error(),
			})
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
			"step_name":     stepDef.Name,
			"status_code":   resp.StatusCode,
			"latency_ms":    resp.LatencyMs,
			"ttft_ms":       resp.TTFTMs,
			"stream_chunks": resp.StreamChunks,
			"finish_reason": resp.FinishReason,
			"usage":         resp.Usage,
			"request_url":   resp.RequestURL,
			"request_body":  resp.RequestBody,
			"response_text": resp.ResponseText,
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
	run.Status = "done"
	run.UpdatedAt = r.now().Unix()
	run.Output = mustJSON(map[string]any{
		"tags": tags,
		"score": score,
	})
	if err := store.SaveDiagnosticRun(run); err != nil {
		return nil, err
	}
	return run, nil
}

type diagnosticStore interface {
	SaveDiagnosticRun(*storage.DiagnosticRun) error
	SaveDiagnosticStep(*storage.DiagnosticStep) error
	SaveDiagnosticScore(*storage.DiagnosticScore) error
}

type diagnosticHTTPClient struct {
	client *http.Client
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

func (c *diagnosticHTTPClient) chat(ctx context.Context, target DiagnosticTarget, messages []diagnosticSessionMessage) (*diagnosticExecution, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}
	requestURL := baseURL + "/v1/chat/completions"
	payload := diagnosticRequest{
		Model:       firstNonEmpty(target.RequestModel, target.Model),
		Messages:    messages,
		Stream:      true,
		Temperature: 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("Content-Type", "application/json")
	if target.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+target.AccessToken)
	}
	if target.UserID != "" {
		req.Header.Set("New-Api-User", target.UserID)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	exec := &diagnosticExecution{
		StatusCode:      resp.StatusCode,
		RequestURL:      requestURL,
		RequestBody:     body,
		ResponseHeaders: headerMap(resp.Header),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		exec.ResponseText = string(raw)
		exec.ResponsePreview = previewText(exec.ResponseText)
		exec.LatencyMs = time.Since(start).Milliseconds()
		return exec, fmt.Errorf("http %d", resp.StatusCode)
	}

	ctype := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ctype, "text/event-stream") {
		raw, ttft, chunks, finish, err := readSSE(resp.Body)
		exec.LatencyMs = time.Since(start).Milliseconds()
		exec.TTFTMs = ttft.Milliseconds()
		exec.StreamChunks = chunks
		exec.FinishReason = finish
		exec.ResponseText = raw
		exec.ResponsePreview = previewText(raw)
		return exec, err
	}

	raw, err := io.ReadAll(resp.Body)
	exec.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return exec, err
	}
	exec.ResponseText = string(raw)
	exec.ResponsePreview = previewText(exec.ResponseText)
	exec.StreamChunks = []string{exec.ResponsePreview}
	exec.FinishReason = "non_stream"
	return exec, nil
}

func readSSE(r io.Reader) (string, time.Duration, []string, string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024), 2<<20)
	var (
		firstChunkAt time.Time
		start        = time.Now()
		rawLines     []string
		chunks       []string
		finish       string
		builder      strings.Builder
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
			builder.WriteString(payload)
			builder.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), 0, chunks, finish, err
	}
	if !firstChunkAt.IsZero() {
		finish = "sse"
		return builder.String(), firstChunkAt.Sub(start), chunks, finish, nil
	}
	return builder.String(), 0, chunks, finish, nil
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

func scoreDiagnosticRun(steps []*storage.DiagnosticStep, tags []string) *storage.DiagnosticScore {
	var (
		auth = 100
		proto = 100
		sse = 100
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
