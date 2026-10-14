import { t, type UiText } from "../i18n";

// 人物と団体の名前の規則（specs/047-video-work-info/data-model.md「Fields」、
// ui-design.md「Picker」）。前後の空白を除いて 1〜200 符号位置で、制御文字を含まない。

const controlCharPattern = /\p{Cc}/u;

/** creditNameMaxLength は人物・団体の名前に許す長さ（符号位置の数）である。 */
export const creditNameMaxLength = 200;

/**
 * creditNameReason は、打つたびに確かめる名前の理由を返す。空は打っている間は理由を
 * 出さない（Enter を押したときに CreditCommand が出す）。
 */
export function creditNameReason(raw: string): UiText | null {
  if (controlCharPattern.test(raw)) return t.creditName.controlCharacters;
  if (Array.from(raw.trim()).length > creditNameMaxLength) {
    return t.creditName.tooLong(creditNameMaxLength);
  }
  return null;
}
