import type { LibraryItem } from "../api/client";
import type { FavoritesChange } from "../api/favorites";
import {
  type HomeShelf,
  type HomeShelfItems,
  type HomeShelfKey,
  MAX_HOME_SHELF_KEYS,
} from "../api/home";
import { videoInGroupPath } from "../groups/groupPaths";
import { t, type UiText } from "../i18n";
import { LIBRARY_PATH } from "../library/libraryPath";
import {
  smartFolderPagePath,
  videoInSmartFolderPath,
} from "../smartFolders/smartFolderPaths";

// ホームの棚の規則（specs/057-home/ui-design.md「Home」、research.md R-12・R-14）。画面の部品から
// 外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** ShelfState は棚 1 つの項目の読み込みの状態である。無い棚はまだ読みに行っていない。 */
export type ShelfState =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; items: LibraryItem[]; total: number };

/** ShelfStates は棚の鍵ごとの状態である。 */
export type ShelfStates = ReadonlyMap<HomeShelfKey, ShelfState>;

/** shelfTitle は棚の見出しの名前である。フォルダの棚はフォルダの名前、決まった棚は画面の名前。 */
export function shelfTitle(shelf: HomeShelf): string {
  return shelf.kind === "folder" ? shelf.name : t.home.fixed[shelf.kind];
}

/** shelfHeadingPath は見出しが開く場所である。フォルダの棚だけがフォルダのページを開く。 */
export function shelfHeadingPath(shelf: HomeShelf): string | undefined {
  return shelf.kind === "folder" && shelf.folderId !== undefined
    ? smartFolderPagePath(shelf.folderId)
    : undefined;
}

/**
 * seeAllPath は棚の「See all」が開く場所である。おすすめには無い（undefined）。最近追加は
 * ライブラリの既定の並び（追加日の新しい順）をそのまま開く。
 */
export function seeAllPath(shelf: HomeShelf): string | undefined {
  switch (shelf.kind) {
    case "folder":
      return shelfHeadingPath(shelf);
    case "continue":
      return "/history";
    case "recent":
      return LIBRARY_PATH;
    case "unshelved":
      return `${LIBRARY_PATH}?unshelved=1`;
    case "recommended":
      return undefined;
  }
}

/**
 * shelfItemPath は棚のカードが開く再生画面の場所である（R-14）。動画はその再生画面で、
 * フォルダの棚では `?smartFolder` を付けて次の動画を棚の並びに従わせる。グループは開いている
 * 動画を `?group` 付きで開く。棚の項目にフォルダは無い。
 */
export function shelfItemPath(
  shelf: HomeShelf,
  item: Exclude<LibraryItem, { kind: "smartFolder" }>,
): string {
  if (item.kind === "group") {
    return videoInGroupPath(item.group.openVideoId, item.group.id);
  }
  return shelf.kind === "folder" && shelf.folderId !== undefined
    ? videoInSmartFolderPath(item.video.id, shelf.folderId)
    : `/videos/${String(item.video.id)}`;
}

/** shelfRowHeightPx は棚の行 1 つの高さの見積もり（見出し、card-0 のカード、棚の間）である。 */
const shelfRowHeightPx = 240;

/**
 * shelfBatchSize は 1 回に読む棚の数である。画面に入る棚とその次の 2 つで、8 を超えない
 * （R-12）。棚の行はまだ描いていないので、高さは見積もる。
 */
export function shelfBatchSize(viewportHeight: number): number {
  const fit = Math.max(1, Math.ceil(viewportHeight / shelfRowHeightPx));
  return Math.min(MAX_HOME_SHELF_KEYS, fit + 2);
}

/** settled は棚の状態が決まっている（項目が届いたか、失敗した）かである。 */
function settled(state: ShelfState | undefined): boolean {
  return state !== undefined && state.status !== "loading";
}

/** shown は棚を描くかである。項目の無い棚は描かない。 */
function shown(state: ShelfState | undefined): boolean {
  return (
    state !== undefined &&
    (state.status === "failed" || (state.status === "ready" && state.items.length > 0))
  );
}

/**
 * nextShelfBatch は次に読む棚の鍵である。状態の決まっていない最初の棚（先頭）から、まだ
 * 読みに行っていない棚を上から `count` 個まで選ぶ。先頭を読んでいる間は何も選ばない。
 */
