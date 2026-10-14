import type { FolderRef, LibraryItem, Video } from "./client";

/**
 * 一覧の項目（LibraryItem）を読み分ける小さな道具である。一覧の保持
 * （useVideos・listSnapshot）と、項目を並べる画面が共有する。
 */

/** videoItem は動画1件を動画の項目に包む（GET /api/videos・フォルダの動画の応答）。 */
export function videoItem(video: Video): LibraryItem {
  return { kind: "video", video };
}

/** folderRefKey はフォルダを値で比べるための文字列にする。 */
export function folderRefKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/**
 * itemKey は項目を一覧の中で一意に指す文字列である。動画は動画の id、グループは
 * グループの id で指す。
 */
export function itemKey(item: LibraryItem): string {
  return item.kind === "video"
    ? `video\0${String(item.video.id)}`
    : `group\0${String(item.group.id)}`;
}

/** itemVideos は項目のうち動画の項目の動画だけを、並びの順に返す。 */
export function itemVideos(items: readonly LibraryItem[]): Video[] {
  return items.flatMap((item) => (item.kind === "video" ? [item.video] : []));
}

/** shownGroupIds は項目のうちグループの項目の id を、並びの順に返す。 */
export function shownGroupIds(items: readonly LibraryItem[]): number[] {
  return items.flatMap((item) => (item.kind === "group" ? [item.group.id] : []));
}

/**
 * groupsWithMembers は、`videoIds` のどれかをメンバーに持つグループの項目の id を返す
 * （メンバーの変化でグループを取り直すため。Structural Decisions 10）。
 */
export function groupsWithMembers(
  items: readonly LibraryItem[],
  videoIds: Iterable<number>,
): number[] {
  const targets = new Set(videoIds);
  if (targets.size === 0) return [];
  return items.flatMap((item) =>
    item.kind === "group" && item.group.videoIds.some((id) => targets.has(id))
      ? [item.group.id]
      : [],
  );
}

/** groupsIn は、id が `groupIds` にあるグループの項目の id を返す（表示中のものに絞る）。 */
export function groupsIn(
  items: readonly LibraryItem[],
  groupIds: Iterable<number>,
): number[] {
  const targets = new Set(groupIds);
  if (targets.size === 0) return [];
  return items.flatMap((item) =>
    item.kind === "group" && targets.has(item.group.id) ? [item.group.id] : [],
  );
}
