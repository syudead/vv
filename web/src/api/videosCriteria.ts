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
 * （specs/013-library-search/contracts/list-url.md の q・watch・playable・sort・seed と、
 * specs/035-favorites の fav）。
 * 省略した項目はサーバーの既定になる。
 */
export interface VideosCriteria {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  /** true ならお気に入りの項目だけにする（specs/035-favorites/contracts/screen-api.md §2）。 */
  favorite?: boolean;
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
  /**
   * 絞り込む人物・団体の id（tag と同じく listFolderVideos には渡さない。
   * specs/047-video-work-info/contracts/screen-api.md「List parameters `person` and
   * `organization`」）。
   */
  person?: number;
  organization?: number;
  /** 絞り込むタグの種類の id（specs/051-tag-kinds/contracts/screen-api.md）。 */
  tagKind?: number;
  /**
   * 追加日と長さの絞り込み（tag と同じく listFolderVideos には渡さない。
   * specs/052-smart-folders/research.md R-4）。
   */
  addedWithin?: number;
  minDuration?: number;
  maxDuration?: number;
}

/** criteriaKey は条件を値で比べるための文字列にする。 */
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
  ]);
}

/**
 * listsSmartFolders はこの条件のライブラリの一番上にスマートフォルダのカードが並ぶかを返す。
 * 検索もお気に入りのみ以外の絞り込みも無いときだけ並ぶ（specs/052-smart-folders/research.md
 * R-7、サーバーの `domain.VideoQuery.ListsSmartFolders` と同じ判定）。並べ替えは絞り込みではない。
 */
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
    criteria.maxDuration === undefined
  );
}
