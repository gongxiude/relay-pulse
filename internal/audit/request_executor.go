package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"monitor/internal/config"
)

type diagnosticHTTPClient struct {
	client *http.Client
}

func (c *diagnosticHTTPClient) executeOpenAIChat(ctx context.Context, target DiagnosticTarget, messages []diagnosticSessionMessage) (*diagnosticExecution, error) {
	tmpl := target.Template
	if tmpl == nil {
		return nil, fmt.Errorf("diagnostic template is required")
	}
	if strings.TrimSpace(tmpl.RequestFamily) != "openai_chat" {
		return nil, fmt.Errorf("unsupported diagnostic request family %q", tmpl.RequestFamily)
	}
	if strings.TrimSpace(tmpl.ResponseParser) != "openai_chat_sse" {
		return nil, fmt.Errorf("unsupported diagnostic response parser %q", tmpl.ResponseParser)
	}
	rendered, err := renderDiagnosticRequestTemplate(tmpl, target, messages)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, rendered.Method, rendered.URL, bytes.NewReader(rendered.Body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for key, value := range rendered.Headers {
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	if target.UserID != "" && req.Header.Get("New-Api-User") == "" {
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
		RequestURL:      rendered.URL,
		RequestBody:     rendered.Body,
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
		raw, ttft, chunks, finish, responseModel, usage, err := readSSE(resp.Body)
		exec.LatencyMs = time.Since(start).Milliseconds()
		exec.TTFTMs = ttft.Milliseconds()
		exec.StreamChunks = chunks
		exec.FinishReason = finish
		exec.ResponseModel = responseModel
		exec.Usage = usage
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

type renderedDiagnosticRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    json.RawMessage
}

func renderDiagnosticRequestTemplate(tmpl *config.ProbeTemplate, target DiagnosticTarget, messages []diagnosticSessionMessage) (*renderedDiagnosticRequest, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("diagnostic template is nil")
	}
	requestURL := replaceDiagnosticPlaceholders(tmpl.URL, target)
	if strings.TrimSpace(requestURL) == "" {
		return nil, fmt.Errorf("diagnostic template url is empty")
	}
	method := strings.ToUpper(strings.TrimSpace(tmpl.Method))
	if method == "" {
		return nil, fmt.Errorf("diagnostic template method is empty")
	}
	headers := make(map[string]string, len(tmpl.Headers))
	for key, value := range tmpl.Headers {
		headers[key] = replaceDiagnosticPlaceholders(value, target)
	}
	if target.AccessToken != "" {
		if value, ok := headers["Authorization"]; !ok || value == "" || strings.Contains(value, "{{API_KEY}}") {
			headers["Authorization"] = diagnosticAuthorizationHeader(target.AccessToken)
		}
	}

	var body any
	if len(tmpl.BodyRaw) == 0 {
		body = map[string]any{}
	} else if err := json.Unmarshal(tmpl.BodyRaw, &body); err != nil {
		return nil, fmt.Errorf("parse diagnostic template body: %w", err)
	}
	body = replacePlaceholdersInJSON(body, target)
	overrides := map[string]any{
		"model":    firstNonEmpty(target.RequestModel, target.Model),
		"messages": messages,
		"stream":   true,
	}
	for name, value := range overrides {
		path, ok := tmpl.OverridePaths[name]
		if !ok || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("diagnostic template missing override path %q", name)
		}
		if err := setJSONPath(body, path, value); err != nil {
			return nil, fmt.Errorf("apply override %s: %w", name, err)
		}
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnostic request body: %w", err)
	}
	return &renderedDiagnosticRequest{
		Method:  method,
		URL:     requestURL,
		Headers: headers,
		Body:    bodyBytes,
	}, nil
}

func replaceDiagnosticPlaceholders(value string, target DiagnosticTarget) string {
	replacer := strings.NewReplacer(
		"{{BASE_URL}}", strings.TrimRight(strings.TrimSpace(target.BaseURL), "/"),
		"{{API_KEY}}", diagnosticAuthorizationHeader(target.AccessToken),
		"{{MODEL}}", firstNonEmpty(target.RequestModel, target.Model),
		"{{USER_ID}}", strings.TrimSpace(target.UserID),
	)
	return replacer.Replace(value)
}

func replacePlaceholdersInJSON(value any, target DiagnosticTarget) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = replacePlaceholdersInJSON(child, target)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = replacePlaceholdersInJSON(child, target)
		}
		return out
	case string:
		return replaceDiagnosticPlaceholders(typed, target)
	default:
		return typed
	}
}

func setJSONPath(root any, path string, value any) error {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "$.") {
		return fmt.Errorf("path %q must start with $.", path)
	}
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	if len(parts) == 0 {
		return fmt.Errorf("path %q is empty", path)
	}
	current, ok := root.(map[string]any)
	if !ok {
		return fmt.Errorf("template body root must be an object")
	}
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("path %q contains empty segment", path)
		}
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return fmt.Errorf("path %q segment %q is not an object", path, part)
		}
		current = next
	}
	return nil
}
