import { useTranslation } from 'react-i18next';
import type { AdminSubmission, SubmissionStatus } from '../../types/onboarding';

const STATUS_FILTERS = ['all', 'pending', 'approved', 'rejected', 'published'] as const;
type StatusFilter = (typeof STATUS_FILTERS)[number];

const PAGE_SIZE = 20;

interface SubmissionListProps {
  submissions: AdminSubmission[];
  total: number;
  statusFilter: StatusFilter;
  setStatusFilter: (filter: StatusFilter) => void;
  page: number;
  setPage: (page: number) => void;
  searchQuery: string;
  setSearchQuery: (query: string) => void;
  onSelect: (submission: AdminSubmission) => void;
  isLoading: boolean;
}

/** Status badge with contextual coloring. */
function StatusBadge({ status }: { status: SubmissionStatus }) {
  const { t } = useTranslation();

  const styleMap: Record<SubmissionStatus, string> = {
    pending: 'bg-warning/15 text-warning',
    approved: 'bg-accent/15 text-accent',
    rejected: 'bg-danger/15 text-danger',
    published: 'bg-success/15 text-success',
  };

  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${styleMap[status]}`}>
      {t(`admin.status.${status}`)}
    </span>
  );
}

/** Truncate a public_id for display, showing first 8 characters. */
function truncateId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}...` : id;
}

/** Format a unix timestamp to a locale-appropriate date string. */
function formatTimestamp(ts: number): string {
  if (!ts) return '--';
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export const SubmissionList: React.FC<SubmissionListProps> = ({
  submissions,
  total,
  statusFilter,
  setStatusFilter,
  page,
  setPage,
  searchQuery,
  setSearchQuery,
  onSelect,
  isLoading,
}) => {
  const { t } = useTranslation();
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="space-y-4">
      {/* Search by public_id */}
      <input
        type="text"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        placeholder={t('admin.submissions.searchPlaceholder')}
        className="w-full px-3 py-2 rounded-lg bg-surface border border-default text-sm text-primary
                   placeholder:text-muted focus:outline-none focus:border-accent transition-colors"
      />

      {/* Status filter tabs */}
      <div className="flex gap-1 bg-elevated rounded-lg p-1">
        {STATUS_FILTERS.map((filter) => (
          <button
            key={filter}
            onClick={() => {
              setStatusFilter(filter);
              setPage(1);
            }}
            className={`px-3 py-1.5 text-sm rounded-md transition-colors ${
              statusFilter === filter
                ? 'bg-accent/10 text-accent font-medium'
                : 'text-muted hover:text-secondary'
            }`}
          >
            {t(`admin.filter.${filter}`)}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="bg-surface border border-default rounded-lg overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-default bg-elevated">
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.publicId')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.provider')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.serviceType')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.channelCode')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.sponsorLevel')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.status')}
                </th>
                <th className="text-left px-4 py-3 text-muted font-medium">
                  {t('admin.table.createdAt')}
                </th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-muted">
                    {t('admin.table.loading')}
                  </td>
                </tr>
              ) : submissions.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-muted">
                    {t('admin.table.empty')}
                  </td>
                </tr>
              ) : (
                submissions.map((sub) => (
                  <tr
                    key={sub.id}
                    onClick={() => onSelect(sub)}
                    className="border-b border-default last:border-b-0
                               hover:bg-elevated cursor-pointer transition-colors"
                  >
                    <td className="px-4 py-3 text-muted font-mono text-xs" title={sub.public_id}>
                      {truncateId(sub.public_id)}
                    </td>
                    <td className="px-4 py-3 text-primary font-medium">
                      {sub.provider_name}
                    </td>
                    <td className="px-4 py-3 text-secondary">{sub.service_type}</td>
                    <td className="px-4 py-3 text-secondary">{sub.channel_code || '--'}</td>
                    <td className="px-4 py-3 text-secondary">{sub.sponsor_level}</td>
                    <td className="px-4 py-3">
                      <StatusBadge status={sub.status} />
                    </td>
                    <td className="px-4 py-3 text-muted text-xs">
                      {formatTimestamp(sub.created_at)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-default">
            <span className="text-xs text-muted">
              {t('admin.pagination.total', { count: total })}
            </span>
            <div className="flex items-center gap-1">
              <button
                onClick={() => setPage(Math.max(1, page - 1))}
                disabled={page <= 1}
                className="px-2.5 py-1 text-xs rounded border border-default text-secondary
                           hover:bg-elevated transition-colors
                           disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {t('admin.pagination.prev')}
              </button>
              <span className="px-3 py-1 text-xs text-muted">
                {page} / {totalPages}
              </span>
              <button
                onClick={() => setPage(Math.min(totalPages, page + 1))}
                disabled={page >= totalPages}
                className="px-2.5 py-1 text-xs rounded border border-default text-secondary
                           hover:bg-elevated transition-colors
                           disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {t('admin.pagination.next')}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
