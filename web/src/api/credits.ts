import { request } from "./client";
import type { components } from "./gen/openapi";

// 人物と団体は同じ形の経路を `/api/people` と `/api/organizations` に持つので、1 つの
// モジュールが種類（CreditKind）を受け取って両方を扱う
// （specs/047-video-work-info/plan.md の Structure decision、contracts/screen-api.md
// 「People and organizations」）。
export type Person = components["schemas"]["Person"];
export type Organization = components["schemas"]["Organization"];
export type CreditInput = components["schemas"]["CreditInput"];
export type CreditRef = components["schemas"]["CreditRef"];

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

/** CreditListQuery は `listCredits` の条件である。 */
export interface CreditListQuery {
  /** 名前と別名に部分一致させる語。先頭 100 文字に切って送る。 */
  q?: string;
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

/**
 * getCredit は人物か団体を 1 件読む（`GET /api/people/{id}`・`GET /api/organizations/{id}`）。
 * 無ければ `404`（`person_not_found`・`organization_not_found`）の RequestFailed になる。
 */
export function getCredit(
  kind: CreditKind,
  id: number,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(`${basePath(kind)}/${String(id)}`, { signal });
}
