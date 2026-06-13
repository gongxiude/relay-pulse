// i18n locale 完整性守护。本仓库历史上反复出现两类 i18n 缺口（多次只能靠人工
// 自查 + playwright 才发现，见 v2.41.x / v2.48.x 发版记录），本测试把两类都钉成
// CI 硬闸（跑在已有的「Frontend test」/ vitest 步骤里，零新基建）：
//
//   A 类「加到部分 locale」：新键只落 2/4 个 locale → 缺失 locale 在 UI 外显原始
//        key（如 statusQuery.idLabel）。locale-vs-locale 键集比对可抓。
//   B 类「全 locale 都缺」：组件写 t(key, { defaultValue: '中文' }) 兜底，但键从未
//        加进任何 locale → 所有非中文用户都看到中文默认值。这类对 A 类的 parity
//        比对完全失明（四 locale 都没有 = 无差异），只能扫源码里的 t('字面键')
//        反向比对 locale 键集才能发现。
//
// 纯 node（文件系统读 JSON + 正则扫源码），不引 DOM、不引新依赖。
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative } from 'node:path';
import { describe, it, expect } from 'vitest';

const LOCALES = ['zh-CN', 'en-US', 'ja-JP', 'ru-RU'] as const;

const i18nDir = dirname(fileURLToPath(import.meta.url));
const localesDir = join(i18nDir, 'locales');
const srcDir = join(i18nDir, '..'); // frontend/src

type JsonObject = Record<string, unknown>;

/** 把嵌套 locale 对象拍平成点号路径键集（数组当叶子）。 */
function flattenKeys(obj: JsonObject, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([key, value]) =>
    value && typeof value === 'object' && !Array.isArray(value)
      ? flattenKeys(value as JsonObject, `${prefix}${key}.`)
      : [`${prefix}${key}`],
  );
}

function loadLocaleKeys(locale: string): Set<string> {
  const raw = readFileSync(join(localesDir, `${locale}.json`), 'utf8');
  return new Set(flattenKeys(JSON.parse(raw) as JsonObject));
}

const keySetByLocale = Object.fromEntries(
  LOCALES.map((locale) => [locale, loadLocaleKeys(locale)]),
) as Record<(typeof LOCALES)[number], Set<string>>;

const unionKeys = new Set(LOCALES.flatMap((locale) => [...keySetByLocale[locale]]));

describe('i18n locale parity（A 类：键集必须四 locale 完全一致）', () => {
  for (const locale of LOCALES) {
    it(`${locale} 不缺其他 locale 拥有的任何键`, () => {
      const missing = [...unionKeys].filter((key) => !keySetByLocale[locale].has(key)).sort();
      expect(missing, `${locale} 缺失以下键（请补齐，或确认其他 locale 是否多余）`).toEqual([]);
    });
  }
});

/** 递归收集 src 下的 .ts/.tsx 源码（排除 node_modules、locales/ 与测试文件本身）。 */
function collectSourceFiles(dir: string): string[] {
  const files: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!/node_modules/.test(fullPath) && fullPath !== localesDir) {
        files.push(...collectSourceFiles(fullPath));
      }
    } else if (/\.(ts|tsx)$/.test(entry.name) && !/\.(test|spec)\./.test(entry.name)) {
      files.push(fullPath);
    }
  }
  return files;
}

describe('i18n code coverage（B 类：t() 字面键必须在 locale 中存在）', () => {
  it("所有 t('literal.key') 引用的键都已落 locale（含 defaultValue 兜底的也不例外）", () => {
    // 只匹配字面量首参（单/双引号）。模板字面量动态键 t(`a.${x}`) 无法静态解析，
    // 故跳过——不会误报，但也守不住动态键，这类仍需人工保证子键齐全。
    const literalKeyCall = /[^a-zA-Z0-9_]t\(\s*(['"])([a-zA-Z0-9_.]+)\1/g;
    const missing = new Map<string, string[]>();

    for (const file of collectSourceFiles(srcDir)) {
      const text = readFileSync(file, 'utf8');
      let match: RegExpExecArray | null;
      while ((match = literalKeyCall.exec(text))) {
        const key = match[2];
        if (!unionKeys.has(key)) {
          const rel = relative(srcDir, file);
          const seen = missing.get(key) ?? [];
          if (!seen.includes(rel)) seen.push(rel);
          missing.set(key, seen);
        }
      }
    }

    const report = [...missing.entries()].map(([key, fileList]) => `${key} <- ${fileList.join(', ')}`);
    expect(
      report,
      '以下键被 t() 引用但不在任何 locale（B 类债：仅靠 defaultValue 兜底，非中文用户会看到中文默认值）',
    ).toEqual([]);
  });
});
