import type { FolderRef, VideoFolder } from "../api/client";
import { t } from "../i18n";

export const FOLDERS_ROOT = "/folders";

export type FolderLocation =
  { kind: "root" } | { kind: "folder"; folder: FolderRef } | { kind: "invalid" };

/** Encode each segment to preserve names containing %, #, ?, or spaces through URL round trips. */
export function folderUrl(folder?: FolderRef): string {
  if (folder === undefined) return FOLDERS_ROOT;
  const segments = folder.path === "" ? [] : folder.path.split("/");
  return [FOLDERS_ROOT, String(folder.rootId), ...segments.map(encodeURIComponent)].join(
    "/",
  );
}

/**
 * Decode location.pathname once per segment; React Router splats can decode %25 twice and select a
 * different folder.
 */
export function parseFolderPathname(pathname: string): FolderLocation {
  const trimmed = pathname.replace(/\/+$/, "");
  if (trimmed === FOLDERS_ROOT) return { kind: "root" };
  if (!trimmed.startsWith(`${FOLDERS_ROOT}/`)) return { kind: "invalid" };

  const [rootSegment, ...rest] = trimmed.slice(FOLDERS_ROOT.length + 1).split("/");
  if (rootSegment === undefined || !/^[1-9][0-9]*$/.test(rootSegment)) {
    return { kind: "invalid" };
  }

  const segments: string[] = [];
  for (const raw of rest) {
    let segment: string;
    try {
      segment = decodeURIComponent(raw);
    } catch {
      return { kind: "invalid" };
    }
    if (segment === "" || segment === "." || segment === ".." || segment.includes("/")) {
      return { kind: "invalid" };
    }
    segments.push(segment);
  }
  return {
    kind: "folder",
    folder: { rootId: Number(rootSegment), path: segments.join("/") },
  };
}

/** label may truncate at the start; title retains the full location. */
export interface VideoLocationLabel {
  label: string;
  title: string;
}

/** Use a path relative to the current folder; label and title are identical. */
export function folderLocationLabel(
  open: FolderRef,
  target: VideoFolder,
): VideoLocationLabel {
  if (target.path === open.path)
    return { label: t.folders.thisFolder, title: t.folders.thisFolder };
  const prefix = open.path === "" ? "" : `${open.path}/`;
  const relative = target.path.startsWith(prefix)
    ? target.path.slice(prefix.length)
    : target.path;
  return { label: relative, title: relative };
}

export interface RootDisplay {
  name: string;
  /** Absolute root path; absent for guests. */
  rootPath?: string;
}

/**
 * Labels start with the root display name, titles with its absolute path. Return undefined until
 * the root is known so callers omit the row.
 */
export function topLevelLocationLabel(
  target: VideoFolder,
  root: RootDisplay | undefined,
): VideoLocationLabel | undefined {
  if (root === undefined) return undefined;
  // Guests fall back to the display name.
  const base = root.rootPath ?? root.name;
  if (target.path === "") return { label: root.name, title: base };
  return {
    label: `${root.name}/${target.path}`,
    title: `${base}/${target.path}`,
  };
}

export function folderKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/**
 * Follow server path semantics: backslashes are filename characters in Unix paths, separators in
 * Windows paths.
 */
export function rootDisplayName(rootPath: string): string {
  const separator = rootPath.startsWith("/") ? /\// : /[/\\]/;
  const segments = rootPath.split(separator).filter((segment) => segment !== "");
  const last = segments.at(-1);
  if (last === undefined || /^[A-Za-z]:$/.test(last)) return rootPath;
  return last;
}

/** Without an absolute path, use the server's identically derived name. */
export function rootFolderName(root: { name: string; rootPath?: string }): string {
  return root.rootPath === undefined ? root.name : rootDisplayName(root.rootPath);
}

/** Root names by ID, named as breadcrumbs name them. */
export function rootDisplays(
  roots: readonly { rootId: number; name: string; rootPath?: string }[],
): Map<number, RootDisplay> {
  return new Map(
    roots.map((root) => [
      root.rootId,
      { name: rootFolderName(root), rootPath: root.rootPath },
    ]),
  );
}

/** A crumb without to is the current location. */
export interface Crumb {
  label: string;
  to?: string;
}

/** An unknown rootName produces a skeleton root crumb. */
export function breadcrumbsFor(
  folder: FolderRef,
  rootName: string | undefined,
): (Crumb | undefined)[] {
  const crumbs: (Crumb | undefined)[] = [{ label: t.folders.title, to: FOLDERS_ROOT }];
  const segments = folder.path === "" ? [] : folder.path.split("/");
  crumbs.push(
    rootName === undefined
      ? undefined
      : { label: rootName, to: folderUrl({ rootId: folder.rootId, path: "" }) },
  );
  segments.forEach((segment, index) => {
    crumbs.push({
      label: segment,
      to: folderUrl({
        rootId: folder.rootId,
        path: segments.slice(0, index + 1).join("/"),
      }),
    });
  });
  const last = crumbs.at(-1);
  if (last !== undefined) crumbs[crumbs.length - 1] = { label: last.label };
  return crumbs;
}
