import { type FolderRef, request } from "./client";
import type { components } from "./gen/openapi";
import { folderRefKey } from "./libraryItems";
import { applyFavoritesToListSnapshot, holdListSnapshot } from "./listSnapshot";

// 型は api/openapi.yaml からの生成物を使う（specs/035-favorites/contracts/screen-api.md §1・§5）。
export type FavoritesResponse = components["schemas"]["FavoritesResponse"];

/**
 * お気に入りの付け外しの結果を、公開の切り替え（api/visibility.ts）と同じ形で画面へ
 * 知らせる（specs/035-favorites/research.md R-6）。サーバーからの `/api/events` の知らせは無い。
 *
 * - `videoIds` は付け外しが確かに反映された動画で、一覧の項目と控えの `favorite` を
 *   取り直さずに差し替える。
 * - `folders` は要求に含めたグループのフォルダで、一覧はそのグループの項目を
 *   `GET /api/folders/{rootId}/group` で取り直す（404 なら外す。017「Refresh and removal」）。
 *   グループの値はサーバーが持つので、反映の数にかかわらず取り直す。
 */
export interface FavoritesChange {
  videoIds: readonly number[];
  folders: readonly FolderRef[];
  favorite: boolean;
}

type Listener = (change: FavoritesChange) => void;

const listeners = new Set<Listener>();

/**
 * サーバーが要求より少ない本数の動画にしか反映しなかった（`appliedVideos` が異なる id の
 * 数より少ない）とき、どの動画が変わったかは応答から分からない。そのときは手元で
 * 変えた扱いにせず、これで知らせて該当の動画を取り直させる（visibility と同じ）。
 */
type StaleListener = (videoIds: readonly number[]) => Promise<void> | undefined;

const staleListeners = new Set<StaleListener>();

/** sent は要求を送る直前に払い出す通し番号で、送った順に増える。 */
let sent = 0;

/**
 * settledThrough は決着した（応答を受けたか失敗した）付け外しのうち最後の通し番号である。
 * 付け外しは 1 本ずつ順に送るので、これ以下の付け外しはすべて決着している。
 */
let settledThrough = 0;

/** applied は動画ごとに、反映済みの要求の通し番号を持つ（古い応答で巻き戻さない）。 */
const applied = new Map<number, number>();

/**
 * changes は反映した付け外しの数で、latest は動画・グループごとの直近の結果と、それが
 * 何番目の反映だったかを持つ。付け外しの前に始まった GET の応答はサーバーが変える前の
 * `favorite` を読んでいることがあるので、favoriteMark と withFavoriteSince で補正する。
 */
let changes = 0;
const latestVideos = new Map<number, { favorite: boolean; change: number }>();
const latestGroups = new Map<string, { favorite: boolean; change: number }>();

/** tail は最後に送った（または送る順番を待っている）付け外しが決着する Promise である。 */
let tail: Promise<void> | undefined;

function record(
  videoIds: readonly number[],
  uncertainIds: readonly number[],
  folders: readonly FolderRef[],
  foldersCertain: boolean,
  favorite: boolean,
  sequence: number,
): void {
  const isFresh = (videoId: number) => {
    if (sequence <= (applied.get(videoId) ?? 0)) return false;
    applied.set(videoId, sequence);
    return true;
  };
  const fresh = videoIds.filter(isFresh);
  const uncertain = uncertainIds.filter(isFresh);
  changes += 1;
  for (const videoId of fresh) latestVideos.set(videoId, { favorite, change: changes });
  for (const videoId of uncertain) latestVideos.delete(videoId);
  for (const folder of folders) {
    const key = folderRefKey(folder);
    if (foldersCertain) latestGroups.set(key, { favorite, change: changes });
    else latestGroups.delete(key);
  }
  if (fresh.length > 0 || folders.length > 0) {
    applyFavoritesToListSnapshot(fresh, folders, favorite);
    for (const listener of listeners) listener({ videoIds: fresh, folders, favorite });
  }
  if (uncertain.length > 0) {
    // 取り直しが決着するまで一覧の控えを取らせない（visibility の recordUncertain と同じ理由）。
    const release = holdListSnapshot();
    for (const listener of staleListeners) {
      const refetch = listener(uncertain);
      if (refetch === undefined) continue;
      const releaseOne = holdListSnapshot();
      void refetch.then(releaseOne, releaseOne);
    }
    release();
  }
}

