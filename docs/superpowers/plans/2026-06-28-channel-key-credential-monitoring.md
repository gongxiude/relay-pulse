# Channel Key Credential Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为从 `new-api` 同步并存入数据库的渠道增加渠道级 `api_key` 字段，并在渠道监测 / quick-probe / 模板补洞时使用数据库中该渠道自己的 key 发起请求。

**Architecture:** `new-api` 同步仍只负责读取渠道元数据；RelayPulse 本地 `audit_targets` 保存每个 `provider + service + channel + model` 的监测 key。同步渠道时必须保留已有 key，检测时禁止使用全局 `NEWAPI_ACCESS_TOKEN` 伪装渠道级检测；缺少 key 的渠道返回明确的 `missing_credential` 状态。

**Tech Stack:** Go, SQLite, PostgreSQL, Gin API, existing `internal/audit` quick-probe runner, existing template probe executor, existing storage interfaces.

---

## Execution Workspace Boundary

本 plan **必须在单独的 git worktree 中实现**，禁止直接在当前工作区编码。

执行前必须完成：

```bash
git worktree add ../relay-pulse-channel-keys -b channel-key-credential-monitoring
cd ../relay-pulse-channel-keys
git status --short --branch
```

停止条件：

- 如果 `../relay-pulse-channel-keys` 已存在，先执行 `git -C ../relay-pulse-channel-keys status --short --branch` 并报告状态。
- 如果 `channel-key-credential-monitoring` 分支已存在，先报告分支和 worktree 绑定关系，不要覆盖。
- 不移动、不清理、不回滚当前主工作区的未提交文件。

---

## Current State

当前代码的实际状态：

| 能力 | 当前实现 |
|---|---|
| 渠道同步 | `cmd/server/main.go` 启动定时同步，默认 5 分钟一次 |
| 同步入口 | `POST /api/audit/newapi/sync/channels` 手动触发 |
| 同步数据来源 | `new-api GET /api/channel/` |
| `audit_targets` 字段 | `provider/service/channel/model/request_model/group/weight/priority/enabled` |
| 检测凭证 | `resolveAuditProbeCredentials` 返回全局 `NEWAPI_PROBE_ACCESS_TOKEN` 或 fallback `NEWAPI_ACCESS_TOKEN` |
| 核心缺口 | 页面选中某个渠道后，检测请求没有使用该渠道自己的 key |

必须修正的同步风险：

`SQLiteStorage.ReplaceAuditTargets` 和 `PostgresStorage.ReplaceAuditTargets` 当前先 `DELETE FROM audit_targets`，再插入同步目标。新增 `api_key` 后，如果不先读取并保留旧 key，每次 5 分钟渠道同步都会清空人工录入的 key。

---

## Target Behavior

### Credential Rule

检测时只允许使用当前数据库目标的渠道级 key：

```text
target key = audit_targets.api_key where provider/service/channel/model match request
```

规则：

- `api_key` 非空：执行检测。
- `api_key` 为空：拒绝检测，返回 `missing_credential`。
- 不再 fallback 到 `NEWAPI_ACCESS_TOKEN` 做真实性检测。
- `NEWAPI_ACCESS_TOKEN` 只保留用于读取 `new-api` 渠道和日志。
- `NEWAPI_PROBE_ACCESS_TOKEN` 可保留为兼容配置，但不参与 new-api 同步渠道的正式检测路径。

### Key Storage Rule

第一版按 `provider + service + channel` 设置 key，并自动复制到该渠道下所有模型的 `audit_targets.api_key`。

原因：

- 页面上的服务商详情按渠道和模型展开。
- 同一 new-api 渠道下的多个模型应使用同一渠道 key。
- 检测请求最终仍是模型级，但凭证归属是渠道级。

### API Rule

新增只写凭证接口：

```http
PUT /api/audit/targets/credential
Content-Type: application/json

{
  "provider": "alan-官key直连",
  "service": "cc",
  "channel": "78:ClaudeCN-gpt",
  "api_key": "sk-xxx"
}
```

响应不能返回明文 key：

```json
{
  "success": true,
  "data": {
    "provider": "alan-官key直连",
    "service": "cc",
    "channel": "78:ClaudeCN-gpt",
    "updated": 23,
    "key_configured": true,
    "key_last4": "xxxx"
  }
}
```

