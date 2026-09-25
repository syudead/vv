import type { FolderRef, VideoFolder } from "../api/client";

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

/**
 * VideoLocationLabel は動画カードに添える置き場所である（ui-design.md
 * 「Search results」）。`label` は表示・読み上げに使う文字列（先頭の側を
 * 省略して表示する）、`title` は `title` 属性に入れる省略しない全体である。
 */
export interface VideoLocationLabel {
  label: string;
  title: string;
}

/**
 * folderLocationLabel は、開いているフォルダの配下検索結果に添える置き場所を作る
 * （ui-design.md「Search results」）。直下は「このフォルダ」、それ以外は開いている
 * フォルダからの相対パス。相対パス自体が省略しない全体なので、`title` は `label`
 * と同じにする。
 */
export function folderLocationLabel(
  open: FolderRef,
  target: VideoFolder,
): VideoLocationLabel {
  if (target.path === open.path) return { label: "このフォルダ", title: "このフォルダ" };
  const prefix = open.path === "" ? "" : `${open.path}/`;
  const relative = target.path.startsWith(prefix)
    ? target.path.slice(prefix.length)
    : target.path;
  return { label: relative, title: relative };
}

/** RootDisplay は最上位の置き場所を作るのに要る登録フォルダの情報である。 */
export interface RootDisplay {
  /** rootFolderName と同じ規則の表示名。 */
  name: string;
  /** 登録フォルダの絶対パス。ゲストの応答には無い（guest-api.md §1）。 */
  rootPath?: string;
}

/**
 * topLevelLocationLabel は、最上位（`/folders`）の検索結果に添える置き場所を作る。
 * 表示は登録フォルダの表示名から始め、直下ならその名前だけにする。`title` は
 * 省略しない全体として、登録フォルダの絶対パスから始める（ui-design.md
 * 「Search results」）。登録フォルダが分からない間（取り込み直後の入れ替わりなど）
 * は undefined を返し、呼び出し側は行ごと出さない。
 */
export function topLevelLocationLabel(
  target: VideoFolder,
  root: RootDisplay | undefined,
): VideoLocationLabel | undefined {
  if (root === undefined) return undefined;
  // 絶対パスが無い（ゲストの）ときは、表示名から始める。
  const base = root.rootPath ?? root.name;
  if (target.path === "") return { label: root.name, title: base };
  return {
    label: `${root.name}/${target.path}`,
    title: `${base}/${target.path}`,
  };
}

/** folderKey はフォルダを一意に表す文字列である（控えの鍵・React の key）。 */
export function folderKey(folder: FolderRef): string {
  return `${String(folder.rootId)}\0${folder.path}`;
}

/**
 * rootDisplayName は登録フォルダの表示名を絶対パスから作る。サーバーの
 * `FolderSummary.name` と同じ規則（最後の段、段が無ければパスそのもの）。
 *
 * 区切りはサーバーの OS に従う。`/` で始まる（Unix の）パスでは `\` はファイル名の
 * 一部なので区切りにしない。それ以外（Windows のドライブ付きパス）は両方を区切る。
 */
export function rootDisplayName(rootPath: string): string {
  const separator = rootPath.startsWith("/") ? /\// : /[/\\]/;
  const segments = rootPath.split(separator).filter((segment) => segment !== "");
  const last = segments.at(-1);
  if (last === undefined || /^[A-Za-z]:$/.test(last)) return rootPath;
  return last;
}

/**
 * rootFolderName は登録フォルダの集計（`path` が空の FolderSummary）から表示名を作る。
 * 絶対パスがあれば rootDisplayName で作り、ゲストの応答のように無ければサーバーが
 * 同じ規則で作った `name` を使う（specs/016-single-account-auth/contracts/guest-api.md §1）。
 */
export function rootFolderName(root: { name: string; rootPath?: string }): string {
  return root.rootPath === undefined ? root.name : rootDisplayName(root.rootPath);
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
