import { t, type UiText } from "../i18n";

// タグの名前の規則。名前を打つすべての入力が共有する。

export const newlinePattern = /[\r\n]/;
// タグ名では C0/C1 制御文字（U+0000–U+001F・U+007F–U+009F、一般カテゴリ Cc）をすべて拒む。
const controlCharPattern = /\p{Cc}/u;

/** tagNameMaxLength はタグ名に許す長さ（符号位置の数）である。 */
const tagNameMaxLength = 100;

/** codePointLength は前後の空白を除いた符号位置の数を返す（`length` は使わない）。 */
function codePointLength(value: string): number {
  return Array.from(value.trim()).length;
}

/**
 * nameReason は、入力のたびに確かめる名前の検証理由を返す。空や空白だけは
 * 打っている間は理由を出さない（ui-design.md「Combobox」名前の検証）。タグの
 * 名前を打つすべての入力（ui/TagCommand、管理画面の作成・改名・シノニムの
 * 追加、選択バーのタグの検索）が同じ規則を使うので外へ公開する（ui-design.md「Combobox」末尾）。
 */
export function nameReason(raw: string): UiText | null {
  if (controlCharPattern.test(raw)) return t.tagName.controlCharacters;
  const length = codePointLength(raw);
  if (length > tagNameMaxLength) return t.tagName.tooLong(tagNameMaxLength, length);
  return null;
}
