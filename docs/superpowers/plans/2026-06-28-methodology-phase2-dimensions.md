# Methodology Phase2 Dimensions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 补齐“检测方法”页面中当前未实现的 15 个 phase2 检测维度，并让页面统计从 `10/25` 逐步提升到对应已实现数量。

**Architecture:** 保持现有 quick-probe 模板体系和 `DiagnosticRunner` 主链路，优先复用已存储的 `DiagnosticStep.ExecutionMeta`、响应头、usage、stream_chunks、request_body 和 baseline steps。分三阶段推进：先实现不需要新增请求的评分器，再扩展 quick-probe steps 获取缺失证据，最后实现 cache/thinking 这类依赖上游字段和多轮会话行为的维度。

**Tech Stack:** Go, SQLite/PostgreSQL storage, existing `internal/audit/diagnostic_runner.go`, template-driven probe requests, existing `/api/audit/methodology` display.

---

## Execution Workspace Boundary

本 plan **必须在单独的 git worktree 中实现**，禁止直接在当前工作区继续编码。

执行前必须完成：

1. 从当前目标分支创建独立 worktree，例如：

```bash
git worktree add ../relay-pulse-methodology-phase2 -b methodology-phase2-dimensions
```

2. 在新 worktree 中确认分支和状态：

```bash
cd ../relay-pulse-methodology-phase2
git status --short --branch
```

3. 后续所有代码修改、测试、提交都必须在该 worktree 内完成。

4. 当前工作区只作为对照和用户验证环境使用，不允许在当前工作区混入本 plan 的实现代码。

停止条件：

- 如果目标分支已经存在同名 worktree 或同名分支，先报告实际路径和分支状态，由用户确认复用、重建或换名。
- 如果当前工作区存在未提交改动，不要移动、清理或回滚这些改动。

---

## Current Methodology State

当前 `/api/audit/methodology` 返回：

```text
version: v3.24.1
total_dimensions: 25
implemented_count: 10
active_count: 10
implemented_weight: 69
active_weight: 69
total_weight: 188
```

未实现 15 个维度：

| Group | Dimension | Weight | Evidence Source | Requires New Probe Step |
|---|---:|---:|---|---|
| 模型与缓存 | `cache_hit_ratio_match` | 20 | usage cache fields | yes |
| 模型与缓存 | `cache_continuity_intra` | 14 | multi-turn cache behavior | yes |
| 模型与缓存 | `cache_sliding_correctness` | 13 | repeated prompt over time | yes, scheduled/repeated |
| 模型与缓存 | `cache_ttl_consistency` | 15 | cache behavior over TTL windows | yes, scheduled/repeated |
| 身份与知识 | `self_identity_consistency` | 8 | identity + identity_free + envelope | no |
| 身份与知识 | `envelope_self_report_match` | 3 | response model/body identity | no |
| 身份与知识 | `thinking_present` | 4 | response metadata/content blocks | partly |
| 身份与知识 | `thinking_volume_match` | 6 | thinking text/token volume | yes for thinking-capable templates |
| 身份与知识 | `tier_thinking_volume_match` | 8 | cross-tier baseline | yes, cross-model aggregation |
| 身份与知识 | `world_knowledge_tier_match` | 12 | new knowledge probe | yes |
| 上游协议表面 | `anthropic_msg_id_format` | 8 | raw response envelope / ids | no if parser stores id |
| 上游协议表面 | `inference_geo_present` | 5 | response headers/body metadata | no if present |
| 上游协议表面 | `system_prompt_clean` | 8 | request_body size/content | no |
| 上游协议表面 | `sdk_consistency` | 2 | request/response headers over steps | no |
| 流式投递 | `buffer_dump_match` | 5 | stream_chunks timing/shape | no |

---

## File Structure

Modify:

- `internal/audit/diagnostic_runner.go`
  - Add scorers for all phase2 dimensions.
  - Add evidence extraction helpers for usage cache fields, ids, geo, SDK, stream chunks, thinking blocks and prompt/body size.
  - Expand `baselineAwareDimensions`.

- `internal/audit/methodology.go`
  - Flip `Implemented` and `Active` only when a dimension has scorer coverage and tests.
  - Keep dimensions requiring scheduled TTL checks inactive until scheduler support exists.

