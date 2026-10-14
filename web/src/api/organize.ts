import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// ドラッグの操作と取り消しの画面 API（specs/050-drag-organize/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。どれも所有者だけである。操作は 1 つの取引で行い、取り消しの
// トークン（`undoToken`）を返す。トークンは 10 分有効で、1 回だけ使える。

export type OrganizeResult = components["schemas"]["OrganizeResult"];

const jsonHeaders = { "Content-Type": "application/json" };

/** postOperation は操作 1 つを送ってその結果を返し、成功したら一覧の控えを捨てる。 */
async function postOperation(
  path: string,
  body: unknown,
  signal: AbortSignal | undefined,
): Promise<OrganizeResult> {
  const result = await request<OrganizeResult>(path, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
  clearListSnapshot();
  return result;
}

/**
 * reorderGroup はグループ `groupId` の項目 `itemIds` を、画面の順のまままとめて項目 `after` の
 * すぐ後ろへ移す。`null` なら先頭へ移す（POST /api/organize/reorder）。グループ・項目・`after` の
 * どれかが無いかそのグループに無ければ 404 `group_target_not_found` の RequestFailed になり、
 * 何も変わらない。成功したら一覧の控えを捨てる（ライブラリのカードのプレビューと最初に開く動画が
 * 並びで決まる）。
 */
export async function reorderGroup(
  groupId: number,
  itemIds: readonly number[],
  after: number | null,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = {
    groupId,
    itemIds: Array.from(itemIds),
    ...(after === null ? {} : { after }),
  };
  return postOperation("/api/organize/reorder", body, signal);
}

/**
 * groupVideos は動画 `videoIds` を動画 `target` に落として「グループにする」（POST
 * /api/organize/group）。新しいグループは `target` の題名を先頭 100 文字に切った名前で、`target`、
 * 続いて `videoIds` を画面の順に持つ。`from` を渡すと、新しいグループが開いているグループ `from` の
 * 中で `target` の位置を取り、`target` と `videoIds` は `from` から抜ける。応答の `groupId` が新しい
 * グループである。動画か `from` が無いか、動画が `from` に無ければ 404 `group_target_not_found` の
 * RequestFailed になり、何も変わらない。成功したら一覧の控えを捨てる。
 */
export async function groupVideos(
  videoIds: readonly number[],
  target: number,
  from: number | null,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = {
    items: videoIds.map((id) => ({ kind: "video" as const, id })),
    target,
    ...(from === null ? {} : { from }),
  };
  return postOperation("/api/organize/group", body, signal);
}

/**
 * undoOrganize はトークン `token` の操作を取り消す（POST /api/organize/undo）。操作の後で行が
 * 変わっていれば 409 `undo_conflict`、トークンが無い・使い済み・10 分より古ければ 410
 * `undo_expired` の RequestFailed になり、どちらも何も変わらない。成功したら一覧の控えを捨てる。
 */
export async function undoOrganize(token: string, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch("/api/organize/undo", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ token }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  clearListSnapshot();
}
