export interface ItemPage<I> {
  items: readonly I[];
  nextCursor?: string;
}

const maxRefreshPages = 50;

/** Refill to the previously shown count to preserve scroll position; append deduplicates overlapping page boundaries. */
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