- `internal/audit/diagnostic_runner_test.go`
  - Add table tests for scorer pass/partial/fail/skip behavior.

- `internal/audit/request_executor.go`
  - Only if missing raw response fields are not available in `ExecutionMeta`.
  - Add stable extraction of response id, content block types, cache usage fields, stream event timing.

- `templates/cx-gpt-chat-diagnostic.json`
- `templates/cx-gpt-chat-diagnostic-notemp.json`
  - No change for phase 2A.
  - Add optional probe steps only in later tasks through runner prompt definitions, not by hardcoding endpoint paths.

Potentially create:

- `internal/audit/diagnostic_evidence.go`
  - If `diagnostic_runner.go` grows too large, move extraction helpers here.

- `internal/audit/diagnostic_phase2_scorers.go`
  - If scorers exceed ~300 lines, move phase2 scoring functions here.

No frontend changes are required except methodology page automatically reflecting `/api/audit/methodology`.

---

## Implementation Strategy

### Phase 2A: Scorers From Existing Evidence

These dimensions should be implemented first because the current diagnostic run already records enough data in `ExecutionMeta`:

- `self_identity_consistency`
- `envelope_self_report_match`
- `anthropic_msg_id_format`
- `inference_geo_present`
- `system_prompt_clean`
- `sdk_consistency`
- `buffer_dump_match`

Expected result:

```text
implemented_count: 17
active_count: 17
```

### Phase 2B: New Probe Prompts

These require one or more new quick-probe steps:

- `world_knowledge_tier_match`
- `thinking_present`
- `thinking_volume_match`

Expected result after stable prompts and baseline comparison:

```text
implemented_count: 20
active_count: 20
```

### Phase 2C: Cache And Cross-Tier / Time-Window Checks

These require repeated requests or multi-run aggregation:

- `cache_hit_ratio_match`
- `cache_continuity_intra`
- `cache_sliding_correctness`
- `cache_ttl_consistency`
- `tier_thinking_volume_match`

Expected result:

```text
implemented_count: 25
active_count: 21-25
```

`cache_sliding_correctness` and `cache_ttl_consistency` should remain `Implemented=true, Active=false` until scheduled repeated-run orchestration exists, because a single quick-probe run cannot validate 5m/1h TTL honestly.

---

### Task 1: Add Phase2 Evidence Extraction Helpers

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Add helper function signatures**

Add helpers near existing metadata helpers such as `collectServiceTierValues`, `collectRequestIDChain`, and `visibleTextFromStep`:

```go
func collectResponseIDs(steps []*storage.DiagnosticStep) []string
func collectInferenceGeoValues(steps []*storage.DiagnosticStep) []string
func collectSDKNames(steps []*storage.DiagnosticStep) []string
func collectStreamChunkStats(steps []*storage.DiagnosticStep) streamChunkStats
func collectRequestBodySizes(steps []*storage.DiagnosticStep) []int
func collectCacheUsageStats(steps []*storage.DiagnosticStep) cacheUsageStats
func collectThinkingStats(steps []*storage.DiagnosticStep) thinkingStats
```

Define structs:

```go
type streamChunkStats struct {
	StepCount         int
	BufferedSteps     int
	ChunkCounts       []int
	VisibleTextSpans  []int
}

type cacheUsageStats struct {
	InputTokens       int
	CacheReadTokens  int
	CacheCreateTokens int
	HasCacheFields   bool
}

type thinkingStats struct {
	Present       bool
	TokenEstimate int
	TextLength    int
	BlockTypes    []string
}
```

- [x] **Step 2: Implement helper behavior**

Extraction rules:

- `collectResponseIDs` reads `response_text`, `response_headers`, and raw response metadata if available; match ids with prefixes:

```text
msg_
chatcmpl-
gen-
```

- `collectInferenceGeoValues` searches response headers and response JSON fields for:

```text
cf-ray
x-vercel-id
x-amzn-trace-id
x-request-id
inference_region
region
geo
```

- `collectSDKNames` searches request headers/body and response metadata for:

```text
sdk_name
x-stainless-lang
anthropic-version
user-agent
```

- `collectStreamChunkStats` uses `stream_chunks` from `ExecutionMeta`; if a step has `len(stream_chunks) <= 1`, mark as buffered.

