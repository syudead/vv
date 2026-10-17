import type { VideoFolder } from "../api/client";
import type { GroupRef } from "../api/groups";
import type { Crumb } from "../folders/folderPath";
import { t } from "../i18n";
import { LIBRARY_PATH } from "../library/libraryPath";

// グループのページが URL と応答から決める規則。
// 画面の部品から外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** TRAIL_PARAM は通ってきたグループの id を載せる問い合わせの名前である。 */
export const TRAIL_PARAM = "trail";

/** maxTrailLength は `trail` に載せる id の上限である（api/openapi.yaml の maxItems）。 */
const maxTrailLength = 100;

/** parseGroupId は経路の id を正の整数として読む。読めなければ null（無いグループとして扱う）。 */
export function parseGroupId(raw: string | undefined): number | null {
  if (raw === undefined || !/^[1-9]\d*$/.test(raw)) return null;
  const id = Number(raw);
  return Number.isSafeInteger(id) ? id : null;
}

/**
 * parseTrail は URL の問い合わせから通ってきたグループの id を外側から順に読む。読めない段は
 * 捨てる。どの段が今も親と子かはサーバーが答えるので、ここでは形だけを見る。
 */
export function parseTrail(search: string): number[] {
  const raw = new URLSearchParams(search).get(TRAIL_PARAM);
  if (raw === null || raw === "") return [];
  return raw
    .split(",")
    .map(parseGroupId)
    .filter((id): id is number => id !== null)
    .slice(0, maxTrailLength);
}

/** groupPagePath はグループのページの場所である。`trail` が空なら問い合わせを付けない。 */
export function groupPagePath(id: number, trail: readonly number[] = []): string {
  const base = `/groups/${String(id)}`;
  if (trail.length === 0) return base;
  return `${base}?${TRAIL_PARAM}=${trail.slice(-maxTrailLength).map(String).join(",")}`;
}

/**
 * childGroupPath は、グループ `id` のページ（道筋 `trail`）から子グループ `childId` を開く
 * 場所である。道筋を 1 段伸ばす。
 */
export function childGroupPath(
  childId: number,
  trail: readonly GroupRef[],
  id: number,
): string {
  return groupPagePath(childId, [...trail.map((group) => group.id), id]);
}

/**
 * groupItemPath はグループ `id` のページ（道筋 `trail`）で項目が開く場所である。動画は
 * `?group` 付きの再生画面、子グループは道筋を 1 段伸ばしたそのページ。
 */
export function groupItemPath(
  item: { kind: "video" | "group"; video?: { id: number }; group?: { id: number } },
  trail: readonly GroupRef[],
  id: number,
): string {
  if (item.kind === "group" && item.group !== undefined) {
    return childGroupPath(item.group.id, trail, id);
  }
  return videoInGroupPath(item.video?.id ?? 0, id);
}

/** GROUP_PARAM は動画ページを開いたグループの id を載せる問い合わせの名前である。 */
const GROUP_PARAM = "group";

/**
 * groupContextOf は動画ページの URL の問い合わせから、動画を開いたグループの id を読む。
 * 無いか正の整数でなければ undefined（読み取りに `group` を渡さない）。
 */
export function groupContextOf(search: string): number | undefined {
  const raw = new URLSearchParams(search).get(GROUP_PARAM);
  return parseGroupId(raw ?? undefined) ?? undefined;
}

/**
 * videoInGroupPath はグループ `groupId` のページから開く動画ページの場所である。`?group` で
 * 再生画面の `Up next` がそのグループの順に従う。
 */
export function videoInGroupPath(videoId: number, groupId: number): string {
  return `/videos/${String(videoId)}?${GROUP_PARAM}=${String(groupId)}`;
}

/**
 * searchWithoutGroup は動画ページの問い合わせから `?group` を外したものである。ほかの問い合わせは
 * 残し、何も残らなければ空文字を返す（グループの操作の後に `group` なしで読み直す。 「Video page」）。
 */
export function searchWithoutGroup(search: string): string {
  const params = new URLSearchParams(search);
  params.delete(GROUP_PARAM);
  return params.size === 0 ? "" : `?${params.toString()}`;
}

/**
 * parentPagePath はグループが終わった（解散、タグ化）後に移る場所である。道筋の最後の
 * グループ（その手前で切った道筋付き）、道筋が無ければライブラリ。
 */
export function parentPagePath(trail: readonly GroupRef[]): string {
  const parent = trail[trail.length - 1];
  if (parent === undefined) return LIBRARY_PATH;
  return groupPagePath(
    parent.id,
    trail.slice(0, -1).map((each) => each.id),
  );
}

/**
 * trailCrumbs は道筋の帯の段である。「Library」、応答の `trail` の各グループ（その手前で
 * 切った道筋付きのリンク）、開いているグループの名前（現在地、リンクにしない）の順。
 */
export function trailCrumbs(trail: readonly GroupRef[], name: string): Crumb[] {
  return [
    { label: t.library.title, to: LIBRARY_PATH },
    ...trail.map((group, index) => ({
      label: group.name,
      to: groupPagePath(
        group.id,
        trail.slice(0, index).map((each) => each.id),
      ),
    })),
    { label: name },
  ];
}

/**
 * loadingCrumbs は読み込み中の帯の段である。「Library」のあとに、URL の `trail` の段と
 * 開いているグループの段を名前の分からない段（undefined、骨組みで描く）として並べる。
 */
export function loadingCrumbs(trail: readonly number[]): (Crumb | undefined)[] {
  return [
    { label: t.library.title, to: LIBRARY_PATH },
    ...trail.map(() => undefined),
    undefined,
  ];
}

/**
 * folderDisplayPath は自動グループのフォルダの、フォルダ画面のパンくずと同じ見せ方の
 * パスである。登録フォルダの名前 `rootName` が分からない間は相対パスだけ（登録フォルダ
 * そのものなら空）を返す。
 */
export function folderDisplayPath(folder: VideoFolder, rootName?: string): string {
  if (rootName === undefined) return folder.path;
  return folder.path === "" ? rootName : `${rootName}/${folder.path}`;
}
