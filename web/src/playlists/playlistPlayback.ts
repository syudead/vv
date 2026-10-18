import type {
  Playlist,
  PlaylistItem,
  PlaylistPlayback,
  PlaylistStep,
} from "../api/playlists";
import { orderedItemCollection } from "../videoList/ItemRowCells";

//

export interface PlaylistQuery {
  playlistId: number;
  itemId: number;
}

function positiveId(value: string | null): number | null {
  if (value === null || !/^\d+$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

/** Require both positive playlist and item IDs; otherwise omit playlist context. */
export function parsePlaylistQuery(search: string): PlaylistQuery | null {
  const params = new URLSearchParams(search);
  const playlistId = positiveId(params.get("playlist"));
  const itemId = positiveId(params.get("item"));
  if (playlistId === null || itemId === null) return null;
  return { playlistId, itemId };
}

/** Preserve playlist while updating item; include smartFolder only for smart-folder steps. */
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

/** Read smartFolderId from the playback step itself, never the panel response. */
export function playlistStepPath(playlistId: number, step: PlaylistStep): string {
  return stepPath(playlistId, step.video.id, step.itemId, step.smartFolderId);
}

export function itemStartVideoId(item: PlaylistItem): number | undefined {
  if (item.kind === "video") return item.video?.id;
  return orderedItemCollection(item)?.openVideoId;
}

export function itemSmartFolderId(item: PlaylistItem): number | undefined {
  return item.kind === "smartFolder" ? item.smartFolder?.id : undefined;
}

export function itemStepPath(playlistId: number, item: PlaylistItem): string | null {
  const videoId = itemStartVideoId(item);
  if (videoId === undefined) return null;
  return stepPath(playlistId, videoId, item.id, itemSmartFolderId(item));
}

export interface PlaylistPair {
  playback: PlaylistPlayback;
  playlist: Playlist;
}

/** Matching updatedAt values represent one shared order. */
export function pairAgrees(pair: PlaylistPair): boolean {
  return (
    Date.parse(pair.playback.playlist.updatedAt) === Date.parse(pair.playlist.updatedAt)
  );
}

export const maxPairReads = 3;

function pairTime(pair: PlaylistPair): number {
  return Math.max(
    Date.parse(pair.playback.playlist.updatedAt),
    Date.parse(pair.playlist.updatedAt),
  );
}

/**
 * Prefer the last agreeing pair; after exhausting retries use the later updatedAt pair as a whole,
 * never mixed halves.
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
 * Keep the lower third of the preceding row visible as playback context; scroll to zero for the
 * first row.
 */
export function panelScrollTop(previous: { top: number; height: number } | null): number {
  if (previous === null) return 0;
  return Math.max(0, Math.round(previous.top + (previous.height * 2) / 3));
}
