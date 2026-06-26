package audit

import (
	"fmt"
	"strings"
	"time"

	"monitor/internal/config"
	"monitor/internal/probe"
	"monitor/internal/storage"
)

type TemplateProbeCredentials struct {
	BaseURL     string
	AccessToken string
	UserID      string
}

func ResolveTemplateProbeName(cfg *config.AppConfig, service, explicit string) (string, error) {
	if name := strings.TrimSpace(explicit); name != "" {
		return name, nil
	}
	if cfg == nil {
		return "", fmt.Errorf("audit template probe config is nil")
	}
	service = strings.TrimSpace(service)
	if service == "" {
		return "", fmt.Errorf("audit target service is empty")
	}
	if cfg.Audit.Diagnostics.TemplateBinding.Default != nil {
		if name := strings.TrimSpace(cfg.Audit.Diagnostics.TemplateBinding.Default[service]); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("audit.diagnostics.template_binding.default 未配置 service=%s 的模板", service)
}

func BuildTemplateProbeConfig(app *config.AppConfig, target storage.AuditTarget, creds TemplateProbeCredentials, templateName, configDir string) (config.ServiceConfig, error) {
	templateName = strings.TrimSpace(templateName)
	if templateName == "" {
		return config.ServiceConfig{}, fmt.Errorf("template name is empty")
	}
	cfg := config.ServiceConfig{
		Provider:     strings.TrimSpace(target.Provider),
		Service:      strings.TrimSpace(target.Service),
		Channel:      strings.TrimSpace(target.Channel),
		Model:        strings.TrimSpace(target.Model),
		RequestModel: strings.TrimSpace(target.RequestModel),
		Template:     templateName,
		BaseURL:      strings.TrimSpace(creds.BaseURL),
		APIKey:       strings.TrimSpace(creds.AccessToken),
	}
	if cfg.RequestModel == "" {
		cfg.RequestModel = cfg.Model
	}
	if cfg.Provider == "" || cfg.Service == "" || cfg.Channel == "" || cfg.Model == "" {
		return config.ServiceConfig{}, fmt.Errorf("audit target provider/service/channel/model 不能为空")
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return config.ServiceConfig{}, fmt.Errorf("template probe credential is incomplete")
	}
	if err := config.ResolveSingleMonitor(app, &cfg, configDir); err != nil {
		return config.ServiceConfig{}, err
	}
	return cfg, nil
}

func ProbeRecordFromTemplateProbeResult(target storage.AuditTarget, result *probe.Result, observedAt time.Time) (*storage.ProbeRecord, error) {
	if result == nil {
		return nil, fmt.Errorf("template probe result is nil")
	}
	record := &storage.ProbeRecord{
		Provider:  strings.TrimSpace(target.Provider),
		Service:   strings.TrimSpace(target.Service),
		Channel:   strings.TrimSpace(target.Channel),
		Model:     strings.TrimSpace(target.Model),
		Status:    result.ProbeStatus,
		SubStatus: storage.SubStatus(strings.TrimSpace(result.SubStatus)),
		HttpCode:  result.HTTPCode,
		Latency:   result.Latency,
		Timestamp: observedAt.Unix(),
	}
	if record.SubStatus == storage.SubStatus("none") {
		record.SubStatus = storage.SubStatusNone
	}
	if result.ProbeStatus == 0 {
		record.ErrorDetail = truncateAuditProbeError(result.ErrorMessage)
	}
	return record, nil
}

func truncateAuditProbeError(value string) string {
	value = strings.TrimSpace(value)
	const max = 512
	if len(value) <= max {
		return value
	}
	return value[:max]
}
