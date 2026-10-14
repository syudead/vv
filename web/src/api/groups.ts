import { type GroupDetail, request } from "./client";
import type { components } from "./gen/openapi";

// グループの画面 API（specs/048-groups/contracts/screen-api.md）。型は api/openapi.yaml からの
// 生成物を使う。読む経路は所有者とゲストの両方に開いている。

export type GroupItem = components["schemas"]["GroupItem"];
export type GroupRef = components["schemas"]["GroupRef"];
export type GroupItemPage = components["schemas"]["GroupItemPage"];

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