- `collectRequestBodySizes` uses serialized `request_body` size from `ExecutionMeta`.

- `collectCacheUsageStats` reads usage keys:

```text
cache_read_input_tokens
cache_creation_input_tokens
cache_tokens
cache_read
cache_create_tokens
input_tokens
prompt_tokens
```

- `collectThinkingStats` searches response body/metadata for:

```text
thinking
reasoning
reasoning_content
content[].type == "thinking"
```

- [x] **Step 3: Add helper tests**

Add tests in `internal/audit/diagnostic_runner_test.go`:

```go
func TestPhase2EvidenceExtraction(t *testing.T) {
	steps := []*storage.DiagnosticStep{
		{
			RunID: "run-1",
			StepIndex: 1,
			ExecutionMeta: mustJSON(map[string]any{
				"response_headers": map[string]string{
					"x-request-id": "req_01abc",
					"cf-ray": "abc-SJC",
					"x-stainless-lang": "go",
				},
				"usage": map[string]any{
					"input_tokens": 100,
					"cache_read_input_tokens": 40,
					"cache_creation_input_tokens": 10,
				},
				"stream_chunks": []string{"a", "b", "c"},
				"request_body": map[string]any{"model": "claude-opus-4-6"},
				"response_text": `{"id":"msg_01abc","thinking":"hidden chain"}`,
			}),
		},
	}
	if got := collectResponseIDs(steps); len(got) == 0 {
		t.Fatalf("expected response ids")
	}
	if got := collectInferenceGeoValues(steps); len(got) == 0 {
		t.Fatalf("expected geo values")
	}
	if got := collectSDKNames(steps); len(got) == 0 {
		t.Fatalf("expected sdk names")
	}
	if got := collectStreamChunkStats(steps); got.BufferedSteps != 0 || len(got.ChunkCounts) != 1 {
		t.Fatalf("unexpected stream stats: %+v", got)
	}
	if got := collectCacheUsageStats(steps); !got.HasCacheFields || got.CacheReadTokens != 40 {
		t.Fatalf("unexpected cache stats: %+v", got)
	}
	if got := collectThinkingStats(steps); !got.Present {
		t.Fatalf("expected thinking stats")
	}
}
```

- [x] **Step 4: Run tests**

Run:

```bash
go test ./internal/audit -run TestPhase2EvidenceExtraction -v
```

Expected:

```text
PASS
```

---

### Task 2: Implement Existing-Evidence Phase2 Scorers

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Add scorer functions**

Add these functions:

```go
func scoreSelfIdentityConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreEnvelopeSelfReportMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreAnthropicMsgIDFormat(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreInferenceGeoPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreSystemPromptClean(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreSDKConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreBufferDumpMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
```

Scoring rules:

| Dimension | Pass | Partial | Fail | Skip |
|---|---|---|---|---|
| `self_identity_consistency` | structured and free identity same family | same vendor/brand but model family ambiguous | contradicts baseline or self | identity missing |
| `envelope_self_report_match` | response model/id family agrees with text identity | only model family agrees | response model contradicts text identity | response model missing |
| `anthropic_msg_id_format` | candidate has `msg_` when baseline has `msg_` | candidate has stable non-empty id | baseline has `msg_`, candidate missing/wrong | baseline id missing |
| `inference_geo_present` | candidate exposes geo/trace when baseline does | candidate exposes generic trace only | baseline exposes geo, candidate missing | baseline geo missing |
| `system_prompt_clean` | request body size within 2x baseline | within 4x baseline | >4x baseline | baseline body missing |
| `sdk_consistency` | same sdk marker across all steps | one missing value | multiple conflicting sdk markers | no sdk markers |
| `buffer_dump_match` | buffered ratio within 20% baseline | within 50% baseline | candidate far more buffered | stream data missing |

- [x] **Step 2: Add scorers to `baselineAwareDimensions`**

Append after current phase1 dimensions:

```go
	out = append(out, scoreSelfIdentityConsistency(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreEnvelopeSelfReportMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreAnthropicMsgIDFormat(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreInferenceGeoPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreSystemPromptClean(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreSDKConsistency(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreBufferDumpMatch(runID, steps, baselineSteps, createdAt))
```

- [x] **Step 3: Add scorer table test**

