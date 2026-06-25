package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"monitor/internal/logger"
)

// GitHubConfig GitHub 通用配置（用于 GraphQL 拉取等，可复用于其他 GitHub 功能）
type GitHubConfig struct {
	// 访问令牌（建议通过环境变量 GITHUB_TOKEN 注入；不返回给前端）
	Token string `yaml:"token" json:"-"`

	// 代理地址（可选）；为空时自动回退到 HTTPS_PROXY 环境变量
	Proxy string `yaml:"proxy" json:"proxy,omitempty"`

	// 请求超时时间（默认 "30s"）
	Timeout string `yaml:"timeout" json:"timeout"`

	// 解析后的超时时间（内部使用，不序列化）
	TimeoutDuration time.Duration `yaml:"-" json:"-"`
}

// NewAPIConfig new-api 只读接入配置。
// 凭据只允许来自环境变量：NEWAPI_BASE_URL, NEWAPI_ACCESS_TOKEN, NEWAPI_USER_ID。
type NewAPIConfig struct {
	BaseURL             string `yaml:"-" json:"base_url"`
	AccessToken         string `yaml:"-" json:"-"`
	UserID              string `yaml:"-" json:"-"`
	Timeout             string `yaml:"timeout" json:"timeout"`
	ChannelSyncInterval string `yaml:"channel_sync_interval" json:"channel_sync_interval"`
	LogSyncInterval     string `yaml:"log_sync_interval" json:"log_sync_interval"`
	PageSize            int    `yaml:"page_size" json:"page_size"`
}

// normalize 规范化 new-api 配置。
func (c *NewAPIConfig) normalize() error {
	if strings.TrimSpace(c.Timeout) == "" {
		c.Timeout = "10s"
	}
	d, err := time.ParseDuration(strings.TrimSpace(c.Timeout))
	if err != nil || d <= 0 {
		return fmt.Errorf("newapi.timeout 无效: %q", c.Timeout)
	}
	if strings.TrimSpace(c.ChannelSyncInterval) == "" {
		c.ChannelSyncInterval = "5m"
	}
	if d, err := time.ParseDuration(strings.TrimSpace(c.ChannelSyncInterval)); err != nil || d <= 0 {
		return fmt.Errorf("newapi.channel_sync_interval 无效: %q", c.ChannelSyncInterval)
	}
	if strings.TrimSpace(c.LogSyncInterval) == "" {
		c.LogSyncInterval = "1m"
	}
	if d, err := time.ParseDuration(strings.TrimSpace(c.LogSyncInterval)); err != nil || d <= 0 {
		return fmt.Errorf("newapi.log_sync_interval 无效: %q", c.LogSyncInterval)
	}
	if c.PageSize <= 0 {
		c.PageSize = 100
	}
	return nil
}

// Normalize 规范化 GitHub 配置（默认值、环境变量覆盖、基础校验）
func (c *GitHubConfig) normalize() error {
	// token：环境变量优先覆盖
	if envToken := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); envToken != "" {
		c.Token = envToken
	} else {
		c.Token = strings.TrimSpace(c.Token)
	}

	// proxy：优先使用配置；为空时回退到 HTTPS_PROXY
	c.Proxy = strings.TrimSpace(c.Proxy)
	if c.Proxy == "" {
		if envProxy := strings.TrimSpace(os.Getenv("HTTPS_PROXY")); envProxy != "" {
			c.Proxy = envProxy
		}
	}
	// 校验代理 URL 格式
	if c.Proxy != "" {
		u, err := url.Parse(c.Proxy)
		if err != nil || u.Scheme == "" || u.Host == "" {
			logger.Warn("config", "github.proxy 格式无效，已忽略", "value", c.Proxy)
			c.Proxy = ""
		} else {
			switch strings.ToLower(u.Scheme) {
			case "http", "https", "socks5", "socks":
				// 有效协议（socks 是 socks5 的别名）
			default:
				logger.Warn("config", "github.proxy 协议无效（仅支持 http/https/socks5/socks），已忽略", "value", c.Proxy)
				c.Proxy = ""
			}
		}
	}

	// timeout：默认 30s
	if strings.TrimSpace(c.Timeout) == "" {
		c.Timeout = "30s"
	}
	d, err := time.ParseDuration(strings.TrimSpace(c.Timeout))
	if err != nil || d <= 0 {
		logger.Warn("config", "github.timeout 无效，已回退默认值", "value", c.Timeout, "default", "30s")
		d = 30 * time.Second
		c.Timeout = "30s"
	}
	c.TimeoutDuration = d

	return nil
}

