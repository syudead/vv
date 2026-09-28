import type { UiText } from "../i18n";
import type { FolderRef, LibraryGroup, LibraryItem, TagRef, Video } from "./client";
import { folderRefKey, groupRef, itemKey, itemVideos, videoItem } from "./libraryItems";
import { applyTagToTags } from "./tagOrder";

/**
 * mergeRefreshed は取り直した1件を、一覧に出ている項目へ重ねる。
 *
 * 題名と大きさは一覧側の値を残す。フォルダ画面の項目は、そのフォルダにある
 * 所在の題名と大きさを持ち、1件の取得（代表の所在）とは違うことがあるためである。
 */
function mergeRefreshed(current: Video, refreshed: Video): Video {
  return { ...current, ...refreshed, title: current.title, sizeBytes: current.sizeBytes };
}

/**
 * VideosSeed は復元された一覧の初期状態である。
 *
 * 与えると 1 ページ目を**取りに行かない**。再生画面から戻るたびに読み直すと、
 * 300 件まで読んだ状態を戻すのに 5 ページ分の往復が要る。
 */
export interface VideosSeed {
  items: LibraryItem[];
  total: number;
  cursor?: string;
  hasMore: boolean;
  /**
   * 控えを取った後にメンバーが変わったグループの項目（ListSnapshot.staleGroups）。
   * 戻ったときにこれを取り直す。
   */
  staleGroups?: readonly FolderRef[];
}

/**
 * appendUnique は続きのページから、既に出ている項目を捨てて足す（list-api.md §5）。
 * 動画の項目は動画の id、グループの項目はフォルダで比べる（itemKey）。
 */
function appendUnique(current: LibraryItem[], next: LibraryItem[]): LibraryItem[] {
  const seen = new Set(current.map(itemKey));
  const added: LibraryItem[] = [];
  for (const item of next) {
    const key = itemKey(item);
    if (seen.has(key)) continue;
    seen.add(key);
    added.push(item);
  }
  return added.length === 0 ? current : [...current, ...added];
}

/**
 * mapVideos は動画の項目のうち `match` に当たるものだけを `update` で書き換える。
 * 当たる項目が無ければ受け取った配列をそのまま返す（描画し直さないため）。
 */
function mapVideos(
  items: LibraryItem[],
  match: (video: Video) => boolean,
  update: (video: Video) => Video,
): LibraryItem[] {
  if (!items.some((item) => item.kind === "video" && match(item.video))) return items;
  return items.map((item) =>
    item.kind === "video" && match(item.video) ? videoItem(update(item.video)) : item,
  );
}

/** shownVideoIds は動画の項目の id である（グループのメンバーは含まない）。 */
export function shownVideoIds(items: readonly LibraryItem[]): number[] {
  return itemVideos(items).map((video) => video.id);
}

export interface VideosData {
  items: LibraryItem[];
  total: number;
  cursor: string | undefined;
  hasMore: boolean;
  /** inconsistent は異なる時点のページを安全に結合できなかったことを表す。 */
  inconsistent: boolean;
  /**
   * missingTagIds は、直近の要求で `tag` に指定したがもう無かった id である
   * （specs/014-video-tags/contracts/tags-api.md §5）。folder を渡す（`tag` を
   * 送らない）ときは常に空。
   */
  missingTagIds: number[];
}

export type VideosDataAction =
  | { type: "clear" }
  | { type: "stop" }
  | { type: "progress"; videoId: number; progress: NonNullable<Video["progress"]> }
  | {
      type: "tags";
      videoIds: readonly number[];
      tag: TagRef;
      action: "add" | "remove";
    }
  | { type: "visibility"; videoIds: readonly number[]; isPublic: boolean }
  | { type: "refresh"; videoId: number; video: Video }
  | { type: "remove"; videoId: number }
  | { type: "refreshGroup"; folderKey: string; group: LibraryGroup }
  | { type: "removeGroup"; folderKey: string }
  | {
      type: "page";
      page: {
        items: LibraryItem[];
        total: number;
        nextCursor?: string;
        missingTagIds?: number[];
      };
      replace: boolean;
    };

