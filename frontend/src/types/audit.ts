export interface AuditChannelSnapshot {
  id: number;
  newapi_channel_id: number;
  snapshot_at: number;
  provider: string;
  service: string;
  channel: string;
  model: string;
  enabled: boolean;
  channelType?: 'recommended' | 'official' | 'reverse' | 'mixed' | 'unknown' | 'user';
  channelTypeLabel?: string;
  raw?: Record<string, unknown> | null;
}

export interface AuditChannelsResponse {
  success: boolean;
  data: AuditChannelSnapshot[];
  meta?: {
    count?: number;
  };
}

export interface AuditSyncStatusResponse {
  success: boolean;
  data: {
    newapi_base_url: string;
    targets: {
      total: number;
      enabled: number;
    };
    channels?: {
      snapshot_at?: number;
      channel_count?: number;
      enabled_count?: number;
    } | null;
    log_cursor?: {
      name?: string;
      last_id?: number;
      last_time?: number;
      updated_at?: number;
    } | null;
    probe_runtime: {
      probe_credential_mode: 'dedicated' | 'sync_fallback' | 'missing' | string;
      probe_auth_configured: boolean;
      probe_user_configured: boolean;
      probe_ready: boolean;
      warning?: string;
    };
  };
}

export interface AuditDiagnosticRunSummary {
  run_id: string;
  provider: string;
  service: string;
  channel: string;
  model: string;
  status: string;
  run_status?: string;
  run_status_reason?: string;
  created_at: number;
  updated_at: number;
  request_model?: string;
  base_url?: string;
  group_id?: string;
  baseline_mode?: string;
  methodology_version?: string;
  weights_hash?: string;
  candidate_type?: string;
}

export interface AuditDiagnosticScoreSummary {
  run_id: string;
  authenticity_score: number;
  protocol_score: number;
  sse_score: number;
  overall_score: number;
  active_weight: number;
  skipped_dimensions?: string[];
  methodology_version?: string;
  weights_hash?: string;
  tags?: string[];
}

export interface AuditDiagnosticLatestItem {
  run: AuditDiagnosticRunSummary;
  score?: AuditDiagnosticScoreSummary | null;
  compare_url?: string;
  usable: boolean;
  filter_reason?: string;
}

export interface AuditDiagnosticLatestResponse {
  success: boolean;
  data: {
    items: AuditDiagnosticLatestItem[];
    meta?: {
      limit?: number;
      count?: number;
    };
  };
}

export interface AuditMethodologyDimension {
  key: string;
  weight: number;
  group: string;
  description: string;
  implemented: boolean;
  active: boolean;
  phase: string;
}

export interface AuditMethodologyResponse {
  success: boolean;
  data: {
    summary: {
      version: string;
      weights_hash: string;
      total_dimensions: number;
      total_weight: number;
      implemented_count: number;
      implemented_weight: number;
      active_count: number;
      active_weight: number;
    };
    coverage: {
      done_runs: number;
      dimension_runs: number;
      dimension_row_count: number;
      failed_auth_runs: number;
      failed_request_runs: number;
      filtered_runs: number;
    };
    runtime: {
      probe_credential_mode: 'dedicated' | 'sync_fallback' | 'missing' | string;
      probe_auth_configured: boolean;
      probe_user_configured: boolean;
      probe_ready: boolean;
      warning?: string;
    };
    dimensions: AuditMethodologyDimension[];
  };
}

export interface AuditDiagnosticCompareResponse {
  success: boolean;
  data: {
    group: {
      group_id?: string;
      candidate_run_id: string;
      baseline_run_id?: string;
      baseline_mode?: string;
      methodology_version?: string;
      weights_hash?: string;
    };
    candidate: {
      run: AuditDiagnosticRunSummary;
      score?: AuditDiagnosticScoreSummary | null;
      steps: Array<{
        id: number;
        run_id: string;
        step_index: number;
        step_name?: string;
        session_mode?: string;
        prompt: string;
        resolved_prompt?: string;
        response_preview?: string;
        result_summary?: string;
        execution: {
          status_code?: number;
          duration_ms?: number;
          latency_ms?: number;
          http_ttfb_ms?: number;
          first_text_ms?: number;
          ttft_ms?: number;
          response_text?: string;
          usage?: Record<string, unknown>;
        };
        error_message?: string;
      }>;
    };
    baseline?: {
      run: AuditDiagnosticRunSummary;
      score?: AuditDiagnosticScoreSummary | null;
      steps: Array<{
        id: number;
        run_id: string;
        step_index: number;
        step_name?: string;
        session_mode?: string;
        prompt: string;
        resolved_prompt?: string;
        response_preview?: string;
        result_summary?: string;
        execution: {
          status_code?: number;
          duration_ms?: number;
          latency_ms?: number;
          http_ttfb_ms?: number;
          first_text_ms?: number;
          ttft_ms?: number;
          response_text?: string;
          usage?: Record<string, unknown>;
        };
        error_message?: string;
      }>;
    } | null;
    dimensions: Array<{
      run_id: string;
      dimension_key: string;
      weight: number;
      score: number;
      normalized_score: number;
      status: string;
      reason: string;
      evidence?: Record<string, unknown> | null;
    }>;
    steps: Array<{
      step_index: number;
      step_name?: string;
      candidate: {
        id: number;
        run_id: string;
        step_index: number;
        step_name?: string;
        session_mode?: string;
        prompt: string;
        response_preview?: string;
        result_summary?: string;
        execution: {
          status_code?: number;
          duration_ms?: number;
          latency_ms?: number;
          http_ttfb_ms?: number;
          first_text_ms?: number;
          ttft_ms?: number;
          response_text?: string;
          usage?: Record<string, unknown>;
        };
        error_message?: string;
      };
      baseline?: {
        id: number;
        run_id: string;
        step_index: number;
        step_name?: string;
        session_mode?: string;
        prompt: string;
        response_preview?: string;
        result_summary?: string;
        execution: {
          status_code?: number;
          duration_ms?: number;
          latency_ms?: number;
          http_ttfb_ms?: number;
          first_text_ms?: number;
          ttft_ms?: number;
          response_text?: string;
          usage?: Record<string, unknown>;
        };
        error_message?: string;
      } | null;
    }>;
    summary: {
      overall_score: number;
      active_weight: number;
      skipped_dimensions?: string[];
      tags?: string[];
    };
  };
}
