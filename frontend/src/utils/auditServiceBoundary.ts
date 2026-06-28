import type { AuditChannelSnapshot } from '../types/audit';

export function getAuditDataService(
  snapshot?: Pick<AuditChannelSnapshot, 'service'> | null,
): string | undefined {
  const service = snapshot?.service?.trim();
  return service || undefined;
}
