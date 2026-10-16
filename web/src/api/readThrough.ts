/** ItemPage は続きを持つ項目の 1 ページである（`nextCursor` は続きがあるときだけ入る）。 */
export interface ItemPage<I> {
  items: readonly I[];
  nextCursor?: string;
}

/** maxRefreshPages は読み直しで続きのページを辿る回数の上限である（100 件ずつ）。 */
const maxRefreshPages = 50;

/**
 * readThrough は最初のページ `first` に続けて、項目が `atLeast` 件そろうか続きが無くなるまで
 * `more` で続きのページを辿る。読み直しで、それまで並んでいた分を減らさずに入れ替えるため
 * である（スクロールの位置を保つ）。続きは `append` で足す（ページの境目で同じ項目が二度
 * 返ったときに足さない）。グループのページとスマートフォルダのページが共有する。
 */
export async function readThrough<I>(
  first: ItemPage<I>,
  more: (cursor: string) => Promise<ItemPage<I>>,
  append: (current: readonly I[], page: readonly I[]) => I[],
  atLeast: number,
): Promise<{ items: readonly I[]; nextCursor?: string }> {
  let items: readonly I[] = first.items;
  let nextCursor = first.nextCursor;
  for (let page = 0; page < maxRefreshPages; page += 1) {
    if (nextCursor === undefined || items.length >= atLeast) break;
    const next = await more(nextCursor);
    items = append(items, next.items);
    nextCursor = next.nextCursor;
  }
  return { items, nextCursor };
}