Add:

```go
func TestBuildDimensionsForRunWithPhase2ExistingEvidence(t *testing.T) {
	candidateSteps := []*storage.DiagnosticStep{
		{
			RunID: "candidate",
			StepIndex: 2,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name": "identity",
				"response_model": "claude-opus-4-6",
				"response_text": "vendor=Anthropic\nbrand=Claude\nmodel=Claude Opus 4",
				"response_headers": map[string]string{"x-request-id": "req_01candidate", "cf-ray": "abc-SJC", "x-stainless-lang": "go"},
				"request_body": map[string]any{"model": "claude-opus-4-6", "messages": []any{}},
				"stream_chunks": []string{"a", "b", "c"},
			}),
		},
	}
	baselineSteps := []*storage.DiagnosticStep{
		{
			RunID: "baseline",
			StepIndex: 2,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name": "identity",
				"response_model": "claude-opus-4-6",
				"response_text": "vendor=Anthropic\nbrand=Claude\nmodel=Claude Opus 4",
				"response_headers": map[string]string{"x-request-id": "req_01baseline", "cf-ray": "xyz-SJC", "x-stainless-lang": "go"},
				"request_body": map[string]any{"model": "claude-opus-4-6", "messages": []any{}},
				"stream_chunks": []string{"a", "b", "c"},
			}),
		},
	}
	dimensions := buildDimensionsForRun("run-1", &storage.DiagnosticScore{OverallScore: 90}, nil, candidateSteps, baselineSteps, 1710000000)
	found := make(map[string]*storage.DiagnosticDimension, len(dimensions))
	for _, dimension := range dimensions {
		found[dimension.DimensionKey] = dimension
	}
	for _, key := range []string{
		"self_identity_consistency",
		"envelope_self_report_match",
		"anthropic_msg_id_format",
		"inference_geo_present",
		"system_prompt_clean",
		"sdk_consistency",
		"buffer_dump_match",
	} {
		if found[key] == nil {
			t.Fatalf("missing dimension %s", key)
		}
		if found[key].Status == "skip" {
			t.Fatalf("dimension %s skipped unexpectedly: %+v", key, found[key])
		}
	}
}
```

- [x] **Step 4: Run tests**

Run:

```bash
go test ./internal/audit -run 'TestBuildDimensionsForRunWithPhase2ExistingEvidence|TestPhase2EvidenceExtraction' -v
```

Expected:

```text
PASS
```

---

### Task 3: Mark Phase2A Dimensions Implemented

**Files:**
- Modify: `internal/audit/methodology.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Flip implemented flags**

In `internal/audit/methodology.go`, set these to `Implemented: true, Active: true`:

```go
self_identity_consistency
envelope_self_report_match
anthropic_msg_id_format
inference_geo_present
system_prompt_clean
sdk_consistency
buffer_dump_match
```

- [x] **Step 2: Run methodology API locally or unit test by package**

Run:

```bash
go test ./internal/audit ./internal/api
```

Expected:

```text
ok  	monitor/internal/audit
ok  	monitor/internal/api
```

Expected `/api/audit/methodology` after service restart:

```text
implemented_count: 17
active_count: 17
implemented_weight: 107
active_weight: 107
```

---

### Task 4: Add New Probe Steps For World Knowledge And Thinking

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Extend quick probe steps**

Add step definitions to `quickProbeSteps`:

```go
{
	Name: "world_knowledge_tier",
	Prompt: "Answer with only one line: RP_WORLD_KNOWLEDGE=<number>. How many permanent members are in the United Nations Security Council?",
	FreshSession: true,
},
{
	Name: "thinking_probe",
	Prompt: "Solve mentally and answer only RP_THINKING_CHECK=<final number>: if x=17 and y=23, compute x*y + 19.",
	FreshSession: true,
},
```

Use deterministic factual/math probes. Do not ask the model to reveal chain-of-thought.

- [x] **Step 2: Update `stepNameForStorageStep` fallback**

Extend fallback mapping for new indexes if needed:

```go
case 7:
	return "world_knowledge_tier"
case 8:
	return "thinking_probe"