新增清除凭证接口：

```http
DELETE /api/audit/targets/credential
Content-Type: application/json

{
  "provider": "alan-官key直连",
  "service": "cc",
  "channel": "78:ClaudeCN-gpt"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "provider": "alan-官key直连",
    "service": "cc",
    "channel": "78:ClaudeCN-gpt",
    "updated": 23,
    "key_configured": false,
    "key_last4": ""
  }
}
```

---

## File Structure

Modify:

- `internal/storage/audit_models.go`
  - Add `AuditTarget.APIKey string`.
  - Add `api_key` column to SQLite and PostgreSQL `audit_targets`.
  - Add migration for existing SQLite and PostgreSQL databases.
  - Preserve existing `api_key` during `ReplaceAuditTargets`.
  - Add store methods for setting / clearing / resolving target credentials.

- `internal/audit/targets.go`
  - Keep `BuildAuditTargets` credential-free.
  - Ensure sync-generated targets do not overwrite existing stored key.

- `internal/api/audit_types.go`
  - Add request / response types for credential update and clear.
  - Add non-sensitive credential status fields where needed.

- `internal/api/audit_handler.go`
  - Add handlers for setting and clearing channel credential.
  - Modify diagnostic submit and backfill to resolve target key from storage.
  - Modify template probe backfill to use target key.
  - Return `missing_credential` when key is absent.

- `internal/api/server.go`
  - Register credential routes.

- `internal/storage/audit_models_test.go`
  - Test migration, round-trip, and sync key preservation.

- `internal/newapi/sync_channels_test.go`
  - Test channel sync keeps previously stored key.

- `internal/api/audit_handler_test.go`
  - Test submit uses stored channel key.
  - Test missing key rejects diagnostic with `missing_credential`.
  - Test credential API never returns plaintext key.

Do not modify:

- `new-api` source code.
- `internal/newapi/client.go` GET-only behavior.
- Frontend pages in this plan. UI editing can be planned separately after backend semantics are correct.

---

## Task 1: Add `api_key` To AuditTarget Storage

**Files:**
- Modify: `internal/storage/audit_models.go`
- Test: `internal/storage/audit_models_test.go`

- [ ] **Step 1: Extend `AuditTarget`**

Add `APIKey` with `json:"-"` so API responses do not leak plaintext:

```go
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
```

- [ ] **Step 2: Add schema column**

SQLite `audit_targets` create statement must include:

```sql
api_key TEXT NOT NULL DEFAULT '',
```

PostgreSQL `audit_targets` create statement must include:

```sql
api_key TEXT NOT NULL DEFAULT '',
```

The column belongs before the primary key in both create statements.

- [ ] **Step 3: Add SQLite migration helper**

Add helper near existing audit table initialization helpers:

```go
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
```

Call it immediately after `initAuditTables(ctx)` succeeds in the SQLite init path used by `SQLiteStorage.Init`.

- [ ] **Step 4: Add PostgreSQL migration helper**

Add helper:

```go
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
```

Call it immediately after PostgreSQL `initAuditTables(ctx)` succeeds.

- [ ] **Step 5: Update `ReplaceAuditTargets` to preserve keys**

Before deleting `audit_targets`, read existing keys into a map keyed by `provider/service/channel/model`:

```go
func auditTargetKey(provider, service, channel, model string) string {
	return strings.Join([]string{
		strings.TrimSpace(provider),
		strings.TrimSpace(service),
		strings.TrimSpace(channel),
		strings.TrimSpace(model),
	}, "\x00")
}
```

SQLite preservation query:

```sql
SELECT provider, service, channel, model, api_key
FROM audit_targets
WHERE api_key != ''
```

PostgreSQL preservation query:

```sql
SELECT provider, service, channel, model, api_key
FROM audit_targets
WHERE api_key <> ''
```

When inserting sync-generated targets:

```go
if strings.TrimSpace(target.APIKey) == "" {
	target.APIKey = existingKeys[auditTargetKey(target.Provider, target.Service, target.Channel, target.Model)]
}
```

Update insert SQL:

SQLite:

```sql
INSERT INTO audit_targets (provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```

PostgreSQL:

```sql
INSERT INTO audit_targets (provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
```

