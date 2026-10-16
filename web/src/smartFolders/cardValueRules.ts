import { type LibrarySmartFolder, MAX_SMART_FOLDER_CARDS } from "../api/smartFolders";

// スマートフォルダのカードの値を読む規則（cardValues.tsx）。ロジックのテストで確かめる。

/** CardValue は読めたカードの値か、答えに入らなかった（無いか見せない）フォルダの "gone" である。 */
export type CardValue = LibrarySmartFolder | "gone";

/** chunks は `ids` を 1 回の読み出しで名指せる数ずつに分ける。 */
export function chunks(ids: readonly number[]): number[][] {
  const result: number[][] = [];
  for (let start = 0; start < ids.length; start += MAX_SMART_FOLDER_CARDS) {
    result.push(ids.slice(start, start + MAX_SMART_FOLDER_CARDS));
  }
  return result;
}

/**
 * withAnswer は読み出し `asked` の答え `cards` を値に重ねる。答えに入らなかったフォルダは
 * "gone"（カードを消す）にする。
 */
export function withAnswer(
  current: ReadonlyMap<number, CardValue>,
  asked: readonly number[],
  cards: readonly LibrarySmartFolder[],
): ReadonlyMap<number, CardValue> {
  const next = new Map(current);
  for (const id of asked) next.set(id, "gone");
  for (const card of cards) next.set(card.id, card);
  return next;
}
