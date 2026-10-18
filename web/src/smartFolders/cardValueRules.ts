import { type LibrarySmartFolder, MAX_SMART_FOLDER_CARDS } from "../api/smartFolders";

export type CardValue = LibrarySmartFolder | "gone";

export function chunks(ids: readonly number[]): number[][] {
  const result: number[][] = [];
  for (let start = 0; start < ids.length; start += MAX_SMART_FOLDER_CARDS) {
    result.push(ids.slice(start, start + MAX_SMART_FOLDER_CARDS));
  }
  return result;
}

/** Mark omitted requested IDs as gone so their cards disappear. */
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

/** Keep only IDs for which this is the latest request; later requests own their pending results. */
export function latestAnswer(
  asked: readonly number[],
  cards: readonly LibrarySmartFolder[],
  isLatest: (id: number) => boolean,
): { asked: number[]; cards: LibrarySmartFolder[] } {
  return {
    asked: asked.filter(isLatest),
    cards: cards.filter((card) => isLatest(card.id)),
  };
}