/** subscribeFavorites は付け外しの結果を受け取る。戻り値で購読をやめる。 */
export function subscribeFavorites(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * subscribeFavoritesStale は、付け外しが一部の動画にしか反映されず、該当の動画を
 * 取り直すべきときに呼ばれる。返した Promise が決着するまで一覧の控えを取らない。
 */
export function subscribeFavoritesStale(listener: StaleListener): () => void {
  staleListeners.add(listener);
  return () => {
    staleListeners.delete(listener);
  };
}

/**
 * noteFavoriteOf は、サーバーから読んだ `favorite` を、要求を送らずに `videoIds` の動画へ
 * 反映した付け外しとして知らせる（一覧の項目・控え・取得中の補正）。
 *
 * お気に入りは利用者データの鍵に結ぶので、同じ集まりのバージョンは値を共有する
 * （specs/035-favorites/research.md R-1）。一方、一覧に出るのは代表だけで、代表以外の
 * バージョンを再生画面で付け外しても、知らせる id は一覧に無い。再生画面は付け外しの後に
 * 取り直した動画の `versions.representativeId` と `favorite` でこれを呼び、一覧の代表を
 * 揃える。
 *
 * `sequence` は読んだ GET を始める直前に取った favoriteReadMark である。その GET は、それまでに
 * 決着した付け外しを読んでいるが、まだ決着していない付け外しは読んでいないことがある。この番号で
 * 知らせるので、GET の後に応答が届いた付け外しの値を古い読みで覆わず、GET より前に決着した
 * 付け外しの値は読んだ値で上書きする。
 */
export function noteFavoriteOf(
  videoIds: readonly number[],
  favorite: boolean,
  sequence: number,
): void {
  if (videoIds.length === 0) return;
  record(Array.from(new Set(videoIds)), [], [], true, favorite, sequence);
}

/**
 * favoriteReadMark は GET を始める直前に呼び、その時点までに決着した付け外しの通し番号を返す。
 * 読んだ値を noteFavoriteOf で知らせるときに渡す。
 */
export function favoriteReadMark(): number {
  return settledThrough;
}

/** favoriteMark は取りに行く直前に呼び、その時点までに反映した付け外しの印を返す。 */
export function favoriteMark(): number {
  return changes;
}

/** withFavoriteSince は `mark` の後に反映した付け外しがあれば、動画の `favorite` をその結果へ差し替える。 */
export function withFavoriteSince<T extends { id: number; favorite?: boolean }>(
  video: T,
  mark: number,
): T {
  const change = latestVideos.get(video.id);
  if (change === undefined || change.change <= mark) return video;
  if (video.favorite === change.favorite) return video;
  return { ...video, favorite: change.favorite };
}

/** withGroupFavoriteSince はグループの項目の `favorite` を withFavoriteSince と同じく補正する。 */
export function withGroupFavoriteSince<
  T extends { folder: FolderRef; favorite?: boolean },
>(group: T, mark: number): T {
  const change = latestGroups.get(
    folderRefKey({ rootId: group.folder.rootId, path: group.folder.path }),
  );
  if (change === undefined || change.change <= mark) return group;
  if (group.favorite === change.favorite) return group;
  return { ...group, favorite: change.favorite };
}

/**
 * updateFavorites は `videoIds` の動画と `folders` のグループのお気に入りを `favorite` に
 * そろえる（PUT /api/favorites、specs/035-favorites/contracts/screen-api.md §1）。カード・行の
 * 1 件も、選択バーの一括も、これを使う。前の付け外しが決着するまで送るのを待ち、成功したら
 * 結果を画面へ知らせる。
 */
export function updateFavorites(
  videoIds: readonly number[],
  folders: readonly FolderRef[],
  favorite: boolean,
  signal?: AbortSignal,
): Promise<FavoritesResponse> {
  sent += 1;
  const sequence = sent;
  const send = async () => {
    try {
      return await put();
    } finally {
      settledThrough = Math.max(settledThrough, sequence);
    }
  };
  const put = async () => {
    const result = await request<FavoritesResponse>("/api/favorites", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        videoIds: Array.from(videoIds),
        folders: folders.map((folder) => ({ rootId: folder.rootId, path: folder.path })),
        favorite,
      }),
      signal,
    });
    const unique = Array.from(new Set(videoIds));
    const uniqueFolders = Array.from(
      new Map(folders.map((folder) => [folderRefKey(folder), folder])).values(),
    );
    const videosCertain = result.appliedVideos >= unique.length;
    record(
      videosCertain ? unique : [],
      videosCertain ? [] : unique,
      uniqueFolders,
      result.appliedFolders >= uniqueFolders.length,
      favorite,
      sequence,
    );
    return result;
  };
  // 待っている付け外しが無ければすぐに送る（押した直後に送信中になる）。
  const run = tail === undefined ? send() : tail.then(send);
  const settled = run.then(
    () => undefined,
    () => undefined,
  );
  tail = settled;
  void settled.then(() => {
    if (tail === settled) tail = undefined;
  });
  return run;
}
