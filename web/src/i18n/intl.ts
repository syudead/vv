import { asUiText, type UiText } from "./uiText";

/**
 * LOCALE は画面の言語である。起動時に決まる 1 つの定数で、切り替えない
 * （specs/023-english-i18n/research.md R-1）。カタログ（messages.ts）と書式関数は
 * どちらもこれを使う。
 */
export const LOCALE = "en-US";

type Decorate = (text: string) => string;

const identity: Decorate = (text) => text;
let decorate: Decorate = identity;

/**
 * decorated は書式関数の出力を UiText にする。疑似ロケールの間は印を付けるので、
 * 数や日時も画面テストで「カタログ由来」と分かる（pseudo.ts）。
 */
export function decorated(text: string): UiText {
  return asUiText(decorate(text));
}

/** setDecoration は疑似ロケールの印の付け方を差し替える。null で元に戻す。テスト専用。 */
export function setDecoration(next: Decorate | null): void {
  decorate = next ?? identity;
}

const numberFormat = new Intl.NumberFormat(LOCALE);
const pluralRules = new Intl.PluralRules(LOCALE);
const listFormat = new Intl.ListFormat(LOCALE, { type: "conjunction" });

/** formatNumber は数を桁区切り付きで表す。 */
export function formatNumber(value: number): UiText {
  return decorated(numberFormat.format(value));
}

const decimalFormats = new Map<string, Intl.NumberFormat>();

/**
 * formatDecimal は小数を、小数点以下を最小 min 桁・最大 max 桁で表す（例: 29.97、12.3）。
 * 末尾の 0 は min 桁まで落とす。
 */
export function formatDecimal(value: number, max: number, min = 0): UiText {
  const key = `${String(min)}-${String(max)}`;
  let format = decimalFormats.get(key);
  if (format === undefined) {
    format = new Intl.NumberFormat(LOCALE, {
      minimumFractionDigits: min,
      maximumFractionDigits: max,
    });
    decimalFormats.set(key, format);
  }
  return decorated(format.format(value));
}

const languageNames = new Intl.DisplayNames(LOCALE, { type: "language" });

/**
 * formatLanguage はファイルの言語の印（`jpn`・`eng`・`ja-JP`）を言語の名前にする
 * （specs/056-media-info/research.md R-8）。`und` と空は null（言語を出さない）。名前は
 * 書式の出力なので疑似ロケールの印を付ける。名前にできない印は、ファイルの値のまま返す。
 */
export function formatLanguage(code: string): UiText | string | null {
  const trimmed = code.trim();
  if (trimmed === "" || trimmed.toLowerCase() === "und") return null;
  let name: string | undefined;
  try {
    name = languageNames.of(trimmed);
  } catch {
    return trimmed;
  }
  return name === undefined || name === trimmed ? trimmed : decorated(name);
}

/** PluralForms は単数・複数の形である。英語は one と other だけを使う。 */
export interface PluralForms<T> {
  one: T;
  other: T;
}

/**
 * selectPlural は、件数に合う形を `Intl.PluralRules` で選ぶ。英語に無い区分
 * （zero・two・few・many）は other に寄せる。
 */
export function selectPlural<T>(count: number, forms: PluralForms<T>): T {
  return pluralRules.select(count) === "one" ? forms.one : forms.other;
}

/** formatList は項目を「A, B and C」の形でつなぐ。項目はカタログの文言を渡す。 */
export function formatList(items: readonly string[]): string {
  return listFormat.format(items);
}
