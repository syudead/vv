import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// ドラッグの操作と取り消しの画面 API（specs/050-drag-organize/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。どれも所有者だけである。操作は 1 つの取引で行い、取り消しの
// トークン（`undoToken`）を返す。トークンは 10 分有効で、1 回だけ使える。

export type OrganizeResult = components["schemas"]["OrganizeResult"];
export type OrganizeRef = components["schemas"]["OrganizeRef"];

const jsonHeaders = { "Content-Type": "application/json" };

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
  const result = await request<OrganizeResult>("/api/organize/reorder", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
  clearListSnapshot();
  return result;
}

/**
 * mergeGroups はドラッグしたカード `items`（画面の順。少なくとも 1 つのグループ）をグループ `into` へ
 * マージする（POST /api/organize/merge）。グループはその項目を `into` の末尾へ渡してお気に入り・
 * プレイリストの項目・親を `into` に引き継いで消え、動画は `into` の末尾へ足される。1 つの取引で、
 * トークンも 1 つである。ドラッグしたグループが `into` か `into` の祖先なら 409 `group_cycle`、
 * どれかが無ければ 404 `group_target_not_found` の RequestFailed になり、何も変わらない。成功したら
 * 一覧の控えを捨てる。
 */
export async function mergeGroups(
  items: readonly OrganizeRef[],
  into: number,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const result = await request<OrganizeResult>("/api/organize/merge", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ items: Array.from(items), into }),
    signal,
  });
  clearListSnapshot();
  return result;
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
