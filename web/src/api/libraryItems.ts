import type { FolderRef, LibraryGroup, LibraryItem, Video } from "./client";

/**
 * 一覧の項目（LibraryItem）を読み分ける小さな道具である。一覧の保持
 * （useVideos・listSnapshot）と、項目を並べる画面が共有する
 * （specs/017-folder-groups/plan.md の Structural Decisions 13）。
 */

/** videoItem は動画1件を動画の項目に包む（GET /api/videos・フォルダの動画の応答）。 */
export function videoItem(video: Video): LibraryItem {
  return { kind: "video", video };
}

/** groupRef はグループの項目が指すフォルダである（取り直しの `GET /api/folders/{rootId}/group`）。 */
export function groupRef(group: LibraryGroup): FolderRef {
  return { rootId: group.folder.rootId, path: group.folder.path };
}

/** folderRefKey はフォルダを値で比べるための文字列にする。 */
export function folderRefKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/**
 * itemKey は項目を一覧の中で一意に指す文字列である。動画は動画の id、グループは
 * フォルダで指す（グループの id は API に出ない。LibraryGroup の説明）。
 */
export function itemKey(item: LibraryItem): string {
  return item.kind === "video"
    ? `video\0${String(item.video.id)}`
    : `group\0${folderRefKey(groupRef(item.group))}`;
}

/** itemVideos は項目のうち動画の項目の動画だけを、並びの順に返す。 */
export function itemVideos(items: readonly LibraryItem[]): Video[] {
  return items.flatMap((item) => (item.kind === "video" ? [item.video] : []));
}

/**
 * groupsWithMembers は、`videoIds` のどれかをメンバーに持つグループの項目のフォルダを返す
 * （メンバーの変化でグループを取り直すため。Structural Decisions 10）。
 */
export function groupsWithMembers(
  items: readonly LibraryItem[],
  videoIds: Iterable<number>,
): FolderRef[] {
  const targets = new Set(videoIds);
  if (targets.size === 0) return [];
  return items.flatMap((item) =>
    item.kind === "group" && item.group.videoIds.some((id) => targets.has(id))
      ? [groupRef(item.group)]
      : [],
  );
}