- [ ] **Step 6: Update `ListAuditTargets`**

SQLite select:

```sql
SELECT provider, service, channel, model, request_model, "group", weight, priority, enabled, api_key
FROM audit_targets
ORDER BY provider, service, channel, model
```

SQLite scan:

```go
if err := rows.Scan(&target.Provider, &target.Service, &target.Channel, &target.Model, &target.RequestModel, &target.Group, &target.Weight, &target.Priority, &enabled, &target.APIKey); err != nil {
	return nil, fmt.Errorf("扫描审计目标失败: %w", err)
}
```

PostgreSQL select and scan mirror SQLite, with `enabled bool`.

- [ ] **Step 7: Add storage tests**

Add tests:

```go
func TestAuditTargetsPreserveAPIKeyOnReplace(t *testing.T) {
	store := newTestStore(t)

	first := []AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		APIKey:       "sk-channel-key",
	}}
	if err := store.ReplaceAuditTargets(first); err != nil {
		t.Fatalf("ReplaceAuditTargets first: %v", err)
	}

	second := []AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
	}}
	if err := store.ReplaceAuditTargets(second); err != nil {
		t.Fatalf("ReplaceAuditTargets second: %v", err)
	}

	targets, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(targets))
	}
	if targets[0].APIKey != "sk-channel-key" {
		t.Fatalf("APIKey = %q, want preserved key", targets[0].APIKey)
	}
}
```

- [ ] **Step 8: Run storage tests**

Run:

```bash
go test ./internal/storage
```

Expected: pass.

- [ ] **Step 9: Commit**

```bash
git add internal/storage/audit_models.go internal/storage/audit_models_test.go
git commit -m "feat: store audit target channel keys"
```

---

## Task 2: Add Credential Update Store Methods

**Files:**
- Modify: `internal/storage/audit_models.go`
- Test: `internal/storage/audit_models_test.go`

- [ ] **Step 1: Add credential result type**

Add:

```go
type AuditTargetCredentialUpdate struct {
	Provider      string `json:"provider"`
	Service       string `json:"service"`
	Channel       string `json:"channel"`
	Updated       int    `json:"updated"`
	KeyConfigured bool   `json:"key_configured"`
	KeyLast4      string `json:"key_last4"`
}
```

- [ ] **Step 2: Add helper for last4**

Add:

```go
func last4(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
```

- [ ] **Step 3: Add SQLite update method**

Add:

```go
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
```

- [ ] **Step 4: Add SQLite clear method**

Add:

```go
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
```

- [ ] **Step 5: Add PostgreSQL methods**

Implement the same method signatures on `PostgresStorage` with `$1/$2/$3/$4` placeholders:

```sql
UPDATE audit_targets
SET api_key = $1
WHERE provider = $2 AND service = $3 AND channel = $4
```

Clear:

```sql
UPDATE audit_targets
SET api_key = ''
WHERE provider = $1 AND service = $2 AND channel = $3
```

Return the same `AuditTargetCredentialUpdate` shape.

- [ ] **Step 6: Add tests**

Add:

```go
func TestAuditTargetCredentialUpdateAppliesToChannelModels(t *testing.T) {
	store := newTestStore(t)
	targets := []AuditTarget{
		{Provider: "p1", Service: "cc", Channel: "101:demo", Model: "m1", RequestModel: "m1", Enabled: true},
		{Provider: "p1", Service: "cc", Channel: "101:demo", Model: "m2", RequestModel: "m2", Enabled: true},
		{Provider: "p1", Service: "cc", Channel: "102:other", Model: "m1", RequestModel: "m1", Enabled: true},
	}
	if err := store.ReplaceAuditTargets(targets); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	result, err := store.SetAuditTargetCredential("p1", "cc", "101:demo", "sk-channel-key-1234")
	if err != nil {
		t.Fatalf("SetAuditTargetCredential: %v", err)
	}
	if result.Updated != 2 || !result.KeyConfigured || result.KeyLast4 != "1234" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	got, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	keys := map[string]string{}
	for _, target := range got {
		keys[target.Channel+"|"+target.Model] = target.APIKey
	}
	if keys["101:demo|m1"] != "sk-channel-key-1234" || keys["101:demo|m2"] != "sk-channel-key-1234" {
		t.Fatalf("channel keys not applied: %+v", keys)
	}
	if keys["102:other|m1"] != "" {
		t.Fatalf("other channel key should stay empty: %+v", keys)
	}
}
```

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./internal/storage
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add internal/storage/audit_models.go internal/storage/audit_models_test.go
git commit -m "feat: manage audit target credentials"
```

---

## Task 3: Add Credential API

**Files:**
- Modify: `internal/api/audit_types.go`
- Modify: `internal/api/audit_handler.go`
- Modify: `internal/api/server.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add API types**

