/** The tag list as it was when a card opened a tag's page, so Back returns to the same cards and position. */
export interface TagListSnapshot<P> {
  /** The serialized URL criteria the list was showing. */
  key: string;
  page: P;
  /** The raw search text the loaded pages were read with. */
  search: string;
  scrollY: number;
}

/** One snapshot at a time, as the library's list snapshot: the list holds at most a few thousand tags. */
let held: TagListSnapshot<unknown> | undefined;

export function saveTagListSnapshot<P>(snapshot: TagListSnapshot<P>): void {
  held = snapshot;
}

/** Return the snapshot for key without consuming it, so StrictMode's repeated mount restores the same list. */
export function takeTagListSnapshot<P>(key: string): TagListSnapshot<P> | undefined {
  if (held === undefined || held.key !== key) return undefined;
  return held as TagListSnapshot<P>;
}

/** Any tag write drops it: a merge, detach or kind change can move cards across pages. */
export function clearTagListSnapshot(): void {
  held = undefined;
}
