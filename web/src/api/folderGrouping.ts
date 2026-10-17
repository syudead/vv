import { t, type UiText } from "../i18n";
import { type FolderRef, RequestFailed, request } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// フォルダのまとめ方の例外。
// フォルダの自動グループをタグに変えるのは groups.ts の tagGroup（POST /api/groups/{id}/tag）である。

export type FolderGrouping = components["schemas"]["FolderGrouping"];
export type FolderGroupingMode = components["schemas"]["FolderGroupingMode"];

/** groupingPath はフォルダの経路に、相対パスの問い合わせを付ける。登録フォルダ自身では省く。 */
function groupingPath(folder: FolderRef, suffix: string): string {
  const query = new URLSearchParams();
  if (folder.path !== "") query.set("path", folder.path);
  const search = query.size === 0 ? "" : `?${query.toString()}`;
  return `/api/folders/${String(folder.rootId)}/grouping${suffix}${search}`;
}

/**
 * sendGrouping はまとめ方の要求を送る。成功しても 409 でも一覧の控えを捨てる。
 *
 * 409 は、ほかのタブで先にまとめ方が変わったことを表す。そのときライブラリの控えは
 * 変わる前のカードを持っているので、成功したときと同じく次に開いたときに読み直させる
 * （plan の Structural Decisions 10）。
 */
async function sendGrouping<T>(path: string, init: RequestInit): Promise<T> {
  try {
    const result = await request<T>(path, init);
    clearListSnapshot();
    return result;
  } catch (failure) {
    if (failure instanceof RequestFailed && failure.status === 409) clearListSnapshot();
    throw failure;
  }
}

/**
 * setFolderGrouping はフォルダのまとめ方の例外を付ける・外す（`auto` は外す）。
 *
 * 成功したら（409 でも）一覧の控えを捨てる。ライブラリのカードは例外で変わるので、次にライブラリを
 * 開いたときに読み直させる（plan の Structural Decisions 10、受け入れ条件 3・4・5）。
 */
export async function setFolderGrouping(
  folder: FolderRef,
  mode: FolderGroupingMode,
  signal?: AbortSignal,
): Promise<FolderGrouping> {
  return sendGrouping<FolderGrouping>(groupingPath(folder, ""), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode }),
    signal,
  });
}

/**
 * groupingFailure は、まとめ方の操作の失敗を利用者に伝える文言と、画面が取り直すべきかを返す。
 * 409 はほかのタブで先に変わったことを表すので、画面の状態を取り直す。
 * `notFoundMessage` は 404 のときの文言である。
 */
export function groupingFailure(
  failure: unknown,
  {
    name,
    tagging,
    notFoundMessage,
  }: {
    /** フォルダ（グループ）の名前。 */
    name: string;
    /** グループをタグに変える操作の失敗か。400 の文言はこの操作にだけ当てる。 */
    tagging: boolean;
    notFoundMessage: UiText;
  },
): { message: UiText; conflict: boolean } {
  if (failure instanceof RequestFailed) {
    if (tagging && failure.status === 400 && failure.code === "invalid_request") {
      return { message: t.folderGrouping.cannotTag(name), conflict: false };
    }
    if (failure.status === 409) {
      return { message: t.folderGrouping.notGroup, conflict: true };
    }
    if (failure.status === 404) return { message: notFoundMessage, conflict: false };
  }
  return { message: t.folderGrouping.changeFailed, conflict: false };
}
