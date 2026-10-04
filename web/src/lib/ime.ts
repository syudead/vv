import type { KeyboardEvent } from "react";

// IME の変換中に打った特別なキーの判定。タグの名前・検索・窓の Esc など、文字を打つ
// 入力の操作が共有する（もとは ui/Combobox にあった）。

/**
 * isComposingKey は、IME の変換中に打った特別なキー（Enter・Esc・矢印）かを、
 * `isComposing`・`keyCode` の組から判定する（keyCode 229 は isComposing を
 * 実装しない古いブラウザ向けの後方互換）。React の合成イベントとブラウザの
 * 生のイベントの両方から使えるよう、値だけを受け取る形にしている。
 */
function isComposingKey(isComposing: boolean, keyCode: number): boolean {
  return isComposing || keyCode === 229;
}

/**
 * isComposingKeyEvent は、React の合成イベント（`onKeyDown` など）が、IME の
 * 変換中に打った特別なキーかを返す。変換の確定・移動のためのキーで、
 * combobox やタグの名前を打つほかの入力（管理画面の作成・改名の入力、検索の
 * 入力）の操作にしてはいけない（`web/src/player/keyboard.ts` と同じ判定）。
 */
export function isComposingKeyEvent(event: KeyboardEvent<HTMLInputElement>): boolean {
  return isComposingKey(event.nativeEvent.isComposing, event.nativeEvent.keyCode);
}

/**
 * isComposingNativeKeyEvent は、`document.addEventListener` などで受け取る
 * 生の `KeyboardEvent` に対する同じ判定である。窓の Esc の扱いが使う
 * （IME の変換中の Esc で窓ごと閉じてしまわないため）。
 */
export function isComposingNativeKeyEvent(event: globalThis.KeyboardEvent): boolean {
  return isComposingKey(event.isComposing, event.keyCode);
}
