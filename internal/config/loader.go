package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"monitor/internal/logger"
)

// Loader 配置加载器
type Loader struct {
	currentConfig *AppConfig
}

// NewLoader 创建配置加载器
func NewLoader() *Loader {
	return &Loader{}
}

// Load 加载并验证配置文件
func (l *Loader) Load(filename string) (*AppConfig, error) {
	// 读取文件
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析 YAML
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件路径失败: %w", err)
	}
	configDir := filepath.Dir(absPath)

	// 合并 monitors.d/ 外部 monitor 源
	// 必须在 validate 之前合并，确保所有 monitors 走完整校验/继承/规范化流程
	if err := cfg.mergeExternalMonitorSources(configDir); err != nil {
		return nil, fmt.Errorf("合并外部 monitor 配置失败: %w", err)
	}

	// 应用环境变量覆盖
	cfg.applyEnvOverrides()

	// 验证配置
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// new-api 只读接入配置强校验（仅在加载入口执行，避免污染通用配置单测）
	if err := cfg.validateNewAPIConfig(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 解析模板引用
	if err := cfg.resolveTemplates(configDir); err != nil {
		return nil, err
	}

	// 规范化配置（填充默认值等）
	if err := cfg.normalize(); err != nil {
		return nil, fmt.Errorf("配置规范化失败: %w", err)
	}

	// Phase 1: 最终态校验仅告警，不阻断启动或热更新
	// 捕获 env 覆盖、模板注入、继承和 Normalize 之后可能遗留的不一致配置
	for _, warn := range cfg.validateFinal() {
		logger.Warn("config", "最终配置校验告警", "detail", warn.Error())
	}

	l.currentConfig = &cfg
	return &cfg, nil
}

// mergeExternalMonitorSources 合并 monitors.d/ 到主配置，并执行跨源 PSC 冲突检测。
func (cfg *AppConfig) mergeExternalMonitorSources(configDir string) error {
	dirMonitors, dirFiles, err := loadMonitorsDir(configDir)
	if err != nil {
		return fmt.Errorf("加载 %s 失败: %w", MonitorsDirName, err)
	}

	// config.yaml vs monitors.d/ PSC 冲突检测
	if err := detectPSCConflicts(cfg.Monitors, dirMonitors); err != nil {
		return err
	}

	if len(dirMonitors) > 0 {
		cfg.Monitors = append(cfg.Monitors, dirMonitors...)
		logger.Info("config", "已合并 monitors.d", "files", len(dirFiles), "monitors", len(dirMonitors))
	}

	return nil
}

// detectPSCConflicts 检测 config.yaml 与 monitors.d/ 之间的 PSC 冲突。
func detectPSCConflicts(staticMonitors, dirMonitors []ServiceConfig) error {
	staticKeys := CollectPSCKeys(staticMonitors)
	dirKeys := CollectPSCKeys(dirMonitors)

	var conflicts []string
	for key := range staticKeys {
		if _, ok := dirKeys[key]; ok {
			conflicts = append(conflicts, fmt.Sprintf("PSC %s 同时出现在 config.yaml 与 %s/", key, MonitorsDirName))
		}
	}

	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("跨源 PSC 冲突:\n  %s", strings.Join(conflicts, "\n  "))
}

// CollectPSCKeys 从 monitor 列表收集去重后的 PSC key 集合（格式 "provider/service/channel"，小写）。
// 对子通道执行 parent 继承以填充 PSC 字段。
func CollectPSCKeys(monitors []ServiceConfig) map[string]struct{} {
	keys := make(map[string]struct{})
	if len(monitors) == 0 {
		return keys
	}

	// 复制一份执行 parent 继承，避免修改原始数据
	normalized := make([]ServiceConfig, len(monitors))
	copy(normalized, monitors)
	tmp := AppConfig{Monitors: normalized}
	_ = tmp.preprocessParentInheritance() // 容忍继承失败，后续 validate 会捕获

	for _, m := range tmp.Monitors {
		p := strings.TrimSpace(m.Provider)
		s := strings.TrimSpace(m.Service)
		c := strings.TrimSpace(m.Channel)
		if p == "" || s == "" || c == "" {
			continue
		}
		key := strings.ToLower(p) + "/" + strings.ToLower(s) + "/" + strings.ToLower(c)
		keys[key] = struct{}{}
	}
	return keys
}

// loadOrRollback 加载配置，失败时保持旧配置
func (l *Loader) loadOrRollback(filename string) (*AppConfig, error) {
	newConfig, err := l.Load(filename)
	if err != nil {
		// 返回错误但保持旧配置
		if l.currentConfig != nil {
			return l.currentConfig, fmt.Errorf("配置加载失败，保持旧配置: %w", err)
		}
		return nil, err
	}
	return newConfig, nil
}

// getCurrent 获取当前配置（仅包内使用）
func (l *Loader) getCurrent() *AppConfig {
	return l.currentConfig
}