Add:

```go
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
```

- [ ] **Step 2: Add handler store interface**

Add:

```go
type auditCredentialStore interface {
	SetAuditTargetCredential(provider, service, channel, apiKey string) (*storage.AuditTargetCredentialUpdate, error)
	ClearAuditTargetCredential(provider, service, channel string) (*storage.AuditTargetCredentialUpdate, error)
}
```

- [ ] **Step 3: Add set handler**

Add:

```go
func (h *Handler) PutAuditTargetCredential(c *gin.Context) {
	store, ok := h.storage.(auditCredentialStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持渠道凭证")
		return
	}
	var req auditTargetCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "请求参数错误")
		return
	}
	result, err := store.SetAuditTargetCredential(req.Provider, req.Service, req.Channel, req.APIKey)
	if err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": auditTargetCredentialResponse{
		Provider:      result.Provider,
		Service:       result.Service,
		Channel:       result.Channel,
		Updated:       result.Updated,
		KeyConfigured: result.KeyConfigured,
		KeyLast4:      result.KeyLast4,
	}})
}
```

- [ ] **Step 4: Add clear handler**

Add:

```go
func (h *Handler) DeleteAuditTargetCredential(c *gin.Context) {
	store, ok := h.storage.(auditCredentialStore)
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持渠道凭证")
		return
	}
	var req auditTargetCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "请求参数错误")
		return
	}
	result, err := store.ClearAuditTargetCredential(req.Provider, req.Service, req.Channel)
	if err != nil {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": auditTargetCredentialResponse{
		Provider:      result.Provider,
		Service:       result.Service,
		Channel:       result.Channel,
		Updated:       result.Updated,
		KeyConfigured: result.KeyConfigured,
		KeyLast4:      result.KeyLast4,
	}})
}
```

- [ ] **Step 5: Register routes**

Add to `internal/api/server.go`:

```go
router.PUT("/api/audit/targets/credential", handler.PutAuditTargetCredential)
router.DELETE("/api/audit/targets/credential", handler.DeleteAuditTargetCredential)
```

- [ ] **Step 6: Add API tests**

Add:

```go
func TestAuditTargetCredentialAPIHidesPlaintextKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "m1",
		RequestModel: "m1",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	body := `{"provider":"p1","service":"cc","channel":"101:demo","api_key":"sk-secret-1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/audit/targets/credential", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("credential update unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret-1234") {
		t.Fatalf("response leaked plaintext key: %s", rec.Body.String())
	}
	if !containsJSON(rec.Body.String(), `"key_last4":"1234"`) {
		t.Fatalf("response should expose key_last4 only: %s", rec.Body.String())
	}
}
```

- [ ] **Step 7: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add internal/api/audit_types.go internal/api/audit_handler.go internal/api/server.go internal/api/audit_handler_test.go
git commit -m "feat: add audit target credential API"
```

---

## Task 4: Use Stored Channel Key For Quick-Probe Diagnostics