// AnnouncementsConfig 公告通知配置（GitHub Discussions / Announcements 分类）
type AnnouncementsConfig struct {
	// 是否启用公告功能（默认 true）
	Enabled *bool `yaml:"enabled" json:"enabled"`

	// GitHub 仓库坐标（默认 prehisle/relay-pulse）
	Owner string `yaml:"owner" json:"owner"`
	Repo  string `yaml:"repo" json:"repo"`

	// Discussions 分类名称（默认 "Announcements"）
	CategoryName string `yaml:"category_name" json:"category_name"`

	// 后端轮询间隔（默认 "15m"）
	PollInterval string `yaml:"poll_interval" json:"poll_interval"`

	// 近 N 小时窗口（默认 72，即 3 天）
	WindowHours int `yaml:"window_hours" json:"window_hours"`

	// 拉取最大条数（默认 20；后端按 window_hours 二次过滤）
	MaxItems int `yaml:"max_items" json:"max_items"`

	// API 响应 Cache-Control max-age（秒，默认 60）
	APIMaxAge int `yaml:"api_max_age" json:"api_max_age"`

	// 解析后的时间间隔（内部使用，不序列化）
	PollIntervalDuration time.Duration `yaml:"-" json:"-"`
	WindowDuration       time.Duration `yaml:"-" json:"-"`
}

// IsEnabled 返回是否启用公告功能
func (c *AnnouncementsConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true // 默认启用
	}
	return *c.Enabled
}

// Normalize 规范化 announcements 配置（填充默认值、解析 duration）
func (c *AnnouncementsConfig) normalize() error {
	// owner/repo：默认 prehisle/relay-pulse
	if strings.TrimSpace(c.Owner) == "" {
		c.Owner = "prehisle"
	} else {
		c.Owner = strings.TrimSpace(c.Owner)
	}
	if strings.TrimSpace(c.Repo) == "" {
		c.Repo = "relay-pulse"
	} else {
		c.Repo = strings.TrimSpace(c.Repo)
	}

	// category_name：默认 Announcements
	if strings.TrimSpace(c.CategoryName) == "" {
		c.CategoryName = "Announcements"
	} else {
		c.CategoryName = strings.TrimSpace(c.CategoryName)
	}

	// poll_interval：默认 15m
	if strings.TrimSpace(c.PollInterval) == "" {
		c.PollInterval = "15m"
	}
	d, err := time.ParseDuration(strings.TrimSpace(c.PollInterval))
	if err != nil || d <= 0 {
		logger.Warn("config", "announcements.poll_interval 无效，已回退默认值", "value", c.PollInterval, "default", "15m")
		d = 15 * time.Minute
		c.PollInterval = "15m"
	}
	c.PollIntervalDuration = d

	// window_hours：默认 72（3 天）
	if c.WindowHours == 0 {
		c.WindowHours = 72
	}
	if c.WindowHours < 1 {
		logger.Warn("config", "announcements.window_hours 无效，已回退默认值", "value", c.WindowHours, "default", 72)
		c.WindowHours = 72
	}
	c.WindowDuration = time.Duration(c.WindowHours) * time.Hour

	// max_items：默认 20
	if c.MaxItems == 0 {
		c.MaxItems = 20
	}
	if c.MaxItems < 1 {
		logger.Warn("config", "announcements.max_items 无效，已回退默认值", "value", c.MaxItems, "default", 20)
		c.MaxItems = 20
	}

	// api_max_age：默认 60 秒
	if c.APIMaxAge == 0 {
		c.APIMaxAge = 60
	}
	if c.APIMaxAge < 0 {
		logger.Warn("config", "announcements.api_max_age 无效，已回退默认值", "value", c.APIMaxAge, "default", 60)
		c.APIMaxAge = 60
	}

	return nil
}

