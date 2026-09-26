import { type FolderRef, RequestFailed, request } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { refreshTags } from "./tags";

// フォルダのまとめ方の例外と、グループをタグに変える操作
// （specs/017-folder-groups/contracts/folder-groups-api.md §1・§2）。

export type FolderGrouping = components["schemas"]["FolderGrouping"];
export type FolderGroupingMode = components["schemas"]["FolderGroupingMode"];
export type FolderGroupTagResult = components["schemas"]["FolderGroupTagResult"];

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
 * tagFolderGroup はグループをタグに変える（フォルダ名のタグを使うか作り、まとめを解除する）。
 *
 * 成功したら（409 でも）一覧の控えを捨て、成功したときは共有のタグの一覧も取り直す（タグが増えるか、本数が変わる）。
 */
export async function tagFolderGroup(
  folder: FolderRef,
  signal?: AbortSignal,
): Promise<FolderGroupTagResult> {
  const result = await sendGrouping<FolderGroupTagResult>(groupingPath(folder, "/tag"), {
    method: "POST",
    signal,
  });
  refreshTags().catch(() => undefined);
  return result;
}

/** ungroupedMessage はまとめを解除したことを伝える文言である（ui-design.md「Group line」）。 */
export function ungroupedMessage(name: string): string {
  return `「${name}」のまとめを解除しました`;
}

/** taggedMessage はグループをタグに変えたことを伝える文言である（ui-design.md「Group line」）。 */
export function taggedMessage(result: FolderGroupTagResult): string {
  return result.created
    ? `タグ「${result.tag.name}」を作り、まとめを解除しました`
    : `タグ「${result.tag.name}」を付け、まとめを解除しました`;
}

/**
 * groupingFailure は、まとめ方の操作の失敗を利用者に伝える文言と、画面が取り直すべきかを返す。
 * 409 はほかのタブで先に変わったことを表すので、画面の状態を取り直す。
 * `notFoundMessage` は 404 のときの文言で、画面ごとに違う（ui-design.md「Folder grouping menu」
 * は「このフォルダは見つかりません」、「Group line」は「変更できませんでした」）。
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
    notFoundMessage: string;
  },
): { message: string; conflict: boolean } {
  if (failure instanceof RequestFailed) {
    if (tagging && failure.status === 400 && failure.code === "invalid_request") {
      return {
        message: `「${name}」はタグの名前に使えないため、タグに変えられません`,
        conflict: false,
      };
    }
    if (failure.status === 409) {
      return { message: "このフォルダはもうグループではありません", conflict: true };
    }
    if (failure.status === 404) return { message: notFoundMessage, conflict: false };
  }
  return { message: "変更できませんでした", conflict: false };
}
