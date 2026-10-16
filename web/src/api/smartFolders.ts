import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { dropSnapshotAfter } from "./groups";

// スマートフォルダの画面 API（specs/052-smart-folders/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。読み取りはゲストも読め、書き込みは所有者だけである。
// ライブラリの一覧はフォルダの項目を要約で返し、カードの値は readSmartFolderCards で読む
// （research.md R-14）。

export type SmartRule = components["schemas"]["SmartRule"];
export type SmartRuleRef = components["schemas"]["SmartRuleRef"];
export type SmartFolderSummary = components["schemas"]["SmartFolderSummary"];
export type LibrarySmartFolder = components["schemas"]["LibrarySmartFolder"];
export type SmartFolderSettings = components["schemas"]["SmartFolderSettings"];
export type SmartFolderItem = components["schemas"]["SmartFolderItem"];
export type SmartFolderDetail = components["schemas"]["SmartFolderDetail"];
export type SmartFolderItemPage = components["schemas"]["SmartFolderItemPage"];
export type SmartFolderCreate = components["schemas"]["SmartFolderCreate"];
export type SmartFolderChange = components["schemas"]["SmartFolderChange"];

/** MAX_SMART_FOLDER_CARDS は readSmartFolderCards 1 回で名指せるフォルダの数の上限である。 */
export const MAX_SMART_FOLDER_CARDS = 60;

const jsonHeaders = { "Content-Type": "application/json" };

function smartFolderPath(id: number): string {
  return `/api/smart-folders/${String(id)}`;
}

/**
 * listSmartFolders は見る人に見せるスマートフォルダの要約を、どの深さのものも自然な名前の順に
 * 取得する（GET /api/smart-folders）。親を選ぶ欄とフォルダの木が読む。
 */
export async function listSmartFolders(
  signal?: AbortSignal,
): Promise<SmartFolderSummary[]> {
  const list = await request<components["schemas"]["SmartFolderList"]>(
    "/api/smart-folders",
    {
      signal,
    },
  );
  return list.smartFolders;
}

/**
 * readSmartFolderPage はスマートフォルダ 1 つと、その祖先と中身の最初のページを取得する
 * （GET /api/smart-folders/{id}）。`limit` を省くとサーバーの既定の件数。無いか見せないときは
 * 404 の RequestFailed で失敗する。
 */
export function readSmartFolderPage(
  id: number,
  signal?: AbortSignal,
  limit?: number,
): Promise<SmartFolderDetail> {
  const query = limit === undefined ? "" : `?limit=${String(limit)}`;
  return request<SmartFolderDetail>(`${smartFolderPath(id)}${query}`, { signal });
}

/**
 * listSmartFolderItems はスマートフォルダの中身の続きを 1 ページ取得する
 * （GET /api/smart-folders/{id}/items）。`cursor` は前の応答の `nextCursor` で、中身は解釈しない。
 */
export function listSmartFolderItems(
  id: number,
  cursor: string,
  signal?: AbortSignal,
  limit?: number,
): Promise<SmartFolderItemPage> {
  const query = new URLSearchParams({ cursor });
  if (limit !== undefined) query.set("limit", String(limit));
  return request<SmartFolderItemPage>(
    `${smartFolderPath(id)}/items?${query.toString()}`,
    {
      signal,
    },
  );
}

/**
 * readSmartFolderCards は画面に出たフォルダ `ids`（1〜60 件）のカードの値を、`ids` の順に取得する
 * （GET /api/smart-folders/cards、data-model.md「Cards read」）。無いか見せないフォルダは入らない。
 * `ids` が空なら送らずに空を返す。
 */
export async function readSmartFolderCards(
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<LibrarySmartFolder[]> {
  if (ids.length === 0) return [];
  // id は数字だけなので、区切りの `,` はそのまま載せる（style: form, explode: false）。
  const answer = await request<components["schemas"]["SmartFolderCards"]>(
    `/api/smart-folders/cards?ids=${ids.map(String).join(",")}`,
    { signal },
  );
  return answer.cards;
}

/**
 * createSmartFolder はスマートフォルダを作り、その要約を返す（POST /api/smart-folders）。条件か
 * 並べ順の違反は 400 `invalid_smart_rule`、名前の違反は 400 `invalid_request`、無い親は 404
 * `smart_folder_not_found` の RequestFailed になり、何も作られない。成功したら一覧の控えを捨てる
 * （ライブラリにフォルダのカードが増える）。
 */
export function createSmartFolder(
  folder: SmartFolderCreate,
  signal?: AbortSignal,
): Promise<SmartFolderSummary> {
  return dropSnapshotAfter(
    request<SmartFolderSummary>("/api/smart-folders", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(folder),
      signal,
    }),
  );
}

/**
 * changeSmartFolder はスマートフォルダの名前・条件・並べ順・親のうち `change` が持つものを変え、
 * 要約を返す（PATCH /api/smart-folders/{id}）。`parentId: null` は一番上にする。自分か子孫の下へ
 * 移すと 409 `smart_folder_cycle` の RequestFailed になる。成功したら一覧の控えを捨てる。
 */
export function changeSmartFolder(
  id: number,
  change: SmartFolderChange,
  signal?: AbortSignal,
): Promise<SmartFolderSummary> {
  return dropSnapshotAfter(
    request<SmartFolderSummary>(smartFolderPath(id), {
      method: "PATCH",
      headers: jsonHeaders,
      body: JSON.stringify(change),
      signal,
    }),
  );
}

/**
 * deleteSmartFolder はスマートフォルダとその子孫を消す（DELETE /api/smart-folders/{id}）。動画は
 * 変わらない。無ければ 404 の RequestFailed。成功したら一覧の控えを捨てる。
 */
export function deleteSmartFolder(id: number, signal?: AbortSignal): Promise<void> {
  return dropSnapshotAfter(
    (async () => {
      const response = await apiFetch(smartFolderPath(id), { method: "DELETE", signal });
      if (!response.ok) throw await toRequestFailed(response);
    })(),
  );
}
