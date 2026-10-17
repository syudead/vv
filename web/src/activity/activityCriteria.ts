import type {
  ActivityConditions,
  ActivityDecider,
  ActivityFeature,
  ActivityStatus,
} from "../api/activity";
import { normalizeQuery } from "../videoList/listCriteria";

/**
 * アクティビティの画面の絞り込みと URL の相互変換。既定値は書かず、読めない値は既定値として扱う。React に依存しない純粋な関数
 * だけを置く。
 */

/** ActivityPeriod は期間の絞り込みである。any は絞り込まない。 */
export type ActivityPeriod = "any" | "today" | "week" | "month";

/** ActivityCriteria は一覧を決める絞り込みである。 */
export interface ActivityCriteria {
  /** 状態。undefined は All。 */
  status?: ActivityStatus;
  /** 機能。空は All。並びは機能の表の順。 */
  features: readonly ActivityFeature[];
  /** 決めたもの。undefined は All。 */
  decider?: ActivityDecider;
  period: ActivityPeriod;
  /** 対象の表示名の検索語。前後の空白を落とし、200 文字で切ったもの。 */
  q: string;
}

export const DEFAULT_ACTIVITY_CRITERIA: ActivityCriteria = {
  features: [],
  period: "any",
  q: "",
};

export const activityStatuses: readonly ActivityStatus[] = [
  "running",
  "attention",
  "done",
];

/** 自動の処理の機能。メニューでは区切りの前に置く。 */
export const runFeatures: readonly ActivityFeature[] = [
  "import",
  "scan",
  "duplicate_check",
  "auto_tagging",
];

/** 所有者の操作の機能。メニューでは区切りの後に置く。 */
export const ownerFeatures: readonly ActivityFeature[] = [
  "work_info",
  "tags",
  "versions",
  "media_folders",
  "organize",
];

export const activityFeatures: readonly ActivityFeature[] = [
  ...runFeatures,
  ...ownerFeatures,
];

export const activityDeciders: readonly ActivityDecider[] = ["ai", "rule", "owner"];

export const activityPeriods: readonly ActivityPeriod[] = [
  "any",
  "today",
  "week",
  "month",
];

/** 検索語の上限（契約の `q` の 1–200 文字）。 */
export const ACTIVITY_QUERY_LIMIT = 200;

function oneOf<T extends string>(
  values: readonly T[],
  value: string | null,
): T | undefined {
  return value !== null && (values as readonly string[]).includes(value)
    ? (value as T)
    : undefined;
}

/** normalizeActivityQuery は検索語の前後の空白を落とし、上限の文字数で切る。 */
export function normalizeActivityQuery(value: string): string {
  return normalizeQuery(value, ACTIVITY_QUERY_LIMIT);
}

/** parseActivityCriteria は URL のクエリを絞り込みにする。読めない値は既定にする。 */
export function parseActivityCriteria(params: URLSearchParams): ActivityCriteria {
  const chosen = new Set(params.getAll("feature"));
  const status = oneOf(activityStatuses, params.get("status"));
  const decider = oneOf(activityDeciders, params.get("decider"));
  return {
    ...(status === undefined ? {} : { status }),
    features: activityFeatures.filter((feature) => chosen.has(feature)),
    ...(decider === undefined ? {} : { decider }),
    period: oneOf(activityPeriods, params.get("period")) ?? "any",
    q: normalizeActivityQuery(params.get("q") ?? ""),
  };
}

/**
 * serializeActivityCriteria は絞り込みを URL のクエリにする。既定の値は書かない。順は
 * status・feature・decider・period・q で固定し、同じ絞り込みは同じ文字列になる。
 */
export function serializeActivityCriteria(criteria: ActivityCriteria): URLSearchParams {
  const params = new URLSearchParams();
  if (criteria.status !== undefined) params.set("status", criteria.status);
  const chosen = new Set(criteria.features);
  for (const feature of activityFeatures)
    if (chosen.has(feature)) params.append("feature", feature);
  if (criteria.decider !== undefined) params.set("decider", criteria.decider);
  if (criteria.period !== "any") params.set("period", criteria.period);
  const q = normalizeActivityQuery(criteria.q);
  if (q !== "") params.set("q", q);
  return params;
}

/** activityCriteriaKey は絞り込みを比較・依存配列に使う文字列にする。 */
export function activityCriteriaKey(criteria: ActivityCriteria): string {
  return serializeActivityCriteria(criteria).toString();
}

/** hasActivityConditions は「Clear filters」で外せる絞り込みがあるか。 */
export function hasActivityConditions(criteria: ActivityCriteria): boolean {
  return activityCriteriaKey(criteria) !== "";
}

const dayMs = 24 * 60 * 60 * 1000;

/**
 * periodSince は期間の始まりの時刻を返す。Today は見る人のローカルの 0 時、Last 7 days と
 * Last 30 days は今からその日数だけ前。any は undefined。
 */
export function periodSince(period: ActivityPeriod, now: Date): Date | undefined {
  switch (period) {
    case "any":
      return undefined;
    case "today":
      return new Date(now.getFullYear(), now.getMonth(), now.getDate());
    case "week":
      return new Date(now.getTime() - 7 * dayMs);
    case "month":
      return new Date(now.getTime() - 30 * dayMs);
  }
}

/** activityConditions は絞り込みを一覧の要求の条件にする。 */
export function activityConditions(
  criteria: ActivityCriteria,
  now: Date,
): ActivityConditions {
  const since = periodSince(criteria.period, now);
  return {
    ...(criteria.status === undefined ? {} : { status: criteria.status }),
    ...(criteria.features.length === 0 ? {} : { features: criteria.features }),
    ...(criteria.decider === undefined ? {} : { decider: criteria.decider }),
    ...(since === undefined ? {} : { since: since.toISOString() }),
    ...(criteria.q === "" ? {} : { q: criteria.q }),
  };
}
