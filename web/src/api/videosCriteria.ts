import type { FolderRef, FolderScope, VideoSort, WatchFilter } from "./client";

/**
 * VideosSource は useVideos が読む一覧である。
 *
 * - `"videos"`: 1本ずつの一覧（`GET /api/videos`）。フォルダ画面の最上位の検索結果が使う。
 * - `"library"`: 動画とグループの項目の一覧（`GET /api/library`）。ライブラリが使う
 *   （specs/017-folder-groups/contracts/library-api.md §1）。
 * - フォルダ: そのフォルダの動画（`GET /api/folders/{rootId}/videos`）。フォルダ画面が使う。
 */
export type VideosSource = "videos" | "library" | FolderRef;

/**
 * VideosCriteria は一覧を取りに行く条件である
 * （specs/013-library-search/contracts/list-url.md の q・watch・playable・sort・seed）。
 * 省略した項目はサーバーの既定になる。
 */
export interface VideosCriteria {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  sort: VideoSort;
  /** sort=random のときだけ送る。 */
  seed?: number;
  /**
   * フォルダ画面での検索範囲（direct = 直下だけ、subtree = 配下すべて）。
   * folder を渡さない（ライブラリ）ときは無視する。省略時は direct と同じ。
   */
  scope?: FolderScope;
  /**
   * 絞り込むタグの id（listVideos だけが受け取る。listFolderVideos には渡さない。
   * specs/014-video-tags/contracts/tags-api.md §5）。
   */
  tag?: number[];
}

/** criteriaKey は条件を値で比べるための文字列にする。 */
export function criteriaKey(criteria: VideosCriteria): string {
  return JSON.stringify([
    criteria.query ?? "",
    criteria.watch ?? "all",
    criteria.playable === true,
    criteria.sort,
    criteria.sort === "random" ? (criteria.seed ?? null) : null,
    criteria.scope ?? "direct",
    criteria.tag ?? [],
  ]);
}