```

- [x] **Step 3: Add scorers**

Add:

```go
func scoreWorldKnowledgeTierMatch(runID string, candidate *storage.DiagnosticStep, baseline *storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreThinkingPresent(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreThinkingVolumeMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
```

Rules:

- `world_knowledge_tier_match`: compare extracted numeric answer with baseline. Exact match = 10, off by 1 = 5, missing = skip.
- `thinking_present`: pass only when baseline has thinking signal and candidate has comparable signal; skip when baseline lacks thinking signal.
- `thinking_volume_match`: compare `thinkingStats.TokenEstimate` or text length against baseline using `relativeSimilarityScore`.

- [x] **Step 4: Append scorers to `baselineAwareDimensions`**

Add:

```go
	if candidate, ok := byName["world_knowledge_tier"]; ok {
		out = append(out, scoreWorldKnowledgeTierMatch(runID, candidate, baselineByName["world_knowledge_tier"], createdAt))
	}
	out = append(out, scoreThinkingPresent(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreThinkingVolumeMatch(runID, steps, baselineSteps, createdAt))
```

- [x] **Step 5: Add tests**

Add a test that creates candidate and baseline `world_knowledge_tier` steps with `RP_WORLD_KNOWLEDGE=5` and asserts the dimension passes.

Add a test with baseline thinking metadata and candidate thinking metadata and asserts `thinking_present` and `thinking_volume_match` are not skipped.

- [x] **Step 6: Mark dimensions implemented**

In `internal/audit/methodology.go`, set:

```go
thinking_present: Implemented true, Active true
thinking_volume_match: Implemented true, Active true
world_knowledge_tier_match: Implemented true, Active true
```

Expected methodology after service restart:

```text
implemented_count: 20
active_count: 20
implemented_weight: 129
active_weight: 129
```

---

### Task 5: Implement Cache Dimensions That Can Be Measured In One Run

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Modify: `internal/audit/methodology.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Add cache reuse probe steps**

Add two same-session steps:

```go
{
	Name: "cache_seed",
	Prompt: "Remember this exact marker for the next message: RP_CACHE_MARKER=blue-17-river. Reply only RP_CACHE_SEEDED=1.",
	FreshSession: true,
},
{
	Name: "cache_recall",
	Prompt: "Reply only with the marker from the previous message in the format RP_CACHE_MARKER=<marker>.",
	FreshSession: false,
},
```

- [x] **Step 2: Add scorers**

Add:

```go
func scoreCacheHitRatioMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreCacheContinuityIntra(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
```

Rules:

- `cache_hit_ratio_match`: if baseline and candidate both expose cache fields, compare cache read ratio. Within 20% = pass, within 50% = partial, otherwise fail. If baseline has no cache fields, skip.
- `cache_continuity_intra`: candidate must return `RP_CACHE_MARKER=blue-17-river`; exact = pass, missing = fail. Baseline response stored in evidence.

- [x] **Step 3: Append scorers to `baselineAwareDimensions`**

Add:

```go
	out = append(out, scoreCacheHitRatioMatch(runID, steps, baselineSteps, createdAt))
	out = append(out, scoreCacheContinuityIntra(runID, steps, baselineSteps, createdAt))
```

- [x] **Step 4: Mark dimensions implemented and active**

In `internal/audit/methodology.go`, set:

```go
cache_hit_ratio_match: Implemented true, Active true
cache_continuity_intra: Implemented true, Active true
```

Expected methodology after service restart:

```text
implemented_count: 22
active_count: 22
implemented_weight: 163
active_weight: 163
```

---

### Task 6: Add Scheduled Cache TTL Dimensions As Implemented But Inactive

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Modify: `internal/audit/methodology.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [x] **Step 1: Add placeholder-safe scorers that skip honestly**

Add:

```go
func scoreCacheSlidingCorrectness(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
func scoreCacheTTLConsistency(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
```

Both must return:

```go
Status: "skip"
Reason: "requires scheduled repeated probes across cache TTL windows"
Score: 0
NormalizedScore: 0
```

Do not include these in active scoring until scheduler support exists.

- [x] **Step 2: Append dimensions for evidence visibility**

Append them to `baselineAwareDimensions`, but they will be skipped and excluded from effective score.

- [x] **Step 3: Mark implemented but inactive**

In `internal/audit/methodology.go`, set:

```go
cache_sliding_correctness: Implemented true, Active false
cache_ttl_consistency: Implemented true, Active false
```

Expected methodology:

```text
implemented_count: 24
active_count: 22
implemented_weight: 191
active_weight: 163
```

Note: implemented weight may exceed current total if the total changes later; if keeping total 188, verify exact arithmetic after all previous weights are included. Do not fake active status.

---

### Task 7: Implement Tier Thinking Volume As Aggregation-Aware But Initially Inactive

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Modify: `internal/audit/methodology.go`

- [x] **Step 1: Add scorer**

Add:

```go
func scoreTierThinkingVolumeMatch(runID string, steps []*storage.DiagnosticStep, baselineSteps []*storage.DiagnosticStep, createdAt int64) *storage.DiagnosticDimension
```

Initial rule:

- If target run and baseline run are same model family only, return skip:

```text
requires cross-tier baseline set
```

- Do not attempt to infer tier from one candidate/baseline pair.

- [x] **Step 2: Mark implemented but inactive**

In `internal/audit/methodology.go`, set:

```go
tier_thinking_volume_match: Implemented true, Active false
```

Expected final state:

```text
implemented_count: 25
active_count: 22
```

This dimension becomes active only after baseline storage supports cross-tier model families.

---

### Task 8: Final Verification

**Files:**
- All above.

- [x] **Step 1: Run Go tests**

Run:

```bash
go test ./internal/audit ./internal/api ./internal/config ./internal/storage
```

Expected:

```text
ok  	monitor/internal/audit
ok  	monitor/internal/api
ok  	monitor/internal/config
ok  	monitor/internal/storage
```

- [x] **Step 2: Restart local server and check methodology**

Run:

```bash
pid=$(lsof -tiTCP:18080 -sTCP:LISTEN || true)
if [ -n "$pid" ]; then kill $pid; sleep 1; fi
PORT=18080 go run ./cmd/server/main.go config.yaml
```

Then:

```bash
curl -sS http://127.0.0.1:18080/api/audit/methodology \
  | jq '.data.summary, [.data.dimensions[] | select(.implemented == false) | .key]'
```

Expected:

```text
implemented_count: 25
unimplemented list: []
```

If inactive dimensions remain:

```bash
curl -sS http://127.0.0.1:18080/api/audit/methodology \
  | jq '[.data.dimensions[] | select(.implemented == true and .active == false) | .key]'
```

Expected:

```json
[
  "cache_sliding_correctness",
  "cache_ttl_consistency",
  "tier_thinking_volume_match"
]
```

- [x] **Step 3: Verify UI page**

Open:

```text
http://127.0.0.1:18080/detect
```

Expected:

- 当前检测方法显示 `25/25` 已实现。
- 活跃维度显示 `22/25` 或当前实际 active count。
- Inactive dimensions show `已实现未启用`.
- No dimension remains `计划中`.

---

### Task 9: Commit

**Files:**
- Modified Go source and tests.
- This plan file.

- [x] **Step 1: Check status**

Run:

```bash
git status --short
```

Review unrelated dirty files before adding. Current branch already has other frontend/history-page work in progress; do not accidentally commit unrelated changes unless the user explicitly wants one combined commit.

- [x] **Step 2: Commit methodology work only**

Run:

```bash
git add internal/audit/diagnostic_runner.go \
  internal/audit/diagnostic_runner_test.go \
  internal/audit/methodology.go \
  docs/superpowers/plans/2026-06-28-methodology-phase2-dimensions.md
git commit -m "feat(audit): implement phase2 methodology dimensions"
```

If `request_executor.go` or new scorer files were created, add them explicitly:

```bash
git add internal/audit/request_executor.go internal/audit/diagnostic_evidence.go internal/audit/diagnostic_phase2_scorers.go
git commit -m "feat(audit): implement phase2 methodology dimensions"
```

---

## Self-Review

Spec coverage:

- All 15 currently unimplemented dimensions have a task.
- Dimensions that can be scored from existing evidence are prioritized.
- Dimensions requiring time windows or cross-tier aggregation are not falsely activated.

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No “similar to”.

Type consistency:

- All scorer names map to existing `MethodologyDimension.Key` values.
- All scorers return `*storage.DiagnosticDimension`.
- All new evidence helpers consume existing `[]*storage.DiagnosticStep`.
