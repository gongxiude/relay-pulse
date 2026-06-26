package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"monitor/internal/logger"
)

// ProbeTemplate 描述一次探测请求的完整模板（来自 templates/*.json）
type ProbeTemplate struct {
	Model           string            // 模型系列名（展示/DB 键）
	RequestModel    string            // 实际请求模型 ID（可选，为空时回退 Model）
	URL             string            // URL 模式，支持 {{BASE_URL}} 等占位符
	Method          string            // HTTP 方法
	Headers         map[string]string // 请求头，支持占位符
	BodyRaw         json.RawMessage   // body 原始 JSON 对象
	RequestFamily   string            // 诊断请求族（可选，如 openai_chat / anthropic_messages）
	OverridePaths   map[string]string // 诊断运行时允许覆写的 JSON 字段路径
	ResponseParser  string            // 诊断响应解析器（可选）
	SuccessContains string            // 响应校验关键字，支持 {{EXPECTED_ANSWER}}
	SlowLatency     string            // 慢请求阈值（可选，如 "4s"）
	Timeout         string            // 超时时间（可选，如 "10s"）
	Retry           *int              // 额外重试次数（*int 区分 nil vs 0）
	RetryBaseDelay  string            // 退避基准间隔（可选，如 "200ms"）
	RetryMaxDelay   string            // 退避最大间隔（可选，如 "2s"）
	RetryJitter     *float64          // 抖动比例（*float64 区分 nil vs 0）
}

// probeTemplateFile 是模板 JSON 文件的解析结构
type probeTemplateFile struct {
	Model          string            `json:"model"`
	RequestModel   string            `json:"request_model"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	Headers        map[string]string `json:"headers"`
	Body           json.RawMessage   `json:"body"`
	RequestFamily  string            `json:"request_family"`
	OverridePaths  map[string]string `json:"override_paths"`
	ResponseParser string            `json:"response_parser"`
	Response       struct {
		SuccessContains string `json:"success_contains"`
	} `json:"response"`
	Probe struct {
		SlowLatency    string   `json:"slow_latency"`
		Timeout        string   `json:"timeout"`
		Retry          *int     `json:"retry"`
		RetryBaseDelay string   `json:"retry_base_delay"`
		RetryMaxDelay  string   `json:"retry_max_delay"`
		RetryJitter    *float64 `json:"retry_jitter"`
	} `json:"probe"`
}

// LoadProbeTemplate 从 JSON 文件加载探测模板
func LoadProbeTemplate(filePath string) (*ProbeTemplate, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取模板文件失败 %s: %w", filePath, err)
	}

	var parsed probeTemplateFile
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("解析模板 JSON 失败 %s: %w", filePath, err)
	}

	tmpl := &ProbeTemplate{
		Model:           strings.TrimSpace(parsed.Model),
		RequestModel:    strings.TrimSpace(parsed.RequestModel),
		URL:             strings.TrimSpace(parsed.URL),
		Method:          strings.TrimSpace(parsed.Method),
		Headers:         parsed.Headers,
		BodyRaw:         parsed.Body,
		RequestFamily:   strings.TrimSpace(parsed.RequestFamily),
		OverridePaths:   normalizeOverridePaths(parsed.OverridePaths),
		ResponseParser:  strings.TrimSpace(parsed.ResponseParser),
		SuccessContains: strings.TrimSpace(parsed.Response.SuccessContains),
		SlowLatency:     strings.TrimSpace(parsed.Probe.SlowLatency),
		Timeout:         strings.TrimSpace(parsed.Probe.Timeout),
		Retry:           parsed.Probe.Retry,
		RetryBaseDelay:  strings.TrimSpace(parsed.Probe.RetryBaseDelay),
		RetryMaxDelay:   strings.TrimSpace(parsed.Probe.RetryMaxDelay),
		RetryJitter:     parsed.Probe.RetryJitter,
	}

	if tmpl.Method == "" {
		return nil, fmt.Errorf("模板 %s 未配置 method", filePath)
	}
	if err := validateDiagnosticTemplateContract(filePath, tmpl); err != nil {
		return nil, err
	}

	logger.Info("config", "模板加载完成", "path", filePath)
	return tmpl, nil
}

func normalizeOverridePaths(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" || value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateDiagnosticTemplateContract(filePath string, tmpl *ProbeTemplate) error {
	hasDiagnosticField := tmpl.RequestFamily != "" || len(tmpl.OverridePaths) > 0 || tmpl.ResponseParser != ""
	if !hasDiagnosticField {
		return nil
	}
	if tmpl.RequestFamily == "" {
		return fmt.Errorf("诊断模板 %s 未配置 request_family", filePath)
	}
	if len(tmpl.OverridePaths) == 0 {
		return fmt.Errorf("诊断模板 %s 未配置 override_paths", filePath)
	}
	if tmpl.ResponseParser == "" {
		return fmt.Errorf("诊断模板 %s 未配置 response_parser", filePath)
	}
	for name, path := range tmpl.OverridePaths {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("诊断模板 %s override_paths 包含空名称", filePath)
		}
		if !strings.HasPrefix(path, "$.") {
			return fmt.Errorf("诊断模板 %s override_paths.%s 必须使用 $. 开头的 JSON 路径", filePath, name)
		}
	}
	return nil
}
