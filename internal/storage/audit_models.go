package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type ChannelSnapshot struct {
	ID               int64           `json:"id"`
	NewAPIChannelID  int64           `json:"newapi_channel_id"`
	SnapshotAt       int64           `json:"snapshot_at"`
	Provider         string          `json:"provider"`
	Service          string          `json:"service"`
	Channel          string          `json:"channel"`
	Model            string          `json:"model"`
	Enabled          bool            `json:"enabled"`
	ChannelType      string          `json:"channelType,omitempty"`
	ChannelTypeLabel string          `json:"channelTypeLabel,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
}

type ChannelSnapshotStats struct {
	SnapshotAt   int64 `json:"snapshot_at"`
	ChannelCount int   `json:"channel_count"`
	EnabledCount int   `json:"enabled_count"`
}

type LogSyncCursor struct {
	Name      string `json:"name"`
	LastID    int64  `json:"last_id"`
	LastTime  int64  `json:"last_time"`
	UpdatedAt int64  `json:"updated_at"`
}

type NewAPILog struct {
	ID                int64           `json:"id"`
	CreatedAt         int64           `json:"created_at"`
	Type              int             `json:"type"`
	Content           string          `json:"content"`
	ModelName         string          `json:"model_name"`
	Quota             int             `json:"quota"`
	PromptTokens      int             `json:"prompt_tokens"`
	CompletionTokens  int             `json:"completion_tokens"`
	UseTime           int             `json:"use_time"`
	IsStream          bool            `json:"is_stream"`
	ChannelID         int64           `json:"channel_id"`
	Group             string          `json:"group"`
	RequestID         string          `json:"request_id"`
	UpstreamRequestID string          `json:"upstream_request_id"`
	Other             json.RawMessage `json:"other,omitempty"`
}

type AuditTarget struct {
	Provider     string `json:"provider"`
	Service      string `json:"service"`
	Channel      string `json:"channel"`
	Model        string `json:"model"`
	RequestModel string `json:"request_model"`
	Group        string `json:"group"`
	Weight       int    `json:"weight"`
	Priority     int    `json:"priority"`
	Enabled      bool   `json:"enabled"`
	APIKey       string `json:"-"`
}

type AuditTargetCredentialUpdate struct {
	Provider      string `json:"provider"`
	Service       string `json:"service"`
	Channel       string `json:"channel"`
	Updated       int    `json:"updated"`
	KeyConfigured bool   `json:"key_configured"`
	KeyLast4      string `json:"key_last4"`
}

type DiagnosticRun struct {
	RunID     string          `json:"run_id"`
	Provider  string          `json:"provider"`
	Service   string          `json:"service"`
	Channel   string          `json:"channel"`
	Model     string          `json:"model"`
	Status    string          `json:"status"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
}

type DiagnosticStep struct {
	ID                  int64           `json:"id"`
	RunID               string          `json:"run_id"`
	StepIndex           int             `json:"step_index"`
	Prompt              string          `json:"prompt"`
	ResolvedPrompt      string          `json:"resolved_prompt"`
	ResponsePreview     string          `json:"response_preview"`
	ResultSummary       string          `json:"result_summary"`
	ExecutionMeta       json.RawMessage `json:"execution_meta,omitempty"`
	ChannelFingerprint  string          `json:"channel_fingerprint"`
	ProviderFingerprint string          `json:"provider_fingerprint"`
	ErrorMessage        string          `json:"error_message"`
	CreatedAt           int64           `json:"created_at"`
}

type DiagnosticScore struct {
	RunID             string          `json:"run_id"`
	AuthenticityScore int             `json:"authenticity_score"`
	ProtocolScore     int             `json:"protocol_score"`
	SSEScore          int             `json:"sse_score"`
	Tags              json.RawMessage `json:"tags,omitempty"`
	CreatedAt         int64           `json:"created_at"`
}

type DiagnosticRunGroup struct {
	GroupID            string `json:"group_id"`
	CandidateRunID     string `json:"candidate_run_id"`
	BaselineRunID      string `json:"baseline_run_id"`
	BaselineMode       string `json:"baseline_mode"`
	MethodologyVersion string `json:"methodology_version"`
	WeightsHash        string `json:"weights_hash"`
	CreatedAt          int64  `json:"created_at"`
}

type DiagnosticBaselineRun struct {
	ID                 int64  `json:"id"`
	BaselineID         string `json:"baseline_id"`
	Service            string `json:"service"`
	ModelFamily        string `json:"model_family"`
	RunID              string `json:"run_id"`
	Provider           string `json:"provider"`
	Channel            string `json:"channel"`
	Source             string `json:"source"`
	MethodologyVersion string `json:"methodology_version"`
	CapturedAt         int64  `json:"captured_at"`
}

type DiagnosticDimension struct {
	ID              int64           `json:"id"`
	RunID           string          `json:"run_id"`
	DimensionKey    string          `json:"dimension_key"`
	Weight          int             `json:"weight"`
	Score           float64         `json:"score"`
	NormalizedScore float64         `json:"normalized_score"`
	Status          string          `json:"status"`
	Reason          string          `json:"reason"`
	Evidence        json.RawMessage `json:"evidence,omitempty"`
	CreatedAt       int64           `json:"created_at"`
}

type DiagnosticRunFilter struct {
	Provider string
	Service  string
	Channel  string
	Model    string
	Status   string
	Limit    int
	Offset   int
}

func sqliteDiagnosticRunWhere(filter DiagnosticRunFilter) ([]string, []any) {
	args := make([]any, 0, 5)
	clauses := make([]string, 0, 5)
	if strings.TrimSpace(filter.Provider) != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.Service) != "" {
		clauses = append(clauses, "service = ?")
		args = append(args, strings.TrimSpace(filter.Service))
	}
	if strings.TrimSpace(filter.Channel) != "" {
		clauses = append(clauses, "channel = ?")
		args = append(args, strings.TrimSpace(filter.Channel))
	}
	if strings.TrimSpace(filter.Model) != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, strings.TrimSpace(filter.Model))
	}
	if strings.TrimSpace(filter.Status) != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, strings.TrimSpace(filter.Status))
	}
	return clauses, args
}