**Files:**
- Modify: `internal/api/audit_handler.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add target resolver helper**

Add:

```go
func findAuditTarget(targets []storage.AuditTarget, provider, service, channel, model string) (*storage.AuditTarget, bool) {
	provider = strings.TrimSpace(provider)
	service = strings.TrimSpace(service)
	channel = strings.TrimSpace(channel)
	model = strings.TrimSpace(model)
	for i := range targets {
		target := &targets[i]
		if target.Provider == provider && target.Service == service && target.Channel == channel && target.Model == model {
			return target, true
		}
	}
	return nil, false
}
```

- [ ] **Step 2: Add credential resolver helper**

Add:

```go
func resolveStoredAuditTargetCredential(store auditReadStore, provider, service, channel, model string) (*storage.AuditTarget, error) {
	targets, err := store.ListAuditTargets()
	if err != nil {
		return nil, err
	}
	target, ok := findAuditTarget(targets, provider, service, channel, model)
	if !ok {
		return nil, fmt.Errorf("audit target not found")
	}
	if strings.TrimSpace(target.APIKey) == "" {
		return nil, fmt.Errorf("missing_credential")
	}
	return target, nil
}
```

- [ ] **Step 3: Modify `PostAuditDiagnosticSubmit`**

After parsing `target`, resolve stored target:

```go
storedTarget, err := resolveStoredAuditTargetCredential(store, target.Provider, target.Service, target.Channel, target.Model)
if err != nil {
	if err.Error() == "missing_credential" {
		apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "missing_credential: 当前渠道未配置监测 key")
		return
	}
	apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, err.Error())
	return
}
target.AccessToken = strings.TrimSpace(storedTarget.APIKey)
```

Keep:

```go
target.BaseURL = strings.TrimSpace(appCfg.NewAPI.BaseURL)
target.UserID = strings.TrimSpace(appCfg.NewAPI.UserID)
```

Remove use of `creds.AccessToken` for this submit path. `resolveAuditProbeCredentials` can still validate `NEWAPI_BASE_URL`, but it must not provide the detection token.

- [ ] **Step 4: Modify diagnostic input metadata**

Do not store plaintext key in `DiagnosticRun.Input`. Add only credential source:

```go
"credential_source": "audit_targets.api_key",
```

Do not add `api_key`, `key_last4`, or token fingerprint to diagnostic input.

- [ ] **Step 5: Add test proving stored key is used**

Use an HTTP test server that checks Authorization:

```go
func TestAuditDiagnosticSubmitUsesStoredChannelKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
		APIKey:       "sk-channel-key",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-channel-key" {
			t.Fatalf("Authorization = %q, want channel key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     srv.URL,
			AccessToken: "sync-token-must-not-be-used",
			UserID:      "u1",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"OpenAI","service":"cc","channel":"101:demo","model":"gpt-4o","request_model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 6: Add missing credential test**

Add:

```go
func TestAuditDiagnosticSubmitRejectsMissingChannelKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://newapi.example.com",
			AccessToken: "sync-token",
			UserID:      "u1",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"OpenAI","service":"cc","channel":"101:demo","model":"gpt-4o","request_model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_credential") {
		t.Fatalf("submit should fail missing credential: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 7: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "feat: use channel key for diagnostics"
```

---

## Task 5: Use Stored Channel Key For Backfill And Template Probe

**Files:**
- Modify: `internal/api/audit_handler.go`
- Modify: `internal/audit/template_probe_backfill.go`
- Test: `internal/api/audit_handler_test.go`
- Test: `internal/audit/template_probe_backfill_test.go`

- [ ] **Step 1: Modify diagnostic backfill**

Inside `PostAuditDiagnosticBackfill`, each selected `storage.AuditTarget` already comes from `ListAuditTargets`.

Before running:

```go
if strings.TrimSpace(target.APIKey) == "" {
	item := auditDiagnosticBackfillItemResponse{
		Provider: runTarget.Provider,
		Service:  runTarget.Service,
		Channel:  runTarget.Channel,
		Model:    runTarget.Model,
		Status:   "failed",
		Error:    "missing_credential: 当前渠道未配置监测 key",
	}
	failed++
	items = append(items, item)
	continue
}
```

Set:

```go
runTarget.BaseURL = strings.TrimSpace(appCfg.NewAPI.BaseURL)
runTarget.AccessToken = strings.TrimSpace(target.APIKey)
runTarget.UserID = strings.TrimSpace(appCfg.NewAPI.UserID)
```

Do not use `resolveAuditProbeCredentials` for the token.

- [ ] **Step 2: Modify template probe backfill**

In `PostAuditTemplateProbeBackfill`, use:

```go
if strings.TrimSpace(target.APIKey) == "" {
	item.Status = "failed"
	item.Error = "missing_credential: 当前渠道未配置监测 key"
	failed++
	items = append(items, item)
	continue
}
```

Build template probe config with:

```go
probeCfg, err := audit.BuildTemplateProbeConfig(appCfg, target, audit.TemplateProbeCredentials{
	BaseURL:     strings.TrimSpace(appCfg.NewAPI.BaseURL),
	AccessToken: strings.TrimSpace(target.APIKey),
	UserID:      strings.TrimSpace(appCfg.NewAPI.UserID),
}, templateName, configDir)
```

- [ ] **Step 3: Ensure template probe config keeps target key**

`BuildTemplateProbeConfig` already maps `creds.AccessToken` to `ServiceConfig.APIKey`. Keep that contract. Add a test to verify:

```go
func TestBuildTemplateProbeConfigUsesTargetCredential(t *testing.T) {
	app := &config.AppConfig{}
	target := storage.AuditTarget{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}
	cfg, err := BuildTemplateProbeConfig(app, target, TemplateProbeCredentials{
		BaseURL:     "https://newapi.example.com",
		AccessToken: "sk-channel-key",
		UserID:      "u1",
	}, "cx-gpt-chat-diagnostic", testConfigDirWithTemplate(t))
	if err != nil {
		t.Fatalf("BuildTemplateProbeConfig: %v", err)
	}
	if cfg.APIKey != "sk-channel-key" {
		t.Fatalf("APIKey = %q, want channel key", cfg.APIKey)
	}
}
```

Use the existing helper pattern from `internal/audit/template_probe_backfill_test.go` for creating a temporary template file.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/api ./internal/audit
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/audit_handler.go internal/api/audit_handler_test.go internal/audit/template_probe_backfill.go internal/audit/template_probe_backfill_test.go
git commit -m "feat: use channel keys for probe backfill"
```

---

## Task 6: Expose Non-Sensitive Credential Status

**Files:**
- Modify: `internal/api/audit_types.go`
- Modify: `internal/api/audit_handler.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add status fields to model status response**

Extend `auditModelStatusItemResponse`:

```go
CredentialConfigured bool   `json:"credential_configured"`
CredentialLast4      string `json:"credential_last4,omitempty"`
```

- [ ] **Step 2: Populate model status**

When building model status items, set:

```go
CredentialConfigured: strings.TrimSpace(target.APIKey) != "",
CredentialLast4:      last4ForAPIResponse(target.APIKey),
```

Add local helper in API package:

```go
func last4ForAPIResponse(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
```

- [ ] **Step 3: Do not expose plaintext in targets response**

`storage.AuditTarget.APIKey` has `json:"-"`; add a test that `/api/audit/targets` does not include the raw key.

Test:

```go
func TestAuditTargetsResponseDoesNotExposeAPIKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "m1",
		RequestModel: "m1",
		Enabled:      true,
		APIKey:       "sk-secret-1234",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/targets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("targets unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret-1234") {
		t.Fatalf("targets leaked key: %s", rec.Body.String())
	}
}
```

- [ ] **Step 4: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/audit_types.go internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "feat: expose audit credential status"
```

---

## Task 7: Update Documentation

**Files:**
- Modify: `docs/relaypulse-probe-requirements-zh.md`
- Modify: `docs/user/config.md`

- [ ] **Step 1: Update requirements document**

In `docs/relaypulse-probe-requirements-zh.md`, update the new-api 接入 and quick-probe sections with:

```markdown
### 渠道级监测凭证

从 `new-api` 同步过来的渠道元数据不包含每个渠道的真实监测 key。RelayPulse 在本地 `audit_targets` 中保存渠道级 `api_key`，按 `provider + service + channel` 维护，并复制到该渠道下的所有模型目标。

主动检测、模板补洞和 quick-probe-v1 必须使用 `audit_targets.api_key`。如果目标渠道未配置 key，检测结果必须标记为 `missing_credential`，不得 fallback 到 `NEWAPI_ACCESS_TOKEN`。

`NEWAPI_ACCESS_TOKEN` 只用于读取 `new-api` 渠道和日志；它不是渠道真实性检测凭证。
```

- [ ] **Step 2: Update user config document**

In `docs/user/config.md`, add API usage:

```markdown
### 设置渠道监测 key

```bash
curl -X PUT "$RELAY_PULSE_BASE_URL/api/audit/targets/credential" \
  -H 'Content-Type: application/json' \
  -d '{
    "provider": "alan-官key直连",
    "service": "cc",
    "channel": "78:ClaudeCN-gpt",
    "api_key": "sk-xxx"
  }'
```

响应只返回 `key_last4`，不会返回明文 key。
```
```

- [ ] **Step 3: Validate markdown**

Run:

```bash
rg -n "missing_credential|audit_targets.api_key|targets/credential" docs/relaypulse-probe-requirements-zh.md docs/user/config.md
```

Expected: all three terms appear in both docs where applicable.

- [ ] **Step 4: Commit**

```bash
git add docs/relaypulse-probe-requirements-zh.md docs/user/config.md
git commit -m "docs: document channel credential monitoring"
```

---

## Task 8: Full Verification

**Files:**
- No code edits unless tests reveal a concrete bug.

- [ ] **Step 1: Run focused tests**

Run:

```bash
go test ./internal/storage ./internal/newapi ./internal/audit ./internal/api
```

Expected: pass.

- [ ] **Step 2: Run full backend tests**

Run:

```bash
go test ./...
```

Expected: pass.

- [ ] **Step 3: Manual SQLite verification**

Start local app with SQLite, then run:

```bash
curl -sS -X POST http://127.0.0.1:18080/api/audit/newapi/sync/channels
```

Set key:

```bash
curl -sS -X PUT http://127.0.0.1:18080/api/audit/targets/credential \
  -H 'Content-Type: application/json' \
  -d '{"provider":"alan-官key直连","service":"cc","channel":"78:ClaudeCN-gpt","api_key":"sk-test-channel-key"}'
```

Submit diagnostic:

```bash
curl -sS -X POST http://127.0.0.1:18080/api/audit/diagnostics \
  -H 'Content-Type: application/json' \
  -d '{"provider":"alan-官key直连","service":"cc","channel":"78:ClaudeCN-gpt","model":"claude-opus-4-8","request_model":"claude-opus-4-8"}'
```

Expected:

- With key: response contains `run_id`.
- Without key: response contains `missing_credential`.
- Response bodies never contain `sk-test-channel-key`.

- [ ] **Step 4: Verify sync preserves key**

After setting a key, run channel sync again:

```bash
curl -sS -X POST http://127.0.0.1:18080/api/audit/newapi/sync/channels
```

Then submit diagnostic again with the same target.

Expected:

- Diagnostic still uses the stored channel key.
- No `missing_credential` after sync.

- [ ] **Step 5: Final commit**

If verification required code fixes:

```bash
git add .
git commit -m "test: verify channel credential monitoring"
```

If no fixes were needed, do not create an empty commit.

---

## Acceptance Criteria

| ID | Acceptance |
|---|---|
| A1 | `audit_targets` has `api_key` column in SQLite and PostgreSQL |
| A2 | Existing databases migrate without data loss |
| A3 | Channel sync every 5 minutes does not clear stored keys |
| A4 | Credential update API applies one key to all models under the same `provider + service + channel` |
| A5 | Credential API never returns plaintext key |
| A6 | Quick-probe uses `audit_targets.api_key`, not global `NEWAPI_ACCESS_TOKEN` |
| A7 | Template probe backfill uses `audit_targets.api_key` |
| A8 | Missing key returns `missing_credential` |
| A9 | `NEWAPI_ACCESS_TOKEN` remains only for read-only new-api sync and logs |
| A10 | `go test ./internal/storage ./internal/newapi ./internal/audit ./internal/api` passes |

---

## Out Of Scope

- 不从 `new-api` 数据库读取渠道 key。
- 不修改 `new-api`。
- 不在前端新增 key 管理页面。
- 不做 key 加密存储。当前 plan 先按用户要求实现“数据库字段保存 key 并用于检测”；加密可作为后续安全增强单独计划。
- 不改变渠道同步频率。当前默认仍为 5 分钟。

---

## Self-Review

Spec coverage:

- 渠道表增加 key 字段：Task 1。
- 监测时使用数据库 key：Task 4、Task 5。
- 同步不会清空 key：Task 1、Task 8。
- 缺少 key 不使用全局凭证：Task 4、Task 5。
- API 写入 key：Task 3。

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No `implement later`.
- No `Similar to`.

Type consistency:

- Storage field name: `AuditTarget.APIKey`.
- Database column: `audit_targets.api_key`.
- API request field: `api_key`.
- Missing credential marker: `missing_credential`.
