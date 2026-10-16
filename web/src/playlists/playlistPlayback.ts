import type {
  Playlist,
  PlaylistItem,
  PlaylistPlayback,
  PlaylistStep,
} from "../api/playlists";
import { orderedItemCollection } from "../videoList/ItemRowCells";

// 動画ページのプレイリストの文脈が URL と応答から決める規則
// （specs/044-playlists/ui-design.md「Video page in a playlist」「Playlist panel」「One order」）。
// 画面の部品から外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** PlaylistQuery は動画ページの URL の `?playlist=<id>&item=<itemId>` である。 */
export interface PlaylistQuery {
  playlistId: number;
  itemId: number;
}

function positiveId(value: string | null): number | null {
  if (value === null || !/^\d+$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

/**
 * parsePlaylistQuery は URL のクエリからプレイリストの文脈を読む。`playlist` と `item` の
 * どちらかが無いか正の整数でなければ null で、ページはクエリの無いときと同じになる。
 */
export function parsePlaylistQuery(search: string): PlaylistQuery | null {
  const params = new URLSearchParams(search);
  const playlistId = positiveId(params.get("playlist"));
  const itemId = positiveId(params.get("item"));
  if (playlistId === null || itemId === null) return null;
  return { playlistId, itemId };
}

/**
 * stepPath はプレイリストの項目 `itemId` を動画 `videoId` から再生する動画ページの場所である。
 * ステップへ移るときは常に `playlist` を保ち、`item` をステップの項目にする（R-7）。項目が
 * スマートフォルダの項目なら `smartFolderId` にそのフォルダを渡し、`smartFolder` を付ける。
 * 省けば付けない（specs/052-smart-folders/ui-design.md「Video page in a playlist」）。
 */
export function stepPath(
  playlistId: number,
  videoId: number,
  itemId: number,
  smartFolderId?: number,
): string {
  const params = new URLSearchParams({
    playlist: String(playlistId),
    item: String(itemId),
  });
  if (smartFolderId !== undefined) params.set("smartFolder", String(smartFolderId));
  return `/videos/${String(videoId)}?${params.toString()}`;
}

/**
 * playlistStepPath は再生の応答のステップ（`next`・`previousItem`・`nextItem`）を開く場所である。
 * フォルダはステップ自身の `smartFolderId` から取り、パネルの応答には頼らない。
 */
export function playlistStepPath(playlistId: number, step: PlaylistStep): string {
  return stepPath(playlistId, step.video.id, step.itemId, step.smartFolderId);
}

/**
 * itemStartVideoId は項目を始める動画である。動画はその動画、グループとスマートフォルダは
 * `openVideoId`（動画の無いフォルダでは undefined）。
 */
export function itemStartVideoId(item: PlaylistItem): number | undefined {
  if (item.kind === "video") return item.video?.id;
  return orderedItemCollection(item)?.openVideoId;
}

/** itemSmartFolderId はスマートフォルダの項目のフォルダの id である。ほかの項目では undefined。 */
export function itemSmartFolderId(item: PlaylistItem): number | undefined {
  return item.kind === "smartFolder" ? item.smartFolder?.id : undefined;
}

/**
 * itemStepPath は項目 `item` を始める動画ページの場所である。始める動画が無ければ null。
 * スマートフォルダの項目は `smartFolder` を付ける（ui-design.md「Playlist screen row」）。
 */
export function itemStepPath(playlistId: number, item: PlaylistItem): string | null {
  const videoId = itemStartVideoId(item);
  if (videoId === undefined) return null;
  return stepPath(playlistId, videoId, item.id, itemSmartFolderId(item));
}

/** PlaylistPair は同時に読んだ再生の応答とプレイリストの応答の 1 組である。 */
export interface PlaylistPair {
  playback: PlaylistPlayback;
  playlist: Playlist;
}

/** pairAgrees は 2 つの応答が同じ並びを表すか（`updatedAt` が同じか）である（One order）。 */
export function pairAgrees(pair: PlaylistPair): boolean {
  return (
    Date.parse(pair.playback.playlist.updatedAt) === Date.parse(pair.playlist.updatedAt)
  );
}

/** maxPairReads は、並びがそろわないときに組を読む回数の上限である（One order）。 */
export const maxPairReads = 3;

function pairTime(pair: PlaylistPair): number {
  return Math.max(
    Date.parse(pair.playback.playlist.updatedAt),
    Date.parse(pair.playlist.updatedAt),
  );
}

/**
 * choosePair は読んだ組から使う 1 組を選ぶ。そろった組があれば最後のそろった組、
 * 上限まで読んでもそろわなければ `updatedAt` の遅い組で、両方の部分をそれが代表する
 * （ui-design.md「One order」）。組が無ければ null。
 */
export function choosePair(pairs: readonly PlaylistPair[]): PlaylistPair | null {
  let agreed: PlaylistPair | null = null;
  for (const pair of pairs) if (pairAgrees(pair)) agreed = pair;
  if (agreed !== null) return agreed;
  let latest: PlaylistPair | null = null;
  for (const pair of pairs) {
    if (latest === null || pairTime(pair) > pairTime(latest)) latest = pair;
  }
  return latest;
}

/**
 * panelScrollTop は、パネルの一覧の `scrollTop` である。再生中の行を見える位置に置き、
 * 上の行の下端（高さの 3 分の 1）を残して、何が前に流れたかを読めるようにする
 * （ui-design.md「Playlist panel」Scrolling）。位置は一覧の上端からの値で、上の行が無ければ 0。
 */
export function panelScrollTop(previous: { top: number; height: number } | null): number {
  if (previous === null) return 0;
  return Math.max(0, Math.round(previous.top + (previous.height * 2) / 3));
}
