/**
 * 公开提交向导共享样式 —— 单一样式源。
 *
 * 同时服务「申请收录」(OnboardingPage) 与「申请变更」(ChangeRequestPage) 两个面向公开
 * 访客的多步向导：它们视觉上应是同一个产品。这里把输入 / 下拉 / 标签 / 提示 / 主次按钮
 * 的 className 收敛成常量，任一调整改一处即两个向导同步生效，杜绝逐字重复带来的漂移。
 *
 * 规格为 roomy（px-4 py-2 / rounded-lg / ring-2）；后台 admin 的密集表单另有
 * components/admin/fieldStyles（dense 设计语言），不在此处共享。
 */

/**
 * 文本 / URL / 密码输入框（含占位符样式）。
 * error=true 时切到危险色边框，其余情况用默认边框。
 */
export const inputClass = (error = false): string =>
  'w-full px-4 py-2 bg-surface border rounded-lg text-primary placeholder-muted ' +
  'focus:outline-none focus:ring-2 focus:ring-accent disabled:opacity-50 ' +
  (error ? 'border-danger' : 'border-muted');

/** 下拉选择框（无占位符着色）。 */
export const selectClass =
  'w-full px-4 py-2 bg-surface border border-muted rounded-lg text-primary ' +
  'focus:outline-none focus:ring-2 focus:ring-accent disabled:opacity-50';

/** 字段标签 / fieldset legend。 */
export const labelClass = 'block text-sm font-medium text-primary mb-2';

/** 字段下方的说明 / 提示文字。 */
export const hintClass = 'mt-1 text-xs text-secondary';

/** 主操作按钮（下一步 / 提交）：实心强调色。调用方可附加 w-full / flex-1 / justify-center。 */
export const primaryButtonClass =
  'flex items-center gap-2 px-6 py-3 bg-accent text-white rounded-lg font-medium ' +
  'hover:bg-accent-strong transition-colors disabled:opacity-50 disabled:cursor-not-allowed';

/** 次要 / 返回按钮：描边低强调。 */
export const secondaryButtonClass =
  'flex items-center gap-2 px-6 py-3 bg-surface border border-muted text-secondary ' +
  'rounded-lg hover:bg-elevated transition-colors';