export function nextShelfBatch(
  shelves: readonly HomeShelf[],
  states: ShelfStates,
  count: number,
): HomeShelfKey[] {
  const frontier = shelves.findIndex((shelf) => !settled(states.get(shelf.key)));
  const first = shelves[frontier];
  if (first === undefined || states.get(first.key) !== undefined) return [];
  const keys: HomeShelfKey[] = [];
  for (const shelf of shelves.slice(frontier)) {
    if (keys.length >= Math.min(count, MAX_HOME_SHELF_KEYS)) break;
    if (states.get(shelf.key) === undefined) keys.push(shelf.key);
  }
  return keys;
}

/** DrawnShelf は描く棚 1 つと、その状態である。`frontier` は読み込み中の形で描く先頭の棚。 */
export interface DrawnShelf {
  shelf: HomeShelf;
  state: ShelfState;
  frontier: boolean;
}

/**
 * drawnShelves は描く棚を上から順に選ぶ。状態の決まった棚のうち項目のあるものと失敗したものを
 * 描き、状態の決まっていない最初の棚を読み込み中の形で描いて、その下は描かない。項目の無い棚は
 * 描かないので、その下の棚が詰めても描いてある行は動かない（ui-design.md「Home」）。
 */
export function drawnShelves(
  shelves: readonly HomeShelf[],
  states: ShelfStates,
): DrawnShelf[] {
  const drawn: DrawnShelf[] = [];
  for (const shelf of shelves) {
    const state = states.get(shelf.key);
    if (!settled(state)) {
      drawn.push({ shelf, state: { status: "loading" }, frontier: true });
      break;
    }
    if (shown(state) && state !== undefined)
      drawn.push({ shelf, state, frontier: false });
  }
  return drawn;
}

/**
 * withShelfItems は棚の項目の応答を状態に重ねる。名指した鍵が応答に無ければ、その棚は
 * もう無いので項目 0 件として描かない。
 */
export function withShelfItems(
  states: ShelfStates,
  keys: readonly HomeShelfKey[],
  shelves: readonly HomeShelfItems[],
): Map<HomeShelfKey, ShelfState> {
  const next = new Map(states);
  const byKey = new Map(shelves.map((shelf) => [shelf.key, shelf]));
  for (const key of keys) {
    const found = byKey.get(key);
    next.set(key, {
      status: "ready",
      items: found?.items ?? [],
      total: found?.total ?? 0,
    });
  }
  return next;
}

/** withShelfStatus は名指した鍵の状態を `state` にする。 */
export function withShelfStatus(
  states: ShelfStates,
  keys: readonly HomeShelfKey[],
  state: ShelfState,
): Map<HomeShelfKey, ShelfState> {
  const next = new Map(states);
  for (const key of keys) next.set(key, state);
  return next;
}

/**
 * loadedShelfKeys は読み直す棚の鍵である。今の棚の一覧にあって、もう状態の決まった棚を、
 * 1 回に読める数ずつに分ける。
 */
export function loadedShelfKeys(
  shelves: readonly HomeShelf[],
  states: ShelfStates,
): HomeShelfKey[][] {
  const keys = shelves
    .map((shelf) => shelf.key)
    .filter((key) => settled(states.get(key)));
  const batches: HomeShelfKey[][] = [];
  for (let start = 0; start < keys.length; start += MAX_HOME_SHELF_KEYS) {
    batches.push(keys.slice(start, start + MAX_HOME_SHELF_KEYS));
  }
  return batches;
}

/**
 * withFavorites は付け外したお気に入りを棚の項目に重ねる（取り直さない）。動画は反映された
 * 動画だけ、グループは要求に含めたグループを変える（api/favorites.ts の FavoritesChange）。
 */
export function withFavorites(
  states: ShelfStates,
  change: FavoritesChange,
): Map<HomeShelfKey, ShelfState> {
  const videos = new Set(change.videoIds);
  const groups = new Set(change.groupIds);
  const mark = (item: LibraryItem): LibraryItem => {
    if (item.kind === "video" && videos.has(item.video.id)) {
      return { ...item, video: { ...item.video, favorite: change.favorite } };
    }
    if (item.kind === "group" && groups.has(item.group.id)) {
      return { ...item, group: { ...item.group, favorite: change.favorite } };
    }
    return item;
  };
  const next = new Map<HomeShelfKey, ShelfState>();
  for (const [key, state] of states) {
    next.set(
      key,
      state.status === "ready" ? { ...state, items: state.items.map(mark) } : state,
    );
  }
  return next;
}
