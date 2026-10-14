import { t, type UiText } from "../i18n";
import { apiFetch, type LibraryGroup, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { refreshTags } from "./tags";

// グループの一覧と編集の画面 API（specs/048-groups/contracts/screen-api.md「Statuses」）。型は
// api/openapi.yaml からの生成物を使う。一覧はゲストも読め、書き込みは所有者だけである。
// グループ 1 つの読み出し（getGroup）は client.ts にある。

export type GroupListPage = components["schemas"]["GroupListPage"];
export type GroupTagResult = components["schemas"]["GroupTagResult"];

const jsonHeaders = { "Content-Type": "application/json" };

function groupPath(id: number): string {
  return `/api/groups/${String(id)}`;
}

/** 作成と追加で項目にする動画とグループ。動画、次にグループの順に、それぞれこの順で足す。 */
export interface GroupMembers {
  videoIds?: readonly number[];
  groupIds?: readonly number[];
}

function membersBody(members: GroupMembers): { videoIds: number[]; groupIds: number[] } {
  return {
    videoIds: Array.from(members.videoIds ?? []),
    groupIds: Array.from(members.groupIds ?? []),
  };
}

export interface ListGroupsParams {
  /** 名前に含む文字列。サーバーが前後の空白を取り除き、大文字小文字を区別せずに比べる。 */
  q?: string;
  /** 前回の応答の `nextCursor`。中身は解釈しない。 */
  cursor?: string;
  /** 1 ページの件数。省けばサーバーの既定（100 件）。 */
  limit?: number;
  signal?: AbortSignal;
}

/**
 * listGroups は見る人に見せるグループを、どの深さのものも自然な名前の順に 1 ページ取得する
 * （GET /api/groups）。「Add to group」のピッカーが読む。読めないカーソルは 400 `invalid_cursor`
 * の RequestFailed になる。
 */
export function listGroups({
  q,
  cursor,
  limit,
  signal,
}: ListGroupsParams = {}): Promise<GroupListPage> {
  const params = new URLSearchParams();
  if (q !== undefined && q.trim() !== "") params.set("q", q);
  if (cursor !== undefined) params.set("cursor", cursor);
  if (limit !== undefined) params.set("limit", String(limit));
  const query = params.toString();
  return request<GroupListPage>(query === "" ? "/api/groups" : `/api/groups?${query}`, {
    signal,
  });
}

/**
 * createGroup は動画とグループからグループを作る（POST /api/groups）。名前が空か長すぎるか、
 * メンバーが無ければ 400 `invalid_request`、動画かグループのどれかが無くなっていれば 404
 * `group_target_not_found` の RequestFailed になり、何も作られない。
 */
export function createGroup(
  name: string,
  members: GroupMembers,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return request<LibraryGroup>("/api/groups", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ name, ...membersBody(members) }),
    signal,
  });
}

/** renameGroup はグループの名前を変え、前後を削った名前の値を返す（PATCH /api/groups/{id}）。 */
export function renameGroup(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return request<LibraryGroup>(groupPath(id), {
    method: "PATCH",
    headers: jsonHeaders,
    body: JSON.stringify({ name }),
    signal,
  });
}

/**
 * dissolveGroup はグループを解散する（DELETE /api/groups/{id}）。子グループと動画は残る。
 * 無ければ 404 の RequestFailed。
 */
export async function dissolveGroup(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(groupPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * addToGroup は動画とグループをグループの末尾に足し、足した後の値を返す
 * （POST /api/groups/{id}/items）。既にあるものは飛ばす。無くなった対象は 404
 * `group_target_not_found`、このグループかその祖先を入れようとすると 409 `group_cycle` の
 * RequestFailed になり、何も足されない。
 */
export function addToGroup(
  id: number,
  members: GroupMembers,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return request<LibraryGroup>(`${groupPath(id)}/items`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(membersBody(members)),
    signal,
  });
}

/**
 * removeFromGroup は項目（動画やグループの id ではない）を外し、外した後の値を返す
 * （POST /api/groups/{id}/items/remove）。グループに無い項目があれば 404
 * `group_target_not_found` の RequestFailed になり、何も外されない。
 */
export function removeFromGroup(
  id: number,
  itemIds: readonly number[],
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return request<LibraryGroup>(`${groupPath(id)}/items/remove`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ itemIds: Array.from(itemIds) }),
    signal,
  });
}

/**
 * moveGroupItem は項目 `itemId` を項目 `after` のすぐ後ろへ移す。`null` なら先頭へ移す
 * （POST /api/groups/{id}/items/{itemId}/move）。項目か `after` がグループに無ければ 404
 * `group_target_not_found` の RequestFailed になる。
 */
export async function moveGroupItem(
  id: number,
  itemId: number,
  after: number | null,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(`${groupPath(id)}/items/${String(itemId)}/move`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(after === null ? {} : { after }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * tagGroup はグループをタグにする（POST /api/groups/{id}/tag）。子孫の全動画にタグが手で付き、
 * グループは解散する。名前がタグ名に使えなければ 400 `invalid_request` の RequestFailed になり、
 * 何も変わらない。
 *
 * 成功したら一覧の控えを捨て（ライブラリのカードが変わる）、共有のタグの一覧も取り直す（タグが
 * 増えるか、本数が変わる）。
 */
export async function tagGroup(
  id: number,
  signal?: AbortSignal,
): Promise<GroupTagResult> {
  const result = await request<GroupTagResult>(`${groupPath(id)}/tag`, {
    method: "POST",
    signal,
  });
  clearListSnapshot();
  refreshTags().catch(() => undefined);
  return result;
}

/** taggedMessage はグループをタグに変えたことを伝える文言である（specs/048-groups/ui-design.md「Words」）。 */
export function taggedMessage(result: GroupTagResult): UiText {
  return t.groups.tagged(result.tag.name, result.created);
}
