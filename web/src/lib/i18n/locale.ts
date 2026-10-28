// 界面语言的单一来源。
//
// 现在只有一处需要：日期格式化与日历组件。之所以单独抽一个模块而不是就地写死，
// 是因为后面真要接完整 i18n（next-intl / 字典文件）时，改这一个文件就够了，
// 不用再去翻每个组件的 format() 调用。
//
// 注意：zh-CN 与 zh-TW 的差异不只是词表，日期/数字格式也不同。繁体界面不要靠
// 字符串替换硬转，要各自配 locale。

import { zhCN } from "date-fns/locale";
import type { Locale as DateFnsLocale } from "date-fns";

/** BCP 47 语言标签，直接给 <html lang>。 */
export const UI_LANG = "zh-CN";

/** date-fns / react-day-picker 用的 locale 对象。 */
export const UI_DATE_LOCALE: DateFnsLocale = zhCN;

/** 少数需要手动拼的中文文案（等接入完整 i18n 后由字典接管）。 */
export const UI_TEXT = {
  selectDate: "选择日期",
  selectDateRange: "选择日期区间",
} as const;
