import { t, type UiText } from "../i18n";
import {
  apiFetch,
  type GroupDetail,
  type LibraryGroup,
  MAX_PAGE_SIZE,
  request,
  RequestFailed,
  toRequestFailed,
} from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { refreshTags } from "./tags";

// グループの一覧と編集の画面 API（specs/048-groups/contracts/screen-api.md「Statuses」）。型は
// api/openapi.yaml からの生成物を使う。一覧はゲストも読め、書き込みは所有者だけである。
// 一覧の項目の取り直しに使うグループ 1 つの読み出し（getGroup）は client.ts にあり、グループの
// ページは道筋付きの readGroupPage と続きの listGroupItems で読む。

export type GroupItem = components["schemas"]["GroupItem"];
export type GroupRef = components["schemas"]["GroupRef"];
export type GroupItemPage = components["schemas"]["GroupItemPage"];
export type GroupListPage = components["schemas"]["GroupListPage"];
export type GroupTagResult = components["schemas"]["GroupTagResult"];

const jsonHeaders = { "Content-Type": "application/json" };

function groupPath(id: number): string {
  return `/api/groups/${String(id)}`;
}

/**
 * readGroupPage はグループ 1 つと、その道筋と項目の最初のページを取得する
 * （GET /api/groups/{id}?trail=）。`trail` は画面が通ってきたグループの id で、外側から順に
 * 並べる。空なら送らない。応答の `trail` は、そのうち今も親と子の組が続く部分である
 * （research.md R-8）。無いか見せないときは 404 の RequestFailed で失敗する。
 */
export function readGroupPage(
  id: number,
  trail: readonly number[],
  signal?: AbortSignal,
): Promise<GroupDetail> {
  // id は数字だけなので、区切りの `,` はそのまま載せる（style: form, explode: false）。
  const query = trail.length === 0 ? "" : `?trail=${trail.map(String).join(",")}`;
  return request<GroupDetail>(`${groupPath(id)}${query}`, { signal });
}

/**
 * listGroupItems はグループの項目の続きを 1 ページ取得する（GET /api/groups/{id}/items）。
 * `cursor` は前の応答の `nextCursor` で、中身は解釈しない。
 */
export function listGroupItems(
  id: number,
  cursor: string,
  signal?: AbortSignal,
): Promise<GroupItemPage> {
  const query = new URLSearchParams({ cursor });
  return request<GroupItemPage>(`${groupPath(id)}/items?${query.toString()}`, {
    signal,
  });
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
 * listAllGroups は見る人に見せるグループの全部を、`nextCursor` が尽きるまで最大の件数
 * （MAX_PAGE_SIZE）のページで辿って取得する。名前で選ぶ「Add to group」のピッカーが、一覧の
 * 全部を手元で絞るのに使う（specs/048-groups/ui-design.md「Add to group」）。グループの数に
 * 上限は無いので、ページ数では打ち切らない。同じカーソルが二度返ったら、終わらない読みに
 * なるので欠けた一覧を返さずに失敗にする。
 */
export async function listAllGroups(signal?: AbortSignal): Promise<LibraryGroup[]> {
  const groups: LibraryGroup[] = [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  do {
    const answer = await listGroups({ cursor, limit: MAX_PAGE_SIZE, signal });
    groups.push(...answer.groups);
    cursor = answer.nextCursor;
    if (cursor !== undefined && seen.has(cursor)) {
      throw new Error("The group list returned the same cursor twice.");
    }
    if (cursor !== undefined) seen.add(cursor);
  } while (cursor !== undefined);
  return groups;
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
  return dropSnapshotAfter(
    request<LibraryGroup>("/api/groups", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ name, ...membersBody(members) }),
      signal,
    }),
  );
}

/**
 * renameGroup はグループの名前を変え、前後を削った名前の値を返す（PATCH /api/groups/{id}）。
 * 成功したら一覧の控えを捨てる（ライブラリのカードの名前が変わる）。
 */