export function videosDataReducer(
  state: VideosData,
  action: VideosDataAction,
): VideosData {
  switch (action.type) {
    case "clear":
      return {
        items: [],
        total: 0,
        cursor: undefined,
        hasMore: true,
        inconsistent: false,
        missingTagIds: [],
      };
    case "stop":
      return state.hasMore ? { ...state, hasMore: false } : state;
    // 再生位置・タグ・公開・取り直しは動画の項目にだけ直接重ねる。グループの値は
    // メンバーから数えるので、メンバーの変化ではグループを1件取り直す（useVideos の refreshGroups）。
    case "progress": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => ({ ...video, progress: action.progress }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "tags": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id),
        (video) => ({
          ...video,
          tags: applyTagToTags(video.tags, action.tag, action.action),
        }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "visibility": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id) && video.public !== action.isPublic,
        (video) => ({ ...video, public: action.isPublic }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "refresh": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => mergeRefreshed(video, action.video),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "remove": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "video" && item.video.id === action.videoId;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.filter((item) => !isTarget(item)),
        total: Math.max(0, state.total - 1),
      };
    }
    case "refreshGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && folderRefKey(groupRef(item.group)) === action.folderKey;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.map((item) =>
          isTarget(item) ? { kind: "group", group: action.group } : item,
        ),
      };
    }
    case "removeGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && folderRefKey(groupRef(item.group)) === action.folderKey;
      if (!state.items.some(isTarget)) return state;
      // total は次に一覧を読むまでそのままにする（ui-design.md「Refresh and removal」）。
      return { ...state, items: state.items.filter((item) => !isTarget(item)) };
    }
    case "page": {
      const missingTagIds = action.page.missingTagIds ?? [];
      if (action.replace) {
        return {
          items: action.page.items,
          total: action.page.total,
          cursor: action.page.nextCursor,
          hasMore: action.page.nextCursor !== undefined,
          inconsistent: false,
          missingTagIds,
        };
      }
      const items = appendUnique(state.items, action.page.items);
      // ページ間で索引が縮むと、前のページにだけ残る項目と最新の total を
      // 安全に結合できない。古い整合した状態を保ち、先頭からの再読込を求める。
      if (items.length > action.page.total) {
        return { ...state, hasMore: false, inconsistent: true };
      }
      return {
        items,
        total: action.page.total,
        cursor: action.page.nextCursor,
        hasMore: action.page.nextCursor !== undefined,
        inconsistent: false,
        missingTagIds,
      };
    }
  }
}

/** VideosState は一覧の状態である。 */
export interface VideosState {
  /**
   * 読み込み済みの項目。動画の項目とグループの項目がある（グループはライブラリの一覧にだけ現れる）。
   */
  items: LibraryItem[];
  total: number;
  /** cursor は次のページの続き位置。控えを取るときに使う。 */
  cursor: string | undefined;
  /** hasMore は次のページがあるかどうか。 */
  hasMore: boolean;
  /** loading は最初の1ページを待っている間だけ true になる。 */
  loading: boolean;
  /** loadingMore は続きを読んでいる間 true になる。 */
  loadingMore: boolean;
  error: UiText | null;
  /**
   * notFound は folder を渡したときに、そのフォルダの動画の要求が 404 で
   * 返ったことを表す（検索中にフォルダが無くなった場合、
   * list-api.md §5「listFolderVideos でフォルダが無いとき」）。
   */
  notFound: boolean;
  /**
   * missingTagIds は、直近の要求の `tag` のうちもう無かった id である
   * （specs/014-video-tags/contracts/tags-api.md §5）。画面はこれを受けて、
   * もう無いことを伝え、タグの一覧を取り直し、URL から取り除く。
   */
  missingTagIds: number[];
  /** loadMore は次のページを読む。無限スクロールの観測点から呼ぶ。 */
  loadMore: () => void;
  /** retryLoadMore は失敗した続きのページを同じカーソルから再要求する。 */
  retryLoadMore: () => void;
  /** reload は先頭から読み直す。取り込みのあとに使う。 */
  reload: () => void;
  /**
   * staleGroups は、取り直しを求めたがまだ済んでいない表示中のグループの項目の
   * フォルダを返す（取り直しの待ち・途中・一時的な失敗）。一覧の控えを取るときに
   * ListSnapshot.staleGroups へ渡し、戻ったときに取り直させる。
   */
  staleGroups: () => FolderRef[];
}
