import { apiFetch, request, RequestFailed, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// 人物と団体は同じ形の経路を `/api/people` と `/api/organizations` に持つので、1 つの
// モジュールが種類（CreditKind）を受け取って両方を扱う
// （specs/047-video-work-info/plan.md の Structure decision、contracts/screen-api.md
// 「People and organizations」）。
export type Person = components["schemas"]["Person"];
export type Organization = components["schemas"]["Organization"];
export type CreditInput = components["schemas"]["CreditInput"];
export type CreditRef = components["schemas"]["CreditRef"];
export type CreditPatch = components["schemas"]["CreditPatch"];
export type CreditSort = components["schemas"]["CreditSort"];
export type Alias = components["schemas"]["Alias"];
export type ExternalId = components["schemas"]["ExternalId"];

/** CreditKind は人物か団体かである。 */
export type CreditKind = "person" | "organization";

/** Credit は人物か団体 1 件である（団体には `parent` が加わる）。 */
export type Credit = Person | Organization;

/** CreditList は人物か団体の一覧の 1 ページである。 */
export interface CreditList {
  items: Credit[];
  total: number;
  totalAll: number;
  nextCursor?: string;
}

/** maxCreditQueryLength は一覧の `q` に許す長さである（api/openapi.yaml の maxLength）。 */
export const maxCreditQueryLength = 100;

function basePath(kind: CreditKind): string {
  return kind === "person" ? "/api/people" : "/api/organizations";
}

function itemPath(kind: CreditKind, id: number): string {
  return `${basePath(kind)}/${String(id)}`;
}

/** CreditListQuery は `listCredits` の条件である。 */
export interface CreditListQuery {
  /** 名前と別名に部分一致させる語。先頭 100 文字に切って送る。 */
  q?: string;
  /** 並び順。省くとサーバーの既定（名前の自然順）。 */
  sort?: CreditSort;
  /** 1 ページの件数（1〜200）。省くと当たるものをすべて返す。 */
  limit?: number;
  cursor?: string;
}

/**
 * listCredits は人物か団体を名前の自然順で読む（`GET /api/people`・`GET /api/organizations`）。
 * `q` は名前と別名に照合形で部分一致し、一覧の契約が 100 文字までしか受け付けないので
 * 先頭 100 文字に切る（名前は 200 文字まであるが、その名前に当たるものはどれも先頭を含む）。
 */
export function listCredits(
  kind: CreditKind,
  query: CreditListQuery = {},
  signal?: AbortSignal,
): Promise<CreditList> {
  const params = new URLSearchParams();
  const q = Array.from(query.q ?? "")
    .slice(0, maxCreditQueryLength)
    .join("");
  if (q !== "") params.set("q", q);
  if (query.sort !== undefined) params.set("sort", query.sort);
  if (query.limit !== undefined) params.set("limit", String(query.limit));
  if (query.cursor !== undefined && query.cursor !== "")
    params.set("cursor", query.cursor);
  const search = params.toString();
  return request<CreditList>(`${basePath(kind)}${search === "" ? "" : `?${search}`}`, {
    signal,
  });
}

/** createCredit は人物か団体を 1 件作る（`POST /api/people`・`POST /api/organizations`）。 */
export function createCredit(
  kind: CreditKind,
  input: CreditInput,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(basePath(kind), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal,
  });
}

/** getCredit は人物か団体を 1 件読む（`GET /api/people/{id}`・`GET /api/organizations/{id}`）。 */
export function getCredit(
  kind: CreditKind,
  id: number,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(itemPath(kind, id), { signal });
}

/**
 * updateCredit は人物か団体を書き換える（`PATCH`）。送ったプロパティだけを置き換え、
 * 一覧は丸ごと置き換える（`null` は空にし、`parentId: null` は親を外す。research.md R-8）。
 */
export function updateCredit(
  kind: CreditKind,
  id: number,
  patch: CreditPatch,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(itemPath(kind, id), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
    signal,
  });
}

/** CreditMergeResult は統合の結果である。統合先の最新の状態と、もう無かった統合元。 */
export interface CreditMergeResult {
  credit: Credit;
  notFoundIds: number[];
}

/**
 * mergeCredits は `sourceIds` を `id` へ統合する（`POST …/{id}/merge`）。統合元の名前・別名・
 * 外部 ID は統合先の別名・外部 ID になり、動画の結び付きは統合先へ移る。
 */
export async function mergeCredits(
  kind: CreditKind,
  id: number,
  sourceIds: readonly number[],
  signal?: AbortSignal,
): Promise<CreditMergeResult> {
  const body = await request<{
    person?: Credit;
    organization?: Credit;
    notFoundIds: number[];
  }>(`${itemPath(kind, id)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceIds }),
    signal,
  });
  const credit = kind === "person" ? body.person : body.organization;
  if (credit === undefined) throw new RequestFailed(500, "", "");
  return { credit, notFoundIds: body.notFoundIds };
}

/** deleteCredit は人物か団体を 1 件消す（`DELETE`）。動画との結び付きも外れる。 */
export async function deleteCredit(
  kind: CreditKind,
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(itemPath(kind, id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * isCreditGone は、操作の対象の人物・団体そのものがもう無いという失敗かである
 * （`404` の `person_not_found`・`organization_not_found` で `field` が無いもの）。`field` の
 * あるもの（`parentId` の団体が無い）は入力の誤りで、ここには入らない。
 */
export function isCreditGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    (error.reason === "person_not_found" || error.reason === "organization_not_found") &&
    error.field === undefined
  );
}
