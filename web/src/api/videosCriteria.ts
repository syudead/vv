import type { FolderRef, FolderScope, VideoSort } from "./client";
import type { components } from "./gen/openapi";

type WorkPersonRole = components["schemas"]["WorkPersonRole"];

/** Select individual-video, mixed-library, or folder-video endpoints while sharing pagination. */
export type VideosSource = "videos" | "library" | FolderRef;

export interface VideosCriteria {
  query?: string;
  favorite?: boolean;
  sort: VideoSort;
  /** Sent only for random sorting. */
  seed?: number;
  /** Folder-only scope; omitted equals direct. */
  scope?: FolderScope;
  tag?: number[];
  person?: number;
  /** Requires person. */
  personRole?: WorkPersonRole;
  organization?: number;
  tagKind?: number;
}

export function criteriaKey(criteria: VideosCriteria): string {
  return JSON.stringify([
    criteria.query ?? "",
    criteria.favorite === true,
    criteria.sort,
    criteria.sort === "random" ? (criteria.seed ?? null) : null,
    criteria.scope ?? "direct",
    criteria.tag ?? [],
    criteria.person ?? null,
    criteria.personRole ?? null,
    criteria.organization ?? null,
    criteria.tagKind ?? null,
  ]);
}

/** Smart folders appear only without search or filters other than favorites; sorting is not filtering. */
export function listsSmartFolders(criteria: VideosCriteria): boolean {
  return (
    (criteria.query ?? "").trim() === "" &&
    (criteria.tag ?? []).length === 0 &&
    criteria.person === undefined &&
    criteria.organization === undefined &&
    criteria.tagKind === undefined
  );
}