func postgresDiagnosticRunWhere(filter DiagnosticRunFilter) ([]string, []any, int) {
	args := make([]any, 0, 5)
	clauses := make([]string, 0, 5)
	argIndex := 1
	if strings.TrimSpace(filter.Provider) != "" {
		clauses = append(clauses, fmt.Sprintf("provider = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Provider))
		argIndex++
	}
	if strings.TrimSpace(filter.Service) != "" {
		clauses = append(clauses, fmt.Sprintf("service = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Service))
		argIndex++
	}
	if strings.TrimSpace(filter.Channel) != "" {
		clauses = append(clauses, fmt.Sprintf("channel = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Channel))
		argIndex++
	}
	if strings.TrimSpace(filter.Model) != "" {
		clauses = append(clauses, fmt.Sprintf("model = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Model))
		argIndex++
	}
	if strings.TrimSpace(filter.Status) != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Status))
		argIndex++
	}
	return clauses, args, argIndex
}

func auditTargetKey(provider, service, channel, model string) string {
	return strings.Join([]string{
		strings.TrimSpace(provider),
		strings.TrimSpace(service),
		strings.TrimSpace(channel),
		strings.TrimSpace(model),
	}, "\x00")
}

func last4(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}

type DiagnosticDimensionSummary struct {
	RunCount       int `json:"run_count"`
	DimensionCount int `json:"dimension_count"`
}

func (s *SQLiteStorage) initAuditTables(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS newapi_channel_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			newapi_channel_id INTEGER NOT NULL,
			snapshot_at INTEGER NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			raw TEXT NOT NULL DEFAULT '',
			UNIQUE(newapi_channel_id, snapshot_at)
		);`,
		`CREATE TABLE IF NOT EXISTS newapi_log_sync_cursors (
			name TEXT PRIMARY KEY,
			last_id INTEGER NOT NULL DEFAULT 0,
			last_time INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS newapi_logs (
			id INTEGER PRIMARY KEY,
			created_at INTEGER NOT NULL,
			type INTEGER NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL DEFAULT '',
			quota INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			use_time INTEGER NOT NULL DEFAULT 0,
			is_stream INTEGER NOT NULL DEFAULT 0,
			channel_id INTEGER NOT NULL DEFAULT 0,
			"group" TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			upstream_request_id TEXT NOT NULL DEFAULT '',
			other TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS audit_targets (
			provider TEXT NOT NULL,
			service TEXT NOT NULL,
			channel TEXT NOT NULL,
			model TEXT NOT NULL,
			request_model TEXT NOT NULL DEFAULT '',
			"group" TEXT NOT NULL DEFAULT '',
			weight INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			api_key TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (provider, service, channel, model)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_runs (
			run_id TEXT PRIMARY KEY,
			provider TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0,
			input_json TEXT NOT NULL DEFAULT '',
			output_json TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			step_index INTEGER NOT NULL,
			prompt TEXT NOT NULL DEFAULT '',
			resolved_prompt TEXT NOT NULL DEFAULT '',
			response_preview TEXT NOT NULL DEFAULT '',
			result_summary TEXT NOT NULL DEFAULT '',
			execution_meta TEXT NOT NULL DEFAULT '',
			channel_fingerprint TEXT NOT NULL DEFAULT '',
			provider_fingerprint TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			UNIQUE(run_id, step_index)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_scores (
			run_id TEXT PRIMARY KEY,
			authenticity_score INTEGER NOT NULL DEFAULT 0,
			protocol_score INTEGER NOT NULL DEFAULT 0,
			sse_score INTEGER NOT NULL DEFAULT 0,
			tags TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_run_groups (
			group_id TEXT PRIMARY KEY,
			candidate_run_id TEXT NOT NULL DEFAULT '',
			baseline_run_id TEXT NOT NULL DEFAULT '',
			baseline_mode TEXT NOT NULL DEFAULT '',
			methodology_version TEXT NOT NULL DEFAULT '',
			weights_hash TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_baseline_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			baseline_id TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			model_family TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			methodology_version TEXT NOT NULL DEFAULT '',
			captured_at INTEGER NOT NULL DEFAULT 0,
			UNIQUE(run_id)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_dimensions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			dimension_key TEXT NOT NULL,
			weight INTEGER NOT NULL DEFAULT 0,
			score REAL NOT NULL DEFAULT 0,
			normalized_score REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			evidence_json TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			UNIQUE(run_id, dimension_key)
		);`,
	}
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("创建审计表失败: %w", err)
		}
	}
	return nil
}

func (s *PostgresStorage) initAuditTables(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS newapi_channel_snapshots (
			id BIGSERIAL PRIMARY KEY,
			newapi_channel_id BIGINT NOT NULL,
			snapshot_at BIGINT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			raw JSONB NOT NULL DEFAULT '{}'::jsonb,
			UNIQUE(newapi_channel_id, snapshot_at)
		);`,
		`CREATE TABLE IF NOT EXISTS newapi_log_sync_cursors (
			name TEXT PRIMARY KEY,
			last_id BIGINT NOT NULL DEFAULT 0,
			last_time BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS newapi_logs (
			id BIGINT PRIMARY KEY,
			created_at BIGINT NOT NULL,
			type INTEGER NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL DEFAULT '',
			quota INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			use_time INTEGER NOT NULL DEFAULT 0,
			is_stream BOOLEAN NOT NULL DEFAULT FALSE,
			channel_id BIGINT NOT NULL DEFAULT 0,
			"group" TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			upstream_request_id TEXT NOT NULL DEFAULT '',
			other JSONB NOT NULL DEFAULT '{}'::jsonb
		);`,
		`CREATE TABLE IF NOT EXISTS audit_targets (
			provider TEXT NOT NULL,
			service TEXT NOT NULL,
			channel TEXT NOT NULL,
			model TEXT NOT NULL,
			request_model TEXT NOT NULL DEFAULT '',
			"group" TEXT NOT NULL DEFAULT '',
			weight INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			api_key TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (provider, service, channel, model)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_runs (
			run_id TEXT PRIMARY KEY,
			provider TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0,
			input_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			output_json JSONB NOT NULL DEFAULT '{}'::jsonb
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_steps (
			id BIGSERIAL PRIMARY KEY,
			run_id TEXT NOT NULL,
			step_index INTEGER NOT NULL,
			prompt TEXT NOT NULL DEFAULT '',
			resolved_prompt TEXT NOT NULL DEFAULT '',
			response_preview TEXT NOT NULL DEFAULT '',
			result_summary TEXT NOT NULL DEFAULT '',
			execution_meta JSONB NOT NULL DEFAULT '{}'::jsonb,
			channel_fingerprint TEXT NOT NULL DEFAULT '',
			provider_fingerprint TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0,
			UNIQUE(run_id, step_index)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_scores (
			run_id TEXT PRIMARY KEY,
			authenticity_score INTEGER NOT NULL DEFAULT 0,
			protocol_score INTEGER NOT NULL DEFAULT 0,
			sse_score INTEGER NOT NULL DEFAULT 0,
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_run_groups (
			group_id TEXT PRIMARY KEY,
			candidate_run_id TEXT NOT NULL DEFAULT '',
			baseline_run_id TEXT NOT NULL DEFAULT '',
			baseline_mode TEXT NOT NULL DEFAULT '',
			methodology_version TEXT NOT NULL DEFAULT '',
			weights_hash TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_baseline_runs (
			id BIGSERIAL PRIMARY KEY,
			baseline_id TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			model_family TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			methodology_version TEXT NOT NULL DEFAULT '',
			captured_at BIGINT NOT NULL DEFAULT 0,
			UNIQUE(run_id)
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_dimensions (
			id BIGSERIAL PRIMARY KEY,
			run_id TEXT NOT NULL,
			dimension_key TEXT NOT NULL,
			weight INTEGER NOT NULL DEFAULT 0,
			score DOUBLE PRECISION NOT NULL DEFAULT 0,
			normalized_score DOUBLE PRECISION NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at BIGINT NOT NULL DEFAULT 0,
			UNIQUE(run_id, dimension_key)
		);`,
	}
	for _, stmt := range schema {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("创建审计表失败 (PostgreSQL): %w", err)
		}
	}
	return nil
}

func (s *SQLiteStorage) ensureAuditTargetsAPIKeyColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(audit_targets)`)
	if err != nil {
		return fmt.Errorf("读取 audit_targets 表结构失败: %w", err)
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("扫描 audit_targets 表结构失败: %w", err)
		}
		if name == "api_key" {
			hasColumn = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 audit_targets 表结构失败: %w", err)
	}
	if hasColumn {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE audit_targets ADD COLUMN api_key TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("为 audit_targets 增加 api_key 字段失败: %w", err)
	}
	return nil
}

func (s *PostgresStorage) ensureAuditTargetsAPIKeyColumn(ctx context.Context) error {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'audit_targets'
		  AND column_name = 'api_key'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("检查 audit_targets.api_key 字段失败 (PostgreSQL): %w", err)
	}
	if count > 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `ALTER TABLE audit_targets ADD COLUMN api_key TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("为 audit_targets 增加 api_key 字段失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *SQLiteStorage) SaveChannelSnapshot(snapshot *ChannelSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	ctx := s.effectiveCtx()
	raw := ""
	if len(snapshot.Raw) > 0 {
		raw = string(snapshot.Raw)
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO newapi_channel_snapshots (newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(newapi_channel_id, snapshot_at) DO UPDATE SET
			provider=excluded.provider,
			service=excluded.service,
			channel=excluded.channel,
			model=excluded.model,
			enabled=excluded.enabled,
			raw=excluded.raw
	`, snapshot.NewAPIChannelID, snapshot.SnapshotAt, snapshot.Provider, snapshot.Service, snapshot.Channel, snapshot.Model, snapshot.Enabled, raw)
	if err != nil {
		return fmt.Errorf("保存渠道快照失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if id > 0 {
		snapshot.ID = id
	}
	return nil
}

func (s *SQLiteStorage) GetLatestChannelSnapshotStats() (*ChannelSnapshotStats, error) {
	ctx := s.effectiveCtx()
	var latest sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(snapshot_at) FROM newapi_channel_snapshots`).Scan(&latest); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照时间失败: %w", err)
	}
	if !latest.Valid {
		return nil, nil
	}
	stats := &ChannelSnapshotStats{SnapshotAt: latest.Int64}
	var enabledCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled != 0 THEN 1 ELSE 0 END), 0)
		FROM newapi_channel_snapshots
		WHERE snapshot_at = ?
	`, latest.Int64).Scan(&stats.ChannelCount, &enabledCount); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照统计失败: %w", err)
	}
	stats.EnabledCount = enabledCount
	return stats, nil
}

func (s *SQLiteStorage) ListLatestChannelSnapshots() ([]ChannelSnapshot, error) {
	ctx := s.effectiveCtx()
	var latest sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(snapshot_at) FROM newapi_channel_snapshots`).Scan(&latest); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照时间失败: %w", err)
	}
	if !latest.Valid {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw
		FROM newapi_channel_snapshots
		WHERE snapshot_at = ?
		ORDER BY provider, service, channel, newapi_channel_id
	`, latest.Int64)
	if err != nil {
		return nil, fmt.Errorf("查询最新渠道快照列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]ChannelSnapshot, 0)
	for rows.Next() {
		var snap ChannelSnapshot
		var enabled int
		var raw string
		if err := rows.Scan(&snap.ID, &snap.NewAPIChannelID, &snap.SnapshotAt, &snap.Provider, &snap.Service, &snap.Channel, &snap.Model, &enabled, &raw); err != nil {
			return nil, fmt.Errorf("扫描最新渠道快照失败: %w", err)
		}
		snap.Enabled = enabled != 0
		if raw != "" {
			snap.Raw = json.RawMessage(raw)
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最新渠道快照失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) GetChannelSnapshot(newAPIChannelID, snapshotAt int64) (*ChannelSnapshot, error) {
	ctx := s.effectiveCtx()
	var snap ChannelSnapshot
	var enabled int
	var raw string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw
		FROM newapi_channel_snapshots
		WHERE newapi_channel_id = ? AND snapshot_at = ?
	`, newAPIChannelID, snapshotAt).Scan(&snap.ID, &snap.NewAPIChannelID, &snap.SnapshotAt, &snap.Provider, &snap.Service, &snap.Channel, &snap.Model, &enabled, &raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询渠道快照失败: %w", err)
	}
	snap.Enabled = enabled != 0
	if raw != "" {
		snap.Raw = json.RawMessage(raw)
	}
	return &snap, nil
}

func (s *SQLiteStorage) UpsertLogSyncCursor(cursor *LogSyncCursor) error {
	if cursor == nil {
		return fmt.Errorf("cursor is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO newapi_log_sync_cursors (name, last_id, last_time, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			last_id=excluded.last_id,
			last_time=excluded.last_time,
			updated_at=excluded.updated_at
	`, cursor.Name, cursor.LastID, cursor.LastTime, cursor.UpdatedAt)
	if err != nil {
		return fmt.Errorf("更新日志游标失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetLogSyncCursor(name string) (*LogSyncCursor, error) {
	ctx := s.effectiveCtx()
	var c LogSyncCursor
	err := s.db.QueryRowContext(ctx, `
		SELECT name, last_id, last_time, updated_at
		FROM newapi_log_sync_cursors
		WHERE name = ?
	`, name).Scan(&c.Name, &c.LastID, &c.LastTime, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询日志游标失败: %w", err)
	}
	return &c, nil
}

func (s *SQLiteStorage) SaveNewAPILogs(logs []NewAPILog) error {
	if len(logs) == 0 {
		return nil
	}
	ctx := s.effectiveCtx()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始保存 new-api 日志事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, log := range logs {
		raw := ""
		if len(log.Other) > 0 {
			raw = string(log.Other)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO newapi_logs (id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, channel_id, "group", request_id, upstream_request_id, other)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				created_at=excluded.created_at,
				type=excluded.type,
				content=excluded.content,
				model_name=excluded.model_name,
				quota=excluded.quota,
				prompt_tokens=excluded.prompt_tokens,
				completion_tokens=excluded.completion_tokens,
				use_time=excluded.use_time,
				is_stream=excluded.is_stream,
				channel_id=excluded.channel_id,
				"group"=excluded."group",
				request_id=excluded.request_id,
				upstream_request_id=excluded.upstream_request_id,
				other=excluded.other
		`, log.ID, log.CreatedAt, log.Type, log.Content, log.ModelName, log.Quota, log.PromptTokens, log.CompletionTokens, log.UseTime, log.IsStream, log.ChannelID, log.Group, log.RequestID, log.UpstreamRequestID, raw); err != nil {
			return fmt.Errorf("写入 new-api 日志失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 new-api 日志事务失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) ListNewAPILogsSince(since int64) ([]NewAPILog, error) {
	ctx := s.effectiveCtx()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, channel_id, "group", request_id, upstream_request_id, other
		FROM newapi_logs
		WHERE created_at >= ?
		ORDER BY created_at DESC, id DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("查询 new-api 日志失败: %w", err)
	}
	defer rows.Close()

	var out []NewAPILog
	for rows.Next() {
		var log NewAPILog
		var isStream int
		var raw string
		if err := rows.Scan(&log.ID, &log.CreatedAt, &log.Type, &log.Content, &log.ModelName, &log.Quota, &log.PromptTokens, &log.CompletionTokens, &log.UseTime, &isStream, &log.ChannelID, &log.Group, &log.RequestID, &log.UpstreamRequestID, &raw); err != nil {
			return nil, fmt.Errorf("扫描 new-api 日志失败: %w", err)
		}
		log.IsStream = isStream != 0
		if raw != "" {
			log.Other = json.RawMessage(raw)
		}
		out = append(out, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 new-api 日志失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) SaveDiagnosticRun(run *DiagnosticRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_runs (run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			provider=excluded.provider,
			service=excluded.service,
			channel=excluded.channel,
			model=excluded.model,
			status=excluded.status,
			created_at=excluded.created_at,
			updated_at=excluded.updated_at,
			input_json=excluded.input_json,
			output_json=excluded.output_json
	`, run.RunID, run.Provider, run.Service, run.Channel, run.Model, run.Status, run.CreatedAt, run.UpdatedAt, string(run.Input), string(run.Output))
	if err != nil {
		return fmt.Errorf("保存诊断任务失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetDiagnosticRun(runID string) (*DiagnosticRun, error) {
	ctx := s.effectiveCtx()
	var run DiagnosticRun
	var input, output string
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json
		FROM diagnostic_runs
		WHERE run_id = ?
	`, runID).Scan(&run.RunID, &run.Provider, &run.Service, &run.Channel, &run.Model, &run.Status, &run.CreatedAt, &run.UpdatedAt, &input, &output)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询诊断任务失败: %w", err)
	}
	if input != "" {
		run.Input = json.RawMessage(input)
	}
	if output != "" {
		run.Output = json.RawMessage(output)
	}
	return &run, nil
}

func (s *SQLiteStorage) ListDiagnosticRuns(filter DiagnosticRunFilter) ([]*DiagnosticRun, error) {
	ctx := s.effectiveCtx()
	clauses, args := sqliteDiagnosticRunWhere(filter)
	query := `
		SELECT run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json
		FROM diagnostic_runs
	`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	query += ` ORDER BY created_at DESC, updated_at DESC, run_id DESC`
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询诊断任务列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]*DiagnosticRun, 0, limit)
	for rows.Next() {
		var (
			run           DiagnosticRun
			input, output string
		)
		if err := rows.Scan(&run.RunID, &run.Provider, &run.Service, &run.Channel, &run.Model, &run.Status, &run.CreatedAt, &run.UpdatedAt, &input, &output); err != nil {
			return nil, fmt.Errorf("扫描诊断任务列表失败: %w", err)
		}
		if input != "" {
			run.Input = json.RawMessage(input)
		}
		if output != "" {
			run.Output = json.RawMessage(output)
		}
		out = append(out, &run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历诊断任务列表失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) CountDiagnosticRunsFiltered(filter DiagnosticRunFilter) (int, error) {
	ctx := s.effectiveCtx()
	clauses, args := sqliteDiagnosticRunWhere(filter)
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务列表失败: %w", err)
	}
	return count, nil
}

func (s *SQLiteStorage) CountDiagnosticRuns(status string) (int, error) {
	ctx := s.effectiveCtx()
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	args := make([]any, 0, 1)
	if strings.TrimSpace(status) != "" {
		query += ` WHERE status = ?`
		args = append(args, strings.TrimSpace(status))
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务失败: %w", err)
	}
	return count, nil
}

func (s *SQLiteStorage) GetDiagnosticDimensionSummary() (*DiagnosticDimensionSummary, error) {
	ctx := s.effectiveCtx()
	var summary DiagnosticDimensionSummary
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT diagnostic_dimensions.run_id), COUNT(*)
		FROM diagnostic_dimensions
		INNER JOIN diagnostic_runs ON diagnostic_runs.run_id = diagnostic_dimensions.run_id
		WHERE diagnostic_runs.status = 'done'
	`).Scan(&summary.RunCount, &summary.DimensionCount); err != nil {
		return nil, fmt.Errorf("统计诊断维度失败: %w", err)
	}
	return &summary, nil
}

func (s *SQLiteStorage) SaveDiagnosticStep(step *DiagnosticStep) error {
	if step == nil {
		return fmt.Errorf("step is nil")
	}
	ctx := s.effectiveCtx()
	raw := ""
	if len(step.ExecutionMeta) > 0 {
		raw = string(step.ExecutionMeta)
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_steps (run_id, step_index, prompt, resolved_prompt, response_preview, result_summary, execution_meta, channel_fingerprint, provider_fingerprint, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, step_index) DO UPDATE SET
			prompt=excluded.prompt,
			resolved_prompt=excluded.resolved_prompt,
			response_preview=excluded.response_preview,
			result_summary=excluded.result_summary,
			execution_meta=excluded.execution_meta,
			channel_fingerprint=excluded.channel_fingerprint,
			provider_fingerprint=excluded.provider_fingerprint,
			error_message=excluded.error_message,
			created_at=excluded.created_at
	`, step.RunID, step.StepIndex, step.Prompt, step.ResolvedPrompt, step.ResponsePreview, step.ResultSummary, raw, step.ChannelFingerprint, step.ProviderFingerprint, step.ErrorMessage, step.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断步骤失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if id > 0 {
		step.ID = id
	}
	return nil
}

func (s *SQLiteStorage) ListDiagnosticSteps(runID string) ([]*DiagnosticStep, error) {
	ctx := s.effectiveCtx()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, step_index, prompt, resolved_prompt, response_preview, result_summary, execution_meta, channel_fingerprint, provider_fingerprint, error_message, created_at
		FROM diagnostic_steps
		WHERE run_id = ?
		ORDER BY step_index ASC, id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("查询诊断步骤失败: %w", err)
	}
	defer rows.Close()
	var out []*DiagnosticStep
	for rows.Next() {
		var step DiagnosticStep
		var raw string
		if err := rows.Scan(&step.ID, &step.RunID, &step.StepIndex, &step.Prompt, &step.ResolvedPrompt, &step.ResponsePreview, &step.ResultSummary, &raw, &step.ChannelFingerprint, &step.ProviderFingerprint, &step.ErrorMessage, &step.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描诊断步骤失败: %w", err)
		}
		if raw != "" {
			step.ExecutionMeta = json.RawMessage(raw)
		}
		out = append(out, &step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历诊断步骤失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) SaveDiagnosticScore(score *DiagnosticScore) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}
	ctx := s.effectiveCtx()
	tags := ""
	if len(score.Tags) > 0 {
		tags = string(score.Tags)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_scores (run_id, authenticity_score, protocol_score, sse_score, tags, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			authenticity_score=excluded.authenticity_score,
			protocol_score=excluded.protocol_score,
			sse_score=excluded.sse_score,
			tags=excluded.tags,
			created_at=excluded.created_at
	`, score.RunID, score.AuthenticityScore, score.ProtocolScore, score.SSEScore, tags, score.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断评分失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetDiagnosticScore(runID string) (*DiagnosticScore, error) {
	ctx := s.effectiveCtx()
	var score DiagnosticScore
	var tags string
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id, authenticity_score, protocol_score, sse_score, tags, created_at
		FROM diagnostic_scores
		WHERE run_id = ?
	`, runID).Scan(&score.RunID, &score.AuthenticityScore, &score.ProtocolScore, &score.SSEScore, &tags, &score.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询诊断评分失败: %w", err)
	}
	if tags != "" {
		score.Tags = json.RawMessage(tags)
	}
	return &score, nil
}

func (s *SQLiteStorage) SaveDiagnosticRunGroup(group *DiagnosticRunGroup) error {
	if group == nil {
		return fmt.Errorf("group is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_run_groups (group_id, candidate_run_id, baseline_run_id, baseline_mode, methodology_version, weights_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET
			candidate_run_id=excluded.candidate_run_id,
			baseline_run_id=excluded.baseline_run_id,
			baseline_mode=excluded.baseline_mode,
			methodology_version=excluded.methodology_version,
			weights_hash=excluded.weights_hash,
			created_at=excluded.created_at
	`, group.GroupID, group.CandidateRunID, group.BaselineRunID, group.BaselineMode, group.MethodologyVersion, group.WeightsHash, group.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断分组失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) SaveDiagnosticBaselineRun(baseline *DiagnosticBaselineRun) error {
	if baseline == nil {
		return fmt.Errorf("baseline is nil")
	}
	ctx := s.effectiveCtx()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_baseline_runs (baseline_id, service, model_family, run_id, provider, channel, source, methodology_version, captured_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			baseline_id=excluded.baseline_id,
			service=excluded.service,
			model_family=excluded.model_family,
			provider=excluded.provider,
			channel=excluded.channel,
			source=excluded.source,
			methodology_version=excluded.methodology_version,
			captured_at=excluded.captured_at
	`, baseline.BaselineID, baseline.Service, baseline.ModelFamily, baseline.RunID, baseline.Provider, baseline.Channel, baseline.Source, baseline.MethodologyVersion, baseline.CapturedAt)
	if err != nil {
		return fmt.Errorf("保存 baseline run 失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if id > 0 {
		baseline.ID = id
	}
	return nil
}

func (s *SQLiteStorage) GetLatestDiagnosticBaselineRun(service, modelFamily, methodologyVersion, excludeRunID string) (*DiagnosticBaselineRun, error) {
	ctx := s.effectiveCtx()
	var (
		baseline DiagnosticBaselineRun
		args     []any
	)
	query := `
		SELECT id, baseline_id, service, model_family, run_id, provider, channel, source, methodology_version, captured_at
		FROM diagnostic_baseline_runs
		WHERE service = ? AND model_family = ? AND methodology_version = ?
	`
	args = append(args, service, modelFamily, methodologyVersion)
	if strings.TrimSpace(excludeRunID) != "" {
		query += ` AND run_id != ?`
		args = append(args, excludeRunID)
	}
	query += ` ORDER BY captured_at DESC, id DESC LIMIT 1`
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&baseline.ID,
		&baseline.BaselineID,
		&baseline.Service,
		&baseline.ModelFamily,
		&baseline.RunID,
		&baseline.Provider,
		&baseline.Channel,
		&baseline.Source,
		&baseline.MethodologyVersion,
		&baseline.CapturedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询最新 baseline run 失败: %w", err)
	}
	return &baseline, nil
}

func (s *SQLiteStorage) GetDiagnosticRunGroup(groupID string) (*DiagnosticRunGroup, error) {
	ctx := s.effectiveCtx()
	var group DiagnosticRunGroup
	err := s.db.QueryRowContext(ctx, `
		SELECT group_id, candidate_run_id, baseline_run_id, baseline_mode, methodology_version, weights_hash, created_at
		FROM diagnostic_run_groups
		WHERE group_id = ?
	`, groupID).Scan(&group.GroupID, &group.CandidateRunID, &group.BaselineRunID, &group.BaselineMode, &group.MethodologyVersion, &group.WeightsHash, &group.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询诊断分组失败: %w", err)
	}
	return &group, nil
}

func (s *SQLiteStorage) SaveDiagnosticDimension(dimension *DiagnosticDimension) error {
	if dimension == nil {
		return fmt.Errorf("dimension is nil")
	}
	ctx := s.effectiveCtx()
	evidence := ""
	if len(dimension.Evidence) > 0 {
		evidence = string(dimension.Evidence)
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO diagnostic_dimensions (run_id, dimension_key, weight, score, normalized_score, status, reason, evidence_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, dimension_key) DO UPDATE SET
			weight=excluded.weight,
			score=excluded.score,
			normalized_score=excluded.normalized_score,
			status=excluded.status,
			reason=excluded.reason,
			evidence_json=excluded.evidence_json,
			created_at=excluded.created_at
	`, dimension.RunID, dimension.DimensionKey, dimension.Weight, dimension.Score, dimension.NormalizedScore, dimension.Status, dimension.Reason, evidence, dimension.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断维度失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if id > 0 {
		dimension.ID = id
	}
	return nil
}

func (s *SQLiteStorage) ListDiagnosticDimensions(runID string) ([]*DiagnosticDimension, error) {
	ctx := s.effectiveCtx()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, dimension_key, weight, score, normalized_score, status, reason, evidence_json, created_at
		FROM diagnostic_dimensions
		WHERE run_id = ?
		ORDER BY weight DESC, dimension_key ASC, id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("查询诊断维度失败: %w", err)
	}
	defer rows.Close()

	var out []*DiagnosticDimension
	for rows.Next() {
		var (
			dimension DiagnosticDimension
			evidence  string
		)
		if err := rows.Scan(&dimension.ID, &dimension.RunID, &dimension.DimensionKey, &dimension.Weight, &dimension.Score, &dimension.NormalizedScore, &dimension.Status, &dimension.Reason, &evidence, &dimension.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描诊断维度失败: %w", err)
		}
		if evidence != "" {
			dimension.Evidence = json.RawMessage(evidence)
		}
		out = append(out, &dimension)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历诊断维度失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) ReplaceAuditTargets(targets []AuditTarget) error {
	ctx := s.effectiveCtx()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始替换审计目标事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existingKeys := map[string]string{}
	rows, err := tx.QueryContext(ctx, `
		SELECT provider, service, channel, model, api_key
		FROM audit_targets
		WHERE api_key != ''
	`)
	if err != nil {
		return fmt.Errorf("读取已有审计目标凭证失败: %w", err)
	}
	for rows.Next() {
		var provider, service, channel, model, apiKey string
		if err := rows.Scan(&provider, &service, &channel, &model, &apiKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("扫描已有审计目标凭证失败: %w", err)
		}
		existingKeys[auditTargetKey(provider, service, channel, model)] = apiKey
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭已有审计目标凭证查询失败: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历已有审计目标凭证失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM audit_targets`); err != nil {
		return fmt.Errorf("清空审计目标失败: %w", err)
	}
	for _, target := range targets {
		if strings.TrimSpace(target.APIKey) == "" {
			target.APIKey = existingKeys[auditTargetKey(target.Provider, target.Service, target.Channel, target.Model)]
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_targets (provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, target.Provider, target.Service, target.Channel, target.Model, target.RequestModel, target.Group, target.Weight, target.Priority, target.Enabled, target.APIKey)
		if err != nil {
			return fmt.Errorf("写入审计目标失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交审计目标事务失败: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) ListAuditTargets() ([]AuditTarget, error) {
	ctx := s.effectiveCtx()
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key
		FROM audit_targets
		ORDER BY provider, service, channel, model
	`)
	if err != nil {
		return nil, fmt.Errorf("查询审计目标失败: %w", err)
	}
	defer rows.Close()

	var out []AuditTarget
	for rows.Next() {
		var target AuditTarget
		var enabled int
		if err := rows.Scan(&target.Provider, &target.Service, &target.Channel, &target.Model, &target.RequestModel, &target.Group, &target.Weight, &target.Priority, &enabled, &target.APIKey); err != nil {
			return nil, fmt.Errorf("扫描审计目标失败: %w", err)
		}
		target.Enabled = enabled != 0
		out = append(out, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历审计目标失败: %w", err)
	}
	return out, nil
}

func (s *SQLiteStorage) SetAuditTargetCredential(provider, service, channel, apiKey string) (*AuditTargetCredentialUpdate, error) {
	ctx := s.effectiveCtx()
	provider = strings.TrimSpace(provider)
	service = strings.TrimSpace(service)
	channel = strings.TrimSpace(channel)
	apiKey = strings.TrimSpace(apiKey)
	if provider == "" || service == "" || channel == "" {
		return nil, fmt.Errorf("provider/service/channel 不能为空")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE audit_targets
		SET api_key = ?
		WHERE provider = ? AND service = ? AND channel = ?
	`, apiKey, provider, service, channel)
	if err != nil {
		return nil, fmt.Errorf("更新渠道凭证失败: %w", err)
	}
	n, _ := res.RowsAffected()
	return &AuditTargetCredentialUpdate{
		Provider:      provider,
		Service:       service,
		Channel:       channel,
		Updated:       int(n),
		KeyConfigured: apiKey != "",
		KeyLast4:      last4(apiKey),
	}, nil
}

func (s *SQLiteStorage) ClearAuditTargetCredential(provider, service, channel string) (*AuditTargetCredentialUpdate, error) {
	ctx := s.effectiveCtx()
	provider = strings.TrimSpace(provider)
	service = strings.TrimSpace(service)
	channel = strings.TrimSpace(channel)
	if provider == "" || service == "" || channel == "" {
		return nil, fmt.Errorf("provider/service/channel 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE audit_targets
		SET api_key = ''
		WHERE provider = ? AND service = ? AND channel = ?
	`, provider, service, channel)
	if err != nil {
		return nil, fmt.Errorf("清除渠道凭证失败: %w", err)
	}
	n, _ := res.RowsAffected()
	return &AuditTargetCredentialUpdate{
		Provider:      provider,
		Service:       service,
		Channel:       channel,
		Updated:       int(n),
		KeyConfigured: false,
		KeyLast4:      "",
	}, nil
}

func (s *PostgresStorage) SaveChannelSnapshot(snapshot *ChannelSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	ctx := s.effectiveCtx()
	var raw any = []byte("{}")
	if len(snapshot.Raw) > 0 {
		raw = snapshot.Raw
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO newapi_channel_snapshots (newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT(newapi_channel_id, snapshot_at) DO UPDATE SET
			provider=EXCLUDED.provider,
			service=EXCLUDED.service,
			channel=EXCLUDED.channel,
			model=EXCLUDED.model,
			enabled=EXCLUDED.enabled,
			raw=EXCLUDED.raw
	`, snapshot.NewAPIChannelID, snapshot.SnapshotAt, snapshot.Provider, snapshot.Service, snapshot.Channel, snapshot.Model, snapshot.Enabled, raw)
	if err != nil {
		return fmt.Errorf("保存渠道快照失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetLatestChannelSnapshotStats() (*ChannelSnapshotStats, error) {
	ctx := s.effectiveCtx()
	var latest sql.NullInt64
	if err := s.pool.QueryRow(ctx, `SELECT MAX(snapshot_at) FROM newapi_channel_snapshots`).Scan(&latest); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照时间失败 (PostgreSQL): %w", err)
	}
	if !latest.Valid {
		return nil, nil
	}
	stats := &ChannelSnapshotStats{SnapshotAt: latest.Int64}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled THEN 1 ELSE 0 END), 0)
		FROM newapi_channel_snapshots
		WHERE snapshot_at = $1
	`, latest.Int64).Scan(&stats.ChannelCount, &stats.EnabledCount); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照统计失败 (PostgreSQL): %w", err)
	}
	return stats, nil
}

func (s *PostgresStorage) ListLatestChannelSnapshots() ([]ChannelSnapshot, error) {
	ctx := s.effectiveCtx()
	var latest sql.NullInt64
	if err := s.pool.QueryRow(ctx, `SELECT MAX(snapshot_at) FROM newapi_channel_snapshots`).Scan(&latest); err != nil {
		return nil, fmt.Errorf("查询最新渠道快照时间失败 (PostgreSQL): %w", err)
	}
	if !latest.Valid {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw
		FROM newapi_channel_snapshots
		WHERE snapshot_at = $1
		ORDER BY provider, service, channel, newapi_channel_id
	`, latest.Int64)
	if err != nil {
		return nil, fmt.Errorf("查询最新渠道快照列表失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()

	out := make([]ChannelSnapshot, 0)
	for rows.Next() {
		var snap ChannelSnapshot
		var enabled bool
		var raw []byte
		if err := rows.Scan(&snap.ID, &snap.NewAPIChannelID, &snap.SnapshotAt, &snap.Provider, &snap.Service, &snap.Channel, &snap.Model, &enabled, &raw); err != nil {
			return nil, fmt.Errorf("扫描最新渠道快照失败 (PostgreSQL): %w", err)
		}
		snap.Enabled = enabled
		if len(raw) > 0 {
			snap.Raw = json.RawMessage(raw)
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最新渠道快照失败 (PostgreSQL): %w", err)
	}
	return out, nil
}

func (s *PostgresStorage) GetChannelSnapshot(newAPIChannelID, snapshotAt int64) (*ChannelSnapshot, error) {
	ctx := s.effectiveCtx()
	var snap ChannelSnapshot
	var enabled bool
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, newapi_channel_id, snapshot_at, provider, service, channel, model, enabled, raw
		FROM newapi_channel_snapshots
		WHERE newapi_channel_id = $1 AND snapshot_at = $2
	`, newAPIChannelID, snapshotAt).Scan(&snap.ID, &snap.NewAPIChannelID, &snap.SnapshotAt, &snap.Provider, &snap.Service, &snap.Channel, &snap.Model, &enabled, &raw)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, fmt.Errorf("查询渠道快照失败 (PostgreSQL): %w", err)
	}
	snap.Enabled = enabled
	if len(raw) > 0 {
		snap.Raw = json.RawMessage(raw)
	}
	return &snap, nil
}

func (s *PostgresStorage) UpsertLogSyncCursor(cursor *LogSyncCursor) error {
	if cursor == nil {
		return fmt.Errorf("cursor is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO newapi_log_sync_cursors (name, last_id, last_time, updated_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT(name) DO UPDATE SET
			last_id=EXCLUDED.last_id,
			last_time=EXCLUDED.last_time,
			updated_at=EXCLUDED.updated_at
	`, cursor.Name, cursor.LastID, cursor.LastTime, cursor.UpdatedAt)
	if err != nil {
		return fmt.Errorf("更新日志游标失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetLogSyncCursor(name string) (*LogSyncCursor, error) {
	ctx := s.effectiveCtx()
	var c LogSyncCursor
	err := s.pool.QueryRow(ctx, `
		SELECT name, last_id, last_time, updated_at
		FROM newapi_log_sync_cursors
		WHERE name = $1
	`, name).Scan(&c.Name, &c.LastID, &c.LastTime, &c.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, fmt.Errorf("查询日志游标失败 (PostgreSQL): %w", err)
	}
	return &c, nil
}

func (s *PostgresStorage) SaveNewAPILogs(logs []NewAPILog) error {
	if len(logs) == 0 {
		return nil
	}
	ctx := s.effectiveCtx()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开始保存 new-api 日志事务失败 (PostgreSQL): %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, log := range logs {
		var raw any = []byte("{}")
		if len(log.Other) > 0 {
			raw = log.Other
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO newapi_logs (id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, channel_id, "group", request_id, upstream_request_id, other)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT(id) DO UPDATE SET
				created_at=EXCLUDED.created_at,
				type=EXCLUDED.type,
				content=EXCLUDED.content,
				model_name=EXCLUDED.model_name,
				quota=EXCLUDED.quota,
				prompt_tokens=EXCLUDED.prompt_tokens,
				completion_tokens=EXCLUDED.completion_tokens,
				use_time=EXCLUDED.use_time,
				is_stream=EXCLUDED.is_stream,
				channel_id=EXCLUDED.channel_id,
				"group"=EXCLUDED."group",
				request_id=EXCLUDED.request_id,
				upstream_request_id=EXCLUDED.upstream_request_id,
				other=EXCLUDED.other
		`, log.ID, log.CreatedAt, log.Type, log.Content, log.ModelName, log.Quota, log.PromptTokens, log.CompletionTokens, log.UseTime, log.IsStream, log.ChannelID, log.Group, log.RequestID, log.UpstreamRequestID, raw); err != nil {
			return fmt.Errorf("写入 new-api 日志失败 (PostgreSQL): %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交 new-api 日志事务失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListNewAPILogsSince(since int64) ([]NewAPILog, error) {
	ctx := s.effectiveCtx()
	rows, err := s.pool.Query(ctx, `
		SELECT id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, channel_id, "group", request_id, upstream_request_id, other
		FROM newapi_logs
		WHERE created_at >= $1
		ORDER BY created_at DESC, id DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("查询 new-api 日志失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()

	var out []NewAPILog
	for rows.Next() {
		var log NewAPILog
		var raw []byte
		if err := rows.Scan(&log.ID, &log.CreatedAt, &log.Type, &log.Content, &log.ModelName, &log.Quota, &log.PromptTokens, &log.CompletionTokens, &log.UseTime, &log.IsStream, &log.ChannelID, &log.Group, &log.RequestID, &log.UpstreamRequestID, &raw); err != nil {
			return nil, fmt.Errorf("扫描 new-api 日志失败 (PostgreSQL): %w", err)
		}
		if len(raw) > 0 {
			log.Other = json.RawMessage(raw)
		}
		out = append(out, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 new-api 日志失败 (PostgreSQL): %w", err)
	}
	return out, nil
}

func (s *PostgresStorage) SaveDiagnosticRun(run *DiagnosticRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO diagnostic_runs (run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(run_id) DO UPDATE SET
			provider=EXCLUDED.provider,
			service=EXCLUDED.service,
			channel=EXCLUDED.channel,
			model=EXCLUDED.model,
			status=EXCLUDED.status,
			created_at=EXCLUDED.created_at,
			updated_at=EXCLUDED.updated_at,
			input_json=EXCLUDED.input_json,
			output_json=EXCLUDED.output_json
	`, run.RunID, run.Provider, run.Service, run.Channel, run.Model, run.Status, run.CreatedAt, run.UpdatedAt, run.Input, run.Output)
	if err != nil {
		return fmt.Errorf("保存诊断任务失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetDiagnosticRun(runID string) (*DiagnosticRun, error) {
	ctx := s.effectiveCtx()
	var run DiagnosticRun
	var input, output []byte
	err := s.pool.QueryRow(ctx, `
		SELECT run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json
		FROM diagnostic_runs
		WHERE run_id = $1
	`, runID).Scan(&run.RunID, &run.Provider, &run.Service, &run.Channel, &run.Model, &run.Status, &run.CreatedAt, &run.UpdatedAt, &input, &output)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, fmt.Errorf("查询诊断任务失败 (PostgreSQL): %w", err)
	}
	if len(input) > 0 {
		run.Input = json.RawMessage(input)
	}
	if len(output) > 0 {
		run.Output = json.RawMessage(output)
	}
	return &run, nil
}

func (s *PostgresStorage) ListDiagnosticRuns(filter DiagnosticRunFilter) ([]*DiagnosticRun, error) {
	ctx := s.effectiveCtx()
	clauses, args, argIndex := postgresDiagnosticRunWhere(filter)
	query := `
		SELECT run_id, provider, service, channel, model, status, created_at, updated_at, input_json, output_json
		FROM diagnostic_runs
	`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	query += ` ORDER BY created_at DESC, updated_at DESC, run_id DESC`
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询诊断任务列表失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()

	out := make([]*DiagnosticRun, 0, limit)
	for rows.Next() {
		var (
			run           DiagnosticRun
			input, output string
		)
		if err := rows.Scan(&run.RunID, &run.Provider, &run.Service, &run.Channel, &run.Model, &run.Status, &run.CreatedAt, &run.UpdatedAt, &input, &output); err != nil {
			return nil, fmt.Errorf("扫描诊断任务列表失败 (PostgreSQL): %w", err)
		}
		if input != "" {
			run.Input = json.RawMessage(input)
		}
		if output != "" {
			run.Output = json.RawMessage(output)
		}
		out = append(out, &run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历诊断任务列表失败 (PostgreSQL): %w", err)
	}
	return out, nil
}

func (s *PostgresStorage) CountDiagnosticRunsFiltered(filter DiagnosticRunFilter) (int, error) {
	ctx := s.effectiveCtx()
	clauses, args, _ := postgresDiagnosticRunWhere(filter)
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	var count int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务列表失败 (PostgreSQL): %w", err)
	}
	return count, nil
}

func (s *PostgresStorage) CountDiagnosticRuns(status string) (int, error) {
	ctx := s.effectiveCtx()
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	args := make([]any, 0, 1)
	if strings.TrimSpace(status) != "" {
		query += ` WHERE status = $1`
		args = append(args, strings.TrimSpace(status))
	}
	var count int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务失败 (PostgreSQL): %w", err)
	}
	return count, nil
}

func (s *PostgresStorage) GetDiagnosticDimensionSummary() (*DiagnosticDimensionSummary, error) {
	ctx := s.effectiveCtx()
	var summary DiagnosticDimensionSummary
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT diagnostic_dimensions.run_id), COUNT(*)
		FROM diagnostic_dimensions
		INNER JOIN diagnostic_runs ON diagnostic_runs.run_id = diagnostic_dimensions.run_id
		WHERE diagnostic_runs.status = 'done'
	`).Scan(&summary.RunCount, &summary.DimensionCount); err != nil {
		return nil, fmt.Errorf("统计诊断维度失败 (PostgreSQL): %w", err)
	}
	return &summary, nil
}

func (s *PostgresStorage) SaveDiagnosticStep(step *DiagnosticStep) error {
	if step == nil {
		return fmt.Errorf("step is nil")
	}
	ctx := s.effectiveCtx()
	var meta any = []byte("{}")
	if len(step.ExecutionMeta) > 0 {
		meta = step.ExecutionMeta
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO diagnostic_steps (run_id, step_index, prompt, resolved_prompt, response_preview, result_summary, execution_meta, channel_fingerprint, provider_fingerprint, error_message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT(run_id, step_index) DO UPDATE SET
			prompt=EXCLUDED.prompt,
			resolved_prompt=EXCLUDED.resolved_prompt,
			response_preview=EXCLUDED.response_preview,
			result_summary=EXCLUDED.result_summary,
			execution_meta=EXCLUDED.execution_meta,
			channel_fingerprint=EXCLUDED.channel_fingerprint,
			provider_fingerprint=EXCLUDED.provider_fingerprint,
			error_message=EXCLUDED.error_message,
			created_at=EXCLUDED.created_at
	`, step.RunID, step.StepIndex, step.Prompt, step.ResolvedPrompt, step.ResponsePreview, step.ResultSummary, meta, step.ChannelFingerprint, step.ProviderFingerprint, step.ErrorMessage, step.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断步骤失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListDiagnosticSteps(runID string) ([]*DiagnosticStep, error) {
	ctx := s.effectiveCtx()
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, step_index, prompt, resolved_prompt, response_preview, result_summary, execution_meta, channel_fingerprint, provider_fingerprint, error_message, created_at
		FROM diagnostic_steps
		WHERE run_id = $1
		ORDER BY step_index ASC, id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("查询诊断步骤失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()
	var out []*DiagnosticStep
	for rows.Next() {
		var step DiagnosticStep
		var raw []byte
		if err := rows.Scan(&step.ID, &step.RunID, &step.StepIndex, &step.Prompt, &step.ResolvedPrompt, &step.ResponsePreview, &step.ResultSummary, &raw, &step.ChannelFingerprint, &step.ProviderFingerprint, &step.ErrorMessage, &step.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描诊断步骤失败 (PostgreSQL): %w", err)
		}
		if len(raw) > 0 {
			step.ExecutionMeta = json.RawMessage(raw)
		}
		out = append(out, &step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历诊断步骤失败 (PostgreSQL): %w", err)
	}
	return out, nil
}

func (s *PostgresStorage) SaveDiagnosticScore(score *DiagnosticScore) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}
	ctx := s.effectiveCtx()
	var tags any = []byte("[]")
	if len(score.Tags) > 0 {
		tags = score.Tags
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO diagnostic_scores (run_id, authenticity_score, protocol_score, sse_score, tags, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT(run_id) DO UPDATE SET
			authenticity_score=EXCLUDED.authenticity_score,
			protocol_score=EXCLUDED.protocol_score,
			sse_score=EXCLUDED.sse_score,
			tags=EXCLUDED.tags,
			created_at=EXCLUDED.created_at
	`, score.RunID, score.AuthenticityScore, score.ProtocolScore, score.SSEScore, tags, score.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断评分失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetDiagnosticScore(runID string) (*DiagnosticScore, error) {
	ctx := s.effectiveCtx()
	var score DiagnosticScore
	var tags []byte
	err := s.pool.QueryRow(ctx, `
		SELECT run_id, authenticity_score, protocol_score, sse_score, tags, created_at
		FROM diagnostic_scores
		WHERE run_id = $1
	`, runID).Scan(&score.RunID, &score.AuthenticityScore, &score.ProtocolScore, &score.SSEScore, &tags, &score.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, fmt.Errorf("查询诊断评分失败 (PostgreSQL): %w", err)
	}
	if len(tags) > 0 {
		score.Tags = json.RawMessage(tags)
	}
	return &score, nil
}

func (s *PostgresStorage) SaveDiagnosticRunGroup(group *DiagnosticRunGroup) error {
	if group == nil {
		return fmt.Errorf("group is nil")
	}
	ctx := s.effectiveCtx()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO diagnostic_run_groups (group_id, candidate_run_id, baseline_run_id, baseline_mode, methodology_version, weights_hash, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(group_id) DO UPDATE SET
			candidate_run_id=EXCLUDED.candidate_run_id,
			baseline_run_id=EXCLUDED.baseline_run_id,
			baseline_mode=EXCLUDED.baseline_mode,
			methodology_version=EXCLUDED.methodology_version,
			weights_hash=EXCLUDED.weights_hash,
			created_at=EXCLUDED.created_at
	`, group.GroupID, group.CandidateRunID, group.BaselineRunID, group.BaselineMode, group.MethodologyVersion, group.WeightsHash, group.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存诊断分组失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) SaveDiagnosticBaselineRun(baseline *DiagnosticBaselineRun) error {
	if baseline == nil {
		return fmt.Errorf("baseline is nil")
	}
	ctx := s.effectiveCtx()
	err := s.pool.QueryRow(ctx, `
		INSERT INTO diagnostic_baseline_runs (baseline_id, service, model_family, run_id, provider, channel, source, methodology_version, captured_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(run_id) DO UPDATE SET
			baseline_id=EXCLUDED.baseline_id,
			service=EXCLUDED.service,
			model_family=EXCLUDED.model_family,
			provider=EXCLUDED.provider,
			channel=EXCLUDED.channel,
			source=EXCLUDED.source,
			methodology_version=EXCLUDED.methodology_version,
			captured_at=EXCLUDED.captured_at
		RETURNING id
	`, baseline.BaselineID, baseline.Service, baseline.ModelFamily, baseline.RunID, baseline.Provider, baseline.Channel, baseline.Source, baseline.MethodologyVersion, baseline.CapturedAt).Scan(&baseline.ID)
	if err != nil {
		return fmt.Errorf("保存 baseline run 失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) GetLatestDiagnosticBaselineRun(service, modelFamily, methodologyVersion, excludeRunID string) (*DiagnosticBaselineRun, error) {
	ctx := s.effectiveCtx()
	var (
		baseline DiagnosticBaselineRun
		args     []any
	)
	query := `
		SELECT id, baseline_id, service, model_family, run_id, provider, channel, source, methodology_version, captured_at
		FROM diagnostic_baseline_runs
		WHERE service = $1 AND model_family = $2 AND methodology_version = $3
	`
	args = append(args, service, modelFamily, methodologyVersion)
	argPos := 4
	if strings.TrimSpace(excludeRunID) != "" {
		query += fmt.Sprintf(" AND run_id != $%d", argPos)
		args = append(args, excludeRunID)
		argPos++
	}
	query += ` ORDER BY captured_at DESC, id DESC LIMIT 1`
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&baseline.ID,
		&baseline.BaselineID,
		&baseline.Service,
		&baseline.ModelFamily,
		&baseline.RunID,
		&baseline.Provider,
		&baseline.Channel,
		&baseline.Source,
		&baseline.MethodologyVersion,
		&baseline.CapturedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询最新 baseline run 失败 (PostgreSQL): %w", err)
	}
	return &baseline, nil
}

func (s *PostgresStorage) GetDiagnosticRunGroup(groupID string) (*DiagnosticRunGroup, error) {
	ctx := s.effectiveCtx()
	var group DiagnosticRunGroup
	err := s.pool.QueryRow(ctx, `
		SELECT group_id, candidate_run_id, baseline_run_id, baseline_mode, methodology_version, weights_hash, created_at
		FROM diagnostic_run_groups
		WHERE group_id = $1
	`, groupID).Scan(&group.GroupID, &group.CandidateRunID, &group.BaselineRunID, &group.BaselineMode, &group.MethodologyVersion, &group.WeightsHash, &group.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询诊断分组失败 (PostgreSQL): %w", err)
	}
	return &group, nil
}

func (s *PostgresStorage) SaveDiagnosticDimension(dimension *DiagnosticDimension) error {
	if dimension == nil {
		return fmt.Errorf("dimension is nil")
	}
	ctx := s.effectiveCtx()
	evidence := []byte("{}")
	if len(dimension.Evidence) > 0 {
		evidence = dimension.Evidence
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO diagnostic_dimensions (run_id, dimension_key, weight, score, normalized_score, status, reason, evidence_json, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(run_id, dimension_key) DO UPDATE SET
			weight=EXCLUDED.weight,
			score=EXCLUDED.score,
			normalized_score=EXCLUDED.normalized_score,
			status=EXCLUDED.status,
			reason=EXCLUDED.reason,
			evidence_json=EXCLUDED.evidence_json,
			created_at=EXCLUDED.created_at
		RETURNING id
	`, dimension.RunID, dimension.DimensionKey, dimension.Weight, dimension.Score, dimension.NormalizedScore, dimension.Status, dimension.Reason, evidence, dimension.CreatedAt).Scan(&dimension.ID)
	if err != nil {
		return fmt.Errorf("保存诊断维度失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListDiagnosticDimensions(runID string) ([]*DiagnosticDimension, error) {
	ctx := s.effectiveCtx()
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, dimension_key, weight, score, normalized_score, status, reason, evidence_json, created_at
		FROM diagnostic_dimensions
		WHERE run_id = $1
		ORDER BY weight DESC, dimension_key ASC, id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("查询诊断维度失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()

	var out []*DiagnosticDimension
	for rows.Next() {
		var dimension DiagnosticDimension
		if err := rows.Scan(&dimension.ID, &dimension.RunID, &dimension.DimensionKey, &dimension.Weight, &dimension.Score, &dimension.NormalizedScore, &dimension.Status, &dimension.Reason, &dimension.Evidence, &dimension.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描诊断维度失败 (PostgreSQL): %w", err)
		}
		out = append(out, &dimension)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("遍历诊断维度失败 (PostgreSQL): %w", rows.Err())
	}
	return out, nil
}

func (s *PostgresStorage) ReplaceAuditTargets(targets []AuditTarget) error {
	ctx := s.effectiveCtx()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开始替换审计目标事务失败 (PostgreSQL): %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existingKeys := map[string]string{}
	rows, err := tx.Query(ctx, `
		SELECT provider, service, channel, model, api_key
		FROM audit_targets
		WHERE api_key <> ''
	`)
	if err != nil {
		return fmt.Errorf("读取已有审计目标凭证失败 (PostgreSQL): %w", err)
	}
	for rows.Next() {
		var provider, service, channel, model, apiKey string
		if err := rows.Scan(&provider, &service, &channel, &model, &apiKey); err != nil {
			rows.Close()
			return fmt.Errorf("扫描已有审计目标凭证失败 (PostgreSQL): %w", err)
		}
		existingKeys[auditTargetKey(provider, service, channel, model)] = apiKey
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历已有审计目标凭证失败 (PostgreSQL): %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM audit_targets`); err != nil {
		return fmt.Errorf("清空审计目标失败 (PostgreSQL): %w", err)
	}
	for _, target := range targets {
		if strings.TrimSpace(target.APIKey) == "" {
			target.APIKey = existingKeys[auditTargetKey(target.Provider, target.Service, target.Channel, target.Model)]
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_targets (provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, target.Provider, target.Service, target.Channel, target.Model, target.RequestModel, target.Group, target.Weight, target.Priority, target.Enabled, target.APIKey)
		if err != nil {
			return fmt.Errorf("写入审计目标失败 (PostgreSQL): %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交审计目标事务失败 (PostgreSQL): %w", err)
	}
	return nil
}

func (s *PostgresStorage) ListAuditTargets() ([]AuditTarget, error) {
	ctx := s.effectiveCtx()
	rows, err := s.pool.Query(ctx, `
		SELECT provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key
		FROM audit_targets
		ORDER BY provider, service, channel, model
	`)
	if err != nil {
		return nil, fmt.Errorf("查询审计目标失败 (PostgreSQL): %w", err)
	}
	defer rows.Close()

	var out []AuditTarget
	for rows.Next() {
		var target AuditTarget
		var enabled bool
		if err := rows.Scan(&target.Provider, &target.Service, &target.Channel, &target.Model, &target.RequestModel, &target.Group, &target.Weight, &target.Priority, &enabled, &target.APIKey); err != nil {
			return nil, fmt.Errorf("扫描审计目标失败 (PostgreSQL): %w", err)
		}
		target.Enabled = enabled
		out = append(out, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历审计目标失败 (PostgreSQL): %w", err)
	}
	return out, nil
}

func (s *PostgresStorage) SetAuditTargetCredential(provider, service, channel, apiKey string) (*AuditTargetCredentialUpdate, error) {
	ctx := s.effectiveCtx()
	provider = strings.TrimSpace(provider)
	service = strings.TrimSpace(service)
	channel = strings.TrimSpace(channel)
	apiKey = strings.TrimSpace(apiKey)
	if provider == "" || service == "" || channel == "" {
		return nil, fmt.Errorf("provider/service/channel 不能为空")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key 不能为空")
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE audit_targets
		SET api_key = $1
		WHERE provider = $2 AND service = $3 AND channel = $4
	`, apiKey, provider, service, channel)
	if err != nil {
		return nil, fmt.Errorf("更新渠道凭证失败 (PostgreSQL): %w", err)
	}
	return &AuditTargetCredentialUpdate{
		Provider:      provider,
		Service:       service,
		Channel:       channel,
		Updated:       int(ct.RowsAffected()),
		KeyConfigured: apiKey != "",
		KeyLast4:      last4(apiKey),
	}, nil
}

func (s *PostgresStorage) ClearAuditTargetCredential(provider, service, channel string) (*AuditTargetCredentialUpdate, error) {
	ctx := s.effectiveCtx()
	provider = strings.TrimSpace(provider)
	service = strings.TrimSpace(service)
	channel = strings.TrimSpace(channel)
	if provider == "" || service == "" || channel == "" {
		return nil, fmt.Errorf("provider/service/channel 不能为空")
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE audit_targets
		SET api_key = ''
		WHERE provider = $1 AND service = $2 AND channel = $3
	`, provider, service, channel)
	if err != nil {
		return nil, fmt.Errorf("清除渠道凭证失败 (PostgreSQL): %w", err)
	}
	return &AuditTargetCredentialUpdate{
		Provider:      provider,
		Service:       service,
		Channel:       channel,
		Updated:       int(ct.RowsAffected()),
		KeyConfigured: false,
		KeyLast4:      "",
	}, nil
}
