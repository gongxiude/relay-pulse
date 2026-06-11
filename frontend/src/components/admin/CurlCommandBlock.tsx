import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Copy, Check } from 'lucide-react';
import { copyToClipboard } from '../../utils/share';

interface CurlCommandBlockProps {
  /** 后端下发的脱敏 curl（密钥用 $RP_API_KEY 占位）。空串时不渲染。 */
  curl: string;
  /** 真实明文密钥；提供时额外给出"复制（含密钥）"动作。缺省则只复制占位版。 */
  apiKey?: string;
}

/** 把占位 curl 还原成可直接运行的命令：去掉占位提示注释，前缀真实密钥 export。 */
function withRealKey(curl: string, apiKey: string): string {
  const escaped = apiKey.replace(/'/g, "'\\''");
  const body = curl.replace(/^#[^\n]*\n/, ''); // 去掉后端首行占位提示注释
  return `export RP_API_KEY='${escaped}'\n${body}`;
}

/**
 * 管理员测试结果里展示「本次实际请求」对应的可复制 curl 命令。
 *
 * 安全：默认复制的是脱敏版（密钥 $RP_API_KEY 占位，可安全贴进聊天/工单）；
 * 「复制（含密钥）」仅在调用方持有明文密钥时出现，真实密钥只在点击那一刻于
 * 客户端拼接，不随测试响应落入页面状态。
 */
export function CurlCommandBlock({ curl, apiKey }: CurlCommandBlockProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<'plain' | 'key' | null>(null);

  if (!curl) return null;

  const usesKeyVar = curl.includes('$RP_API_KEY');
  const canCopyWithKey = usesKeyVar && !!apiKey;

  const doCopy = async (text: string, which: 'plain' | 'key') => {
    if (await copyToClipboard(text)) {
      setCopied(which);
      setTimeout(() => setCopied(null), 2000);
    }
  };

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs text-muted">{t('admin.detail.testCurl')}</span>
        <button
          type="button"
          onClick={() => doCopy(curl, 'plain')}
          className="inline-flex items-center gap-1 text-xs text-secondary hover:text-primary transition"
        >
          {copied === 'plain' ? <Check className="w-3.5 h-3.5 text-success" /> : <Copy className="w-3.5 h-3.5" />}
          {t('admin.detail.testCurlCopy')}
        </button>
        {canCopyWithKey && (
          <button
            type="button"
            onClick={() => doCopy(withRealKey(curl, apiKey!), 'key')}
            className="inline-flex items-center gap-1 text-xs text-secondary hover:text-primary transition"
          >
            {copied === 'key' ? <Check className="w-3.5 h-3.5 text-success" /> : <Copy className="w-3.5 h-3.5" />}
            {t('admin.detail.testCurlCopyWithKey')}
          </button>
        )}
      </div>
      <pre className="whitespace-pre-wrap break-all text-xs max-h-48 overflow-y-auto bg-surface p-2 rounded text-secondary font-mono">
        {curl}
      </pre>
      {usesKeyVar && (
        <p className="text-[11px] text-muted">{t('admin.detail.testCurlKeyHint')}</p>
      )}
    </div>
  );
}
