package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	ProbeCredentialModeProbeOnly     = "probe_only"
	ProbeCredentialModeProbeFallback = "probe_fallback"
	ProbeCredentialModeNewAPIOnly    = "newapi_only"
)

// AuditConfig contains read-only audit feature configuration.
type AuditConfig struct {
	Diagnostics DiagnosticsConfig `yaml:"diagnostics" json:"diagnostics"`
}

// DiagnosticsConfig controls quick-probe diagnostic runtime behavior.
type DiagnosticsConfig struct {
	Enabled           *bool                 `yaml:"enabled,omitempty" json:"enabled"`
	Methodology       string                `yaml:"methodology" json:"methodology"`
	RequestTimeout    string                `yaml:"request_timeout" json:"request_timeout"`
	StepGapMin        string                `yaml:"step_gap_min" json:"step_gap_min"`
	StepGapMax        string                `yaml:"step_gap_max" json:"step_gap_max"`
	Cross5MBoundary   *bool                 `yaml:"cross_5m_boundary,omitempty" json:"cross_5m_boundary"`
	BaselineEnabled   *bool                 `yaml:"baseline_enabled,omitempty" json:"baseline_enabled"`
	CredentialMode    string                `yaml:"credential_mode" json:"credential_mode"`
	TemplateBinding   TemplateBindingConfig `yaml:"template_binding" json:"template_binding"`
	RequestTimeoutDur time.Duration         `yaml:"-" json:"-"`
	StepGapMinDur     time.Duration         `yaml:"-" json:"-"`
	StepGapMaxDur     time.Duration         `yaml:"-" json:"-"`
}

// TemplateBindingConfig maps audit targets to existing probe templates.
type TemplateBindingConfig struct {
	Default     map[string]string            `yaml:"default" json:"default"`
	ModelFamily map[string]map[string]string `yaml:"model_family" json:"model_family"`
	ChannelType map[string]map[string]string `yaml:"channel_type" json:"channel_type"`
}

func (c *AuditConfig) normalize() error {
	return c.Diagnostics.normalize()
}

func (c *DiagnosticsConfig) normalize() error {
	if c.Enabled == nil {
		v := true
		c.Enabled = &v
	}
	if strings.TrimSpace(c.Methodology) == "" {
		c.Methodology = "quick-probe-v1"
	}
	if strings.TrimSpace(c.Methodology) != "quick-probe-v1" {
		return fmt.Errorf("audit.diagnostics.methodology 无效: %q", c.Methodology)
	}
	if strings.TrimSpace(c.RequestTimeout) == "" {
		c.RequestTimeout = "60s"
	}
	requestTimeout, err := time.ParseDuration(strings.TrimSpace(c.RequestTimeout))
	if err != nil || requestTimeout <= 0 {
		return fmt.Errorf("audit.diagnostics.request_timeout 无效: %q", c.RequestTimeout)
	}
	c.RequestTimeoutDur = requestTimeout

	if strings.TrimSpace(c.StepGapMin) == "" {
		c.StepGapMin = "1m"
	}
	stepGapMin, err := time.ParseDuration(strings.TrimSpace(c.StepGapMin))
	if err != nil || stepGapMin < 0 {
		return fmt.Errorf("audit.diagnostics.step_gap_min 无效: %q", c.StepGapMin)
	}
	c.StepGapMinDur = stepGapMin

	if strings.TrimSpace(c.StepGapMax) == "" {
		c.StepGapMax = "4m"
	}
	stepGapMax, err := time.ParseDuration(strings.TrimSpace(c.StepGapMax))
	if err != nil || stepGapMax < 0 {
		return fmt.Errorf("audit.diagnostics.step_gap_max 无效: %q", c.StepGapMax)
	}
	if stepGapMax < stepGapMin {
		return fmt.Errorf("audit.diagnostics.step_gap_max 必须 >= step_gap_min")
	}
	c.StepGapMaxDur = stepGapMax

	if c.Cross5MBoundary == nil {
		v := true
		c.Cross5MBoundary = &v
	}
	if c.BaselineEnabled == nil {
		v := true
		c.BaselineEnabled = &v
	}

	mode := strings.TrimSpace(c.CredentialMode)
	if mode == "" {
		mode = ProbeCredentialModeProbeFallback
	}
	switch mode {
	case ProbeCredentialModeProbeOnly, ProbeCredentialModeProbeFallback, ProbeCredentialModeNewAPIOnly:
		c.CredentialMode = mode
	default:
		return fmt.Errorf("audit.diagnostics.credential_mode 无效: %q", c.CredentialMode)
	}
	c.TemplateBinding.Default = normalizeStringMap(c.TemplateBinding.Default)
	c.TemplateBinding.ModelFamily = normalizeNestedStringMap(c.TemplateBinding.ModelFamily)
	c.TemplateBinding.ChannelType = normalizeNestedStringMap(c.TemplateBinding.ChannelType)
	return nil
}

func (c DiagnosticsConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func normalizeStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeNestedStringMap(in map[string]map[string]string) map[string]map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		normalized := normalizeStringMap(value)
		if key != "" && len(normalized) > 0 {
			out[key] = normalized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