// Cache TTL 默认值常量（集中定义，避免多处重复）
const (
	defaultCacheTTLShort = 10 * time.Second // 90m, 24h 默认 TTL
	defaultCacheTTLLong  = 60 * time.Second // 7d, 30d 默认 TTL
)

// CacheTTLConfig API 响应缓存 TTL 配置（按 period 区分）
type CacheTTLConfig struct {
	// 近 90 分钟（90m）的缓存 TTL（默认 10s）
	TTL90m string `yaml:"90m" json:"90m"`

	// 近 24 小时（24h/1d）的缓存 TTL（默认 10s）
	TTL24h string `yaml:"24h" json:"24h"`

	// 近 7 天（7d）的缓存 TTL（默认 60s）
	TTL7d string `yaml:"7d" json:"7d"`

	// 近 30 天（30d）的缓存 TTL（默认 60s）
	TTL30d string `yaml:"30d" json:"30d"`

	// 解析后的缓存 TTL（内部使用，不序列化）
	TTL90mDuration time.Duration `yaml:"-" json:"-"`
	TTL24hDuration time.Duration `yaml:"-" json:"-"`
	TTL7dDuration  time.Duration `yaml:"-" json:"-"`
	TTL30dDuration time.Duration `yaml:"-" json:"-"`
}

// Normalize 规范化 cache_ttl 配置（填充默认值并解析 duration）
func (c *CacheTTLConfig) normalize() error {
	parseOrDefault := func(period, raw string, defaultDur time.Duration) (time.Duration, error) {
		if strings.TrimSpace(raw) == "" {
			return defaultDur, nil
		}
		d, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return 0, fmt.Errorf("cache_ttl.%s 解析失败: %w", period, err)
		}
		if d <= 0 {
			return 0, fmt.Errorf("cache_ttl.%s 必须 > 0", period)
		}
		return d, nil
	}

	var err error
	c.TTL90mDuration, err = parseOrDefault("90m", c.TTL90m, defaultCacheTTLShort)
	if err != nil {
		return err
	}
	c.TTL24hDuration, err = parseOrDefault("24h", c.TTL24h, defaultCacheTTLShort)
	if err != nil {
		return err
	}
	c.TTL7dDuration, err = parseOrDefault("7d", c.TTL7d, defaultCacheTTLLong)
	if err != nil {
		return err
	}
	c.TTL30dDuration, err = parseOrDefault("30d", c.TTL30d, defaultCacheTTLLong)
	if err != nil {
		return err
	}

	return nil
}

// TTLForPeriod 根据 period 获取缓存 TTL（未配置/无效时回退默认值）
func (c *CacheTTLConfig) TTLForPeriod(period string) time.Duration {
	switch period {
	case "90m":
		if c.TTL90mDuration > 0 {
			return c.TTL90mDuration
		}
		return defaultCacheTTLShort
	case "24h", "1d":
		if c.TTL24hDuration > 0 {
			return c.TTL24hDuration
		}
		return defaultCacheTTLShort
	case "7d":
		if c.TTL7dDuration > 0 {
			return c.TTL7dDuration
		}
		return defaultCacheTTLLong
	case "30d":
		if c.TTL30dDuration > 0 {
			return c.TTL30dDuration
		}
		return defaultCacheTTLLong
	default:
		return defaultCacheTTLShort
	}
}
