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
