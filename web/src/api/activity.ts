import { request } from "./client";
import type { components } from "./gen/openapi";

// アクティビティの記録の画面 API（specs/055-activity/contracts/screen-api.md）。型は
// api/openapi.yaml からの生成物を使う。

export type ActivityPage = components["schemas"]["ActivityPage"];
export type ActivityEntry = components["schemas"]["ActivityEntry"];
export type ActivityKind = components["schemas"]["ActivityKind"];
export type ActivityFeature = components["schemas"]["ActivityFeature"];
export type ActivityStatus = components["schemas"]["ActivityStatus"];
export type ActivityReason = components["schemas"]["ActivityReason"];
export type ActivityOutcome = components["schemas"]["ActivityOutcome"];
export type ActivityDecider = components["schemas"]["ActivityDecider"];
export type ActivityStage = components["schemas"]["ActivityStage"];
export type ActivityCounts = components["schemas"]["ActivityCounts"];
export type ActivityFeatureCounts = components["schemas"]["ActivityFeatureCounts"];

/** ACTIVITY_PAGE_SIZE は 1 ページの件数（契約の既定の 50）である。 */
export const ACTIVITY_PAGE_SIZE = 50;

/** ActivityConditions は一覧の絞り込みである。省いた項目は絞り込まない。 */
export interface ActivityConditions {
  status?: ActivityStatus;
  features?: readonly ActivityFeature[];
  decider?: ActivityDecider;
  /** この時刻以降に作った記録だけにする（ISO 8601）。 */
  since?: string;
  /** 対象の表示名に含む文字列。空なら送らない。 */
  q?: string;
}

export interface ListActivityParams extends ActivityConditions {
  /** 前回の応答の `nextCursor`。中身は解釈しない。 */
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

/** activityQuery は条件を GET /api/activity の問い合わせにする。 */
export function activityQuery({
  status,
  features,
  decider,
  since,
  q,
  cursor,
  limit = ACTIVITY_PAGE_SIZE,
}: Omit<ListActivityParams, "signal">): URLSearchParams {
  const params = new URLSearchParams();
  if (status !== undefined) params.set("status", status);
  for (const feature of features ?? []) params.append("feature", feature);
  if (decider !== undefined) params.set("decider", decider);
  if (since !== undefined) params.set("since", since);
  if (q !== undefined && q !== "") params.set("q", q);
  if (cursor !== undefined) params.set("cursor", cursor);
  params.set("limit", String(limit));
  return params;
}

/**
 * listActivity は条件を満たす記録を新しい順に 1 ページ取得する（GET /api/activity）。応答の
 * `counts` は status 以外、`features` は feature 以外の条件に従う。
 */
export function listActivity({
  signal,
  ...params
}: ListActivityParams = {}): Promise<ActivityPage> {
  return request<ActivityPage>(`/api/activity?${activityQuery(params).toString()}`, {
    signal,
  });
}
