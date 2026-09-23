import type { FolderRef } from "../api/client";

/** FOLDERS_ROOT はフォルダ画面の最上位の URL である。 */
export const FOLDERS_ROOT = "/folders";

/** FolderLocation は URL が指すフォルダ画面の場所である。 */
export type FolderLocation =
  | { kind: "root" }
  | { kind: "folder"; folder: FolderRef }
  /** 解釈できない URL（id が数でない・復号できない段・空の段）。 */
  | { kind: "invalid" };

/**
 * folderUrl はフォルダの URL を作る。段ごとに encodeURIComponent で包むので、
 * `%`・`#`・`?`・空白を含む名前でも URL の往復で同じフォルダに戻る。
 */
export function folderUrl(folder?: FolderRef): string {
  if (folder === undefined) return FOLDERS_ROOT;
  const segments = folder.path === "" ? [] : folder.path.split("/");
  return [FOLDERS_ROOT, String(folder.rootId), ...segments.map(encodeURIComponent)].join(
    "/",
  );
}

/**
 * parseFolderPathname は URL のパス部分からフォルダを読み取る。
 *
 * react-router の splat 引数には任せず、`location.pathname` を段ごとに1回だけ
 * 復号する。任せると `%25` を含む段が二重に復号され、別のフォルダを指す。
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

/** folderKey はフォルダを一意に表す文字列である（控えの鍵・React の key）。 */
export function folderKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/**
 * rootDisplayName は登録フォルダの表示名を絶対パスから作る。サーバーの
 * `FolderSummary.name` と同じ規則（最後の段、段が無ければパスそのもの）。
 */
export function rootDisplayName(rootPath: string): string {
  const segments = rootPath.split(/[/\\]/).filter((segment) => segment !== "");
  const last = segments.at(-1);
  if (last === undefined || /^[A-Za-z]:$/.test(last)) return rootPath;
  return last;
}

/** Crumb はパンくずの1段である。to が無い段は現在地。 */
export interface Crumb {
  label: string;
  to?: string;
}

/**
 * breadcrumbsFor はフォルダまでのパンくずを作る。rootName が分からない間は
 * undefined を渡し、登録フォルダの段は骨組みで描く。
 */
export function breadcrumbsFor(
  folder: FolderRef,
  rootName: string | undefined,
): (Crumb | undefined)[] {
  const crumbs: (Crumb | undefined)[] = [{ label: "フォルダ", to: FOLDERS_ROOT }];
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
