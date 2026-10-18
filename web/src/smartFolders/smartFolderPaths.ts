import type { GroupRef } from "../api/groups";
import type { Crumb } from "../folders/folderPath";
import { parseGroupId } from "../groups/groupPaths";
import { t } from "../i18n";
import { LIBRARY_PATH } from "../library/libraryPath";

// Parents are persisted, so smart-folder URLs need no group-style trail query.

export const parseSmartFolderId = parseGroupId;

export function smartFolderPagePath(id: number): string {
  return `/smart-folders/${String(id)}`;
}

/** After deletion, navigate to the response's final ancestor or the library. */
export function smartFolderParentPath(trail: readonly GroupRef[]): string {
  const parent = trail.at(-1);
  return parent === undefined ? LIBRARY_PATH : smartFolderPagePath(parent.id);
}

const SMART_FOLDER_PARAM = "smartFolder";

/** Omit absent or invalid smart-folder context from requests. */
export function smartFolderContextOf(search: string): number | undefined {
  const raw = new URLSearchParams(search).get(SMART_FOLDER_PARAM);
  return parseSmartFolderId(raw ?? undefined) ?? undefined;
}

/** Pass smartFolder so Up next follows the folder's playback order. */
export function videoInSmartFolderPath(videoId: number, folderId: number): string {
  return `/videos/${String(videoId)}?${SMART_FOLDER_PARAM}=${String(folderId)}`;
}

export function smartFolderCrumbs(trail: readonly GroupRef[], name: string): Crumb[] {
  return [
    { label: t.home.title, to: LIBRARY_PATH },
    ...trail.map((folder) => ({
      label: folder.name,
      to: smartFolderPagePath(folder.id),
    })),
    { label: name },
  ];
}
