import type { FolderRef, FolderScope, VideoSort, WatchFilter } from "./client";

/** Select individual-video, mixed-library, or folder-video endpoints while sharing pagination. */
export type VideosSource = "videos" | "library" | FolderRef;

export interface VideosCriteria {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  favorite?: boolean;
  sort: VideoSort;
  /** Sent only for random sorting. */
  seed?: number;
  /** Folder-only scope; omitted equals direct. */
  scope?: FolderScope;
  /** Do not pass tag filters to folder listings. */
  tag?: number[];
  /** Do not pass credit filters to folder listings. */
  person?: number;
  organization?: number;
  tagKind?: number;
  /** Do not pass added/length filters to folder listings. */
  addedWithin?: number;
  minDuration?: number;
  maxDuration?: number;
  /**
   * true ならホームのどの棚にも一致しない動画だけにする（listLibrary だけが受け取る。
   * docs/design-docs/home.md）。
   */
  unshelved?: boolean;
}

export function criteriaKey(criteria: VideosCriteria): string {
  return JSON.stringify([
    criteria.query ?? "",
    criteria.watch ?? "all",
    criteria.playable === true,
    criteria.favorite === true,
    criteria.sort,
    criteria.sort === "random" ? (criteria.seed ?? null) : null,
    criteria.scope ?? "direct",
    criteria.tag ?? [],
    criteria.person ?? null,
    criteria.organization ?? null,
    criteria.tagKind ?? null,
    criteria.addedWithin ?? null,
    criteria.minDuration ?? null,
    criteria.maxDuration ?? null,
    criteria.unshelved === true,
  ]);
}

/** Smart folders appear only without search or filters other than favorites; sorting is not filtering. */
export function listsSmartFolders(criteria: VideosCriteria): boolean {
  return (
    (criteria.query ?? "").trim() === "" &&
    (criteria.watch ?? "all") === "all" &&
    criteria.playable !== true &&
    (criteria.tag ?? []).length === 0 &&
    criteria.person === undefined &&
    criteria.organization === undefined &&
    criteria.tagKind === undefined &&
    criteria.addedWithin === undefined &&
    criteria.minDuration === undefined &&
    criteria.maxDuration === undefined &&
    criteria.unshelved !== true
  );
}
