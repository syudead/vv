// 画面の文言と書式の入口である（docs/design-docs/i18n.md）。
export { errorText, probeErrorText, scanErrorText } from "./errors";
export {
  formatDate,
  formatDateTime,
  formatNumber,
  formatRelative,
  selectPlural,
  type PluralForms,
} from "./format";
export { LOCALE } from "./intl";
export { t, type Messages, type MessageSource } from "./messages";
export { type UiText } from "./uiText";