export async function renameGroup(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  const group = await request<LibraryGroup>(groupPath(id), {
    method: "PATCH",
    headers: jsonHeaders,
    body: JSON.stringify({ name }),
    signal,
  });
  clearListSnapshot();
  return group;
}

/**
 * dissolveGroup はグループを解散する（DELETE /api/groups/{id}）。子グループと動画は残る。
 * 無ければ 404 の RequestFailed。成功したら一覧の控えを捨てる（ライブラリのカードが変わる）。
 */
export async function dissolveGroup(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(groupPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
  clearListSnapshot();
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
  return dropSnapshotAfter(
    request<LibraryGroup>(`${groupPath(id)}/items`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(membersBody(members)),
      signal,
    }),
  );
}

/**
 * dropSnapshotAfter は作成と追加の後に一覧の控えを捨てる。足したものはライブラリのいちばん上から
 * 外れるが、控えは書き換えないので、動画ページや別のグループのページから足して戻ると古い一覧が
 * 戻ってしまう。404（途中で対象が無くなった）でも、控えにその項目が残るので捨てる。
 */
async function dropSnapshotAfter<T>(sent: Promise<T>): Promise<T> {
  try {
    const value = await sent;
    clearListSnapshot();
    return value;
  } catch (failure) {
    if (failure instanceof RequestFailed && failure.status === 404) clearListSnapshot();
    throw failure;
  }
}

/**
 * removeFromGroup は項目（動画やグループの id ではない）を外し、外した後の値を返す
 * （POST /api/groups/{id}/items/remove）。最後の項目を外すとサーバーはグループを解散して本文の
 * 無い `204` を返すので、そのときは `null` を返す。グループに無い項目があれば 404
 * `group_target_not_found` の RequestFailed になり、何も外されない。成功したら一覧の控えを
 * 捨てる（ライブラリのカードの本数とプレビューが変わり、外した項目が上に出ることもある）。
 */
export async function removeFromGroup(
  id: number,
  itemIds: readonly number[],
  signal?: AbortSignal,
): Promise<LibraryGroup | null> {
  const response = await apiFetch(`${groupPath(id)}/items/remove`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ itemIds: Array.from(itemIds) }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  const group =
    response.status === 204 ? null : ((await response.json()) as LibraryGroup);
  clearListSnapshot();
  return group;
}

/**
 * moveGroupItem は項目 `itemId` を項目 `after` のすぐ後ろへ移す。`null` なら先頭へ移す
 * （POST /api/groups/{id}/items/{itemId}/move）。項目か `after` がグループに無ければ 404
 * `group_target_not_found` の RequestFailed になる。成功したら一覧の控えを捨てる（ライブラリの
 * カードのプレビューと最初に開く動画が並びで決まる）。
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
  clearListSnapshot();
}

/**
 * tagGroup はグループをタグにする（POST /api/groups/{id}/tag）。子孫の全動画にタグが手で付き、
 * グループは解散する。名前がタグ名に使えなければ 400 `invalid_request` の RequestFailed になり、
 * 何も変わらない。
 *
 * 成功したら一覧の控えを捨て（ライブラリのカードが変わる）、共有のタグの一覧も取り直す（タグが
 * 増えるか、本数が変わる）。404（途中で解散された）でも、控えにそのカードが残るので捨てる。
 */
export async function tagGroup(
  id: number,
  signal?: AbortSignal,
): Promise<GroupTagResult> {
  const result = await dropSnapshotAfter(
    request<GroupTagResult>(`${groupPath(id)}/tag`, { method: "POST", signal }),
  );
  refreshTags().catch(() => undefined);
  return result;
}

/** taggedMessage はグループをタグに変えたことを伝える文言である（specs/048-groups/ui-design.md「Words」）。 */
export function taggedMessage(result: GroupTagResult): UiText {
  return t.groups.tagged(result.tag.name, result.created);
}
