import { useState } from 'react';
import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useLocation } from 'react-router-dom';

import { SUPPORTED_LANGUAGES, LANGUAGE_PATH_MAP, LANGUAGE_NAMES, isSupportedLanguage, type SupportedLanguage } from '../i18n';
import { FlagIcon } from './FlagIcon';
import { ThemeSwitcher } from './ThemeSwitcher';

interface HeaderProps {
  stats: {
    total: number;
    healthy: number;
    issues: number;
  };
}

export function Header({ stats }: HeaderProps) {
  void stats;
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();

  // 语言下拉菜单状态
  const [showMobileLangMenu, setShowMobileLangMenu] = useState(false);
  const [showDesktopLangMenu, setShowDesktopLangMenu] = useState(false);

  // 获取当前语言，使用类型守卫确保类型安全
  const currentLang: SupportedLanguage = isSupportedLanguage(i18n.language) ? i18n.language : 'zh-CN';
  const langPath = LANGUAGE_PATH_MAP[currentLang];
  const homePath = langPath ? `/${langPath}` : '/';
  const methodProviderPath = langPath ? `/${langPath}/p/claudecn-gpt` : '/p/claudecn-gpt';
  const methodPath = `${methodProviderPath}?service=cc&channel=78%3AClaudeCN-gpt`;
  const searchParams = new URLSearchParams(location.search);
  const isHomeActive = location.pathname === homePath || location.pathname === `${homePath}/`;
  const isMethodActive =
    location.pathname === methodProviderPath &&
    searchParams.get('service') === 'cc' &&
    searchParams.get('channel') === '78:ClaudeCN-gpt';

  /**
   * 处理语言切换
   *
   * 逻辑：
   * 1. 移除当前语言的路径前缀（如果有）
   * 2. 添加新语言的路径前缀（中文除外）
   * 3. 保留查询参数和 hash
   * 4. 导航到新路径并更新 i18n 语言状态
   *
   * 示例：
   * - 中文 → 英文：/ → /en/
   * - 英文 → 俄语：/en/docs → /ru/docs
   * - 俄语 → 中文：/ru/docs → /docs
   */
  const handleLanguageChange = (newLang: SupportedLanguage) => {
    // 获取当前语言，使用类型守卫确保类型安全
    const rawLang = i18n.language;
    const currentLang: SupportedLanguage = isSupportedLanguage(rawLang) ? rawLang : 'zh-CN';

    // 构建新路径
    let newPath = location.pathname;
    const queryString = location.search + location.hash;

    // 移除当前语言前缀（如果有）
    const currentPrefix = LANGUAGE_PATH_MAP[currentLang];
    if (currentPrefix && newPath.startsWith(`/${currentPrefix}`)) {
      newPath = newPath.substring(`/${currentPrefix}`.length) || '/';
    }

    // 添加新语言前缀（中文除外）
    const newPrefix = LANGUAGE_PATH_MAP[newLang];
    if (newPrefix) {
      newPath = `/${newPrefix}${newPath === '/' ? '' : newPath}`;
    }

    // 更新 i18n 语言状态
    i18n.changeLanguage(newLang);

    // 导航到新路径
    navigate(newPath + queryString);
  };

  return (
    <header className="flex flex-col gap-1 lg:gap-1.5 mb-2 border-b border-default/50 pb-1.5">
      {/* 第一行：Logo + 标题 + 操作按钮（桌面端右侧完整显示） */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 lg:gap-3">
            <div className="p-1.5 lg:p-2 bg-accent/10 rounded-lg border border-accent/20 flex-shrink-0 animate-heartbeat">
              <Activity className="w-5 h-5 lg:w-6 lg:h-6 text-accent" />
            </div>
            <div>
              <h1 className="text-2xl lg:text-3xl font-bold text-gradient-hero">
                RelayPulse
              </h1>
              {/* 桌面端 Tagline - 作为副标题 */}
              <p className="hidden lg:block text-secondary text-xs mt-0.5">
                {t('header.tagline')}
              </p>
            </div>
          </div>
          {/* 移动端 Tagline - 作为副标题 */}
          <p className="lg:hidden text-[10px] text-muted mt-1 pl-1 truncate">
            {t('header.tagline')}
          </p>
        </div>

        {/* 移动端：右上角操作区（语言 + 主题） */}
        <div className="flex items-center gap-1 lg:hidden flex-shrink-0">
          {/* 语言切换器 - 点击展开 */}
          <div className="relative">
            <button
              onClick={() => setShowMobileLangMenu(!showMobileLangMenu)}
              className="p-2 rounded-lg bg-elevated/50 hover:bg-muted/50 transition-all duration-200 focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none"
              aria-label={t('accessibility.changeLanguage')}
              aria-expanded={showMobileLangMenu}
            >
              <FlagIcon language={currentLang} className="w-5 h-auto" />
            </button>
            {/* 下拉菜单 */}
            {showMobileLangMenu && (
              <>
                {/* 点击外部关闭 */}
                <div
                  className="fixed inset-0 z-40"
                  onClick={() => setShowMobileLangMenu(false)}
                />
                <div
                  className="absolute right-0 mt-1 bg-elevated border border-default rounded-lg shadow-xl z-50"
                  role="listbox"
                  aria-label={t('accessibility.selectLanguage')}
                >
                  {SUPPORTED_LANGUAGES.map((lang) => (
                    <button
                      key={lang}
                      onClick={() => {
                        handleLanguageChange(lang);
                        setShowMobileLangMenu(false);
                      }}
                      className={`w-full p-2 flex items-center justify-center hover:bg-muted/50 transition-colors first:rounded-t-lg last:rounded-b-lg focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
                        currentLang === lang ? 'bg-accent/20' : ''
                      }`}
                      role="option"
                      aria-selected={currentLang === lang}
                      aria-label={LANGUAGE_NAMES[lang]?.native || lang}
                    >
                      <FlagIcon language={lang} className="w-5 h-auto flex-shrink-0" />
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>

          {/* 主题切换器 */}
          <ThemeSwitcher />
        </div>

        {/* 桌面端：右侧完整操作区（首页 / 检测方法 + 语言 + 主题） */}
        <div className="hidden lg:flex items-center gap-2 flex-shrink-0">
          <nav className="flex items-center gap-1 rounded-xl border border-default/60 bg-surface/50 px-1 py-1">
            <button
              onClick={() => navigate(homePath)}
              className={`px-3 py-1.5 text-sm font-medium rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
                isHomeActive ? 'text-primary bg-elevated/80' : 'text-secondary hover:text-primary hover:bg-elevated/60'
              }`}
            >
              首页
            </button>
            <button
              onClick={() => navigate(methodPath)}
              className={`px-3 py-1.5 text-sm font-medium rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
                isMethodActive ? 'text-primary bg-elevated/80' : 'text-secondary hover:text-primary hover:bg-elevated/60'
              }`}
            >
              检测方法
            </button>
          </nav>

          {/* 语言切换器 - 点击/键盘展开 */}
          <div className="relative inline-block">
            <button
              onClick={() => setShowDesktopLangMenu(!showDesktopLangMenu)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') setShowDesktopLangMenu(false);
              }}
              className="p-2 rounded-lg bg-elevated/50 hover:bg-muted/50 transition-all duration-200 focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none"
              aria-label={t('accessibility.changeLanguage')}
              aria-expanded={showDesktopLangMenu}
              aria-haspopup="listbox"
            >
              <FlagIcon language={currentLang} className="w-5 h-auto" />
            </button>
            {showDesktopLangMenu && (
              <>
                {/* 点击外部关闭 */}
                <div
                  className="fixed inset-0 z-40"
                  onClick={() => setShowDesktopLangMenu(false)}
                />
                <div
                  className="absolute left-0 mt-1 bg-elevated border border-default rounded-lg shadow-xl z-50"
                  role="listbox"
                  aria-label={t('accessibility.selectLanguage')}
                >
                  {SUPPORTED_LANGUAGES.map((lang) => (
                    <button
                      key={lang}
                      onClick={() => {
                        handleLanguageChange(lang);
                        setShowDesktopLangMenu(false);
                      }}
                      className={`w-full p-2 flex items-center justify-center hover:bg-muted/50 transition-colors first:rounded-t-lg last:rounded-b-lg focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
                        currentLang === lang ? 'bg-accent/20' : ''
                      }`}
                      role="option"
                      aria-selected={currentLang === lang}
                      aria-label={LANGUAGE_NAMES[lang]?.native || lang}
                    >
                      <FlagIcon language={lang} className="w-5 h-auto flex-shrink-0" />
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>

          {/* 主题切换器 */}
          <ThemeSwitcher />
        </div>
      </div>

      {/* 移动端：统一导航 */}
      <div className="flex items-center gap-1.5 min-[960px]:hidden">
        <div className="flex items-center gap-1 rounded-lg border border-default/60 bg-surface/50 px-1 py-1">
          <button
            onClick={() => navigate(homePath)}
            className={`px-2 py-1 rounded text-xs transition-colors focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
              isHomeActive ? 'text-primary bg-elevated/80' : 'text-secondary hover:text-primary'
            }`}
          >
            首页
          </button>
          <button
            onClick={() => navigate(methodPath)}
            className={`px-2 py-1 rounded text-xs transition-colors focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:outline-none ${
              isMethodActive ? 'text-primary bg-elevated/80' : 'text-secondary hover:text-primary'
            }`}
          >
            检测方法
          </button>
        </div>
      </div>
    </header>
  );
}
