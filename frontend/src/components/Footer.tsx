import { Github, Tag, Bug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useVersionInfo } from '../hooks/useVersionInfo';
import { FEEDBACK_URLS } from '../constants';
 
export function Footer() {
  const { t } = useTranslation();
  const { versionInfo } = useVersionInfo();

  return (
    <footer className="mt-4 border-t border-default/50 pt-4 text-secondary">
      <div className="flex flex-col sm:flex-row items-center justify-center gap-2 text-xs">
        <div className="flex items-center gap-2 flex-wrap justify-center">
          <a
            href="https://github.com/prehisle/relay-pulse"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-elevated/50 text-secondary hover:text-accent hover:bg-muted/50 transition min-h-[36px]"
          >
            <Github size={14} />
            <span>GitHub</span>
          </a>
          <span className="hidden sm:inline text-muted">·</span>
          <a
            href={FEEDBACK_URLS.BUG_REPORT}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-elevated/50 text-secondary hover:text-danger hover:bg-muted/50 transition min-h-[36px]"
          >
            <Bug size={14} />
            <span>{t('footer.issuesBtn')}</span>
          </a>
          <span className="hidden sm:inline text-muted">·</span>
          <span className="text-muted text-[11px] sm:text-xs">{t('footer.openSourceLabel')}</span>
        </div>
        {versionInfo && (
          <>
            <span className="hidden sm:inline text-muted">·</span>
            <div
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-elevated/50 text-secondary"
              title={`Commit: ${versionInfo.git_commit} | Built: ${versionInfo.build_time}`}
            >
              <Tag size={14} className="text-muted" />
              <span className="text-secondary">{versionInfo.version}</span>
            </div>
          </>
        )}
      </div>
    </footer>
  );
}
