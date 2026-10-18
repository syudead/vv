import type {
  ActivityConditions,
  ActivityDecider,
  ActivityFeature,
  ActivityStatus,
} from "../api/activity";
import { normalizeQuery } from "../videoList/listCriteria";

/** Omit defaults from URLs and normalize invalid values to defaults. */

export type ActivityPeriod = "any" | "today" | "week" | "month";

export interface ActivityCriteria {
  /** Undefined means All. */
  status?: ActivityStatus;
  /** Empty means All; order follows the feature catalog. */
  features: readonly ActivityFeature[];
  /** Undefined means All. */
  decider?: ActivityDecider;
  period: ActivityPeriod;
  /** Trimmed query, capped at 200 characters. */
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

/** Automatic features appear before the menu separator. */
export const runFeatures: readonly ActivityFeature[] = [
  "import",
  "scan",
  "duplicate_check",
  "auto_tagging",
];

/** Owner actions appear after the menu separator. */
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

export const ACTIVITY_QUERY_LIMIT = 200;

function oneOf<T extends string>(
  values: readonly T[],
  value: string | null,
): T | undefined {
  return value !== null && (values as readonly string[]).includes(value)
    ? (value as T)
    : undefined;
}

export function normalizeActivityQuery(value: string): string {
  return normalizeQuery(value, ACTIVITY_QUERY_LIMIT);
}

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

/** Omit defaults and keep a stable status, feature, decider, period, q order. */
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

export function activityCriteriaKey(criteria: ActivityCriteria): string {
  return serializeActivityCriteria(criteria).toString();
}

export function hasActivityConditions(criteria: ActivityCriteria): boolean {
  return activityCriteriaKey(criteria) !== "";
}

const dayMs = 24 * 60 * 60 * 1000;

/** Today starts at local midnight; other periods subtract elapsed days; any has no boundary. */
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
