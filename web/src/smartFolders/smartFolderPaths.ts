import type { GroupRef } from "../api/groups";
import type { Crumb } from "../folders/folderPath";
import { parseGroupId } from "../groups/groupPaths";
import { t } from "../i18n";
import { LIBRARY_PATH } from "../library/libraryPath";

// スマートフォルダのページが URL と応答から決める規則（specs/052-smart-folders/ui-design.md
// 「Smart folder page」）。親は保存されているので、グループのページのような `?trail` は無い。

/** parseSmartFolderId は経路の id を正の整数として読む。読めなければ null（無いフォルダ）。 */
export const parseSmartFolderId = parseGroupId;

/** smartFolderPagePath はスマートフォルダのページの場所である。 */
export function smartFolderPagePath(id: number): string {
  return `/smart-folders/${String(id)}`;
}

/**
 * smartFolderParentPath はフォルダを消した後に移る場所である。応答の `trail` の最後（親）の
 * ページ、親が無ければライブラリ（ui-design.md「Deleting」）。
 */
export function smartFolderParentPath(trail: readonly GroupRef[]): string {
  const parent = trail.at(-1);
  return parent === undefined ? LIBRARY_PATH : smartFolderPagePath(parent.id);
}

/** SMART_FOLDER_PARAM は動画ページを開いたスマートフォルダの id を載せる問い合わせの名前である。 */
const SMART_FOLDER_PARAM = "smartFolder";

/**
 * smartFolderContextOf は動画ページの URL の問い合わせから、動画を開いたスマートフォルダの id を
 * 読む。無いか正の整数でなければ undefined（読み取りに `smartFolder` を渡さない）。
 */
export function smartFolderContextOf(search: string): number | undefined {
  const raw = new URLSearchParams(search).get(SMART_FOLDER_PARAM);
  return parseSmartFolderId(raw ?? undefined) ?? undefined;
}

/**
 * videoInSmartFolderPath はスマートフォルダ `folderId` のページから開く動画ページの場所である。
 * `?smartFolder` で再生画面の `Up next` がフォルダの再生順に従う（ui-design.md「Body」）。
 */
export function videoInSmartFolderPath(videoId: number, folderId: number): string {
  return `/videos/${String(videoId)}?${SMART_FOLDER_PARAM}=${String(folderId)}`;
}

/**
 * smartFolderCrumbs は道筋の帯の段である。「Library」、応答の `trail` の各祖先（そのページへの
 * リンク、外側から）、開いているフォルダの名前（現在地、リンクにしない）の順。
 */
export function smartFolderCrumbs(trail: readonly GroupRef[], name: string): Crumb[] {
  return [
    { label: t.library.title, to: LIBRARY_PATH },
    ...trail.map((folder) => ({
      label: folder.name,
      to: smartFolderPagePath(folder.id),
    })),
    { label: name },
  ];
}
