import type { FolderRef, LibraryItem, Video } from "./client";

export function videoItem(video: Video): LibraryItem {
  return { kind: "video", video };
}

export function folderRefKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/** Smart folders are neither drag sources nor drop targets. */
export type OrganizableItem = Exclude<LibraryItem, { kind: "smartFolder" }>;

export function isOrganizable(item: LibraryItem): item is OrganizableItem {
  return item.kind !== "smartFolder";
}

export function itemKey(item: LibraryItem): string {
  if (item.kind === "smartFolder") return smartFolderKey(item.smartFolder.id);
  return item.kind === "video" ? videoKey(item.video.id) : groupKey(item.group.id);
}

export function smartFolderKey(id: number): string {
  return `smartFolder\0${String(id)}`;
}

export function isSmartFolderKey(key: string): boolean {
  return key.startsWith("smartFolder\0");
}

export function videoKey(id: number): string {
  return `video\0${String(id)}`;
}

export function groupKey(id: number): string {
  return `group\0${String(id)}`;
}

export function itemVideo(item: LibraryItem): Video | undefined {
  return item.kind === "video" ? item.video : undefined;
}

export function itemVideos(items: readonly LibraryItem[]): Video[] {
  return items.flatMap((item) => (item.kind === "video" ? [item.video] : []));
}

export function shownGroupIds(items: readonly LibraryItem[]): number[] {
  return items.flatMap((item) => (item.kind === "group" ? [item.group.id] : []));
}

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
