declare const uiTextBrand: unique symbol;

/**
 * UiText は画面に出す固定の文言である。カタログ（messages.ts）と書式関数（format.ts）
 * だけが作る。部品の文言の props など、固定の文言を運ぶ値はこの型にするので、
 * カタログを通さない英語のリテラルは型検査で落ちる（specs/023-english-i18n/research.md R-3）。
 * 利用者のデータ（動画名、タグ名）を運ぶ値は `string` のままにする。
 */
export type UiText = string & { readonly [uiTextBrand]: true };

/** asUiText は i18n の内側で、組み立てた文字列を UiText として渡す。外からは使わない。 */
export function asUiText(text: string): UiText {
  return text as UiText;
}

/**
 * untranslated は、まだ英語のカタログへ移していないディレクトリ（web/eslint.config.js の
 * 除外の一覧）が、UiText を受ける部品へ文言を渡すための一時的な出口である。各領域の単位が
 * 自分のディレクトリの呼び出しを消し、最後の単位でこの関数も消す。除外の外で使うと
 * ESLint が報告する。
 */
export function untranslated(text: string): UiText {
  return text as UiText;
}
