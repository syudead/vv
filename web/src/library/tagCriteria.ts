import { MAX_TAG_FILTER_COUNT } from "../api/client";
import {
  clearConditions as clearBaseConditions,
  hasConditions as hasBaseConditions,
  type ListCriteria,
} from "../videoList/listCriteria";
import { type CreditFilter, hasCreditFilter } from "./creditCriteria";

/** Keep library-only tag filters out of shared ListCriteria and folder URLs. */

export const MAX_TAG_COUNT = MAX_TAG_FILTER_COUNT;

export const TAG_PARAM = "tag";

/** Ignore invalid IDs, duplicates, and entries beyond the 16-tag limit. */
export function parseTagParam(raw: readonly string[]): number[] {
  const seen = new Set<number>();
  const ids: number[] = [];
  for (const value of raw) {
    if (!/^\d+$/.test(value)) continue;
    const id = Number(value);
    if (!Number.isSafeInteger(id) || seen.has(id)) continue;
    seen.add(id);
    ids.push(id);
    if (ids.length >= MAX_TAG_COUNT) break;
  }
  return ids;
}

// Sort IDs for stable snapshot keys.
export function serializeTagIds(ids: readonly number[]): string[] {
  return Array.from(new Set(ids))
    .sort((a, b) => a - b)
    .slice(0, MAX_TAG_COUNT)
    .map(String);
}

export function addTagId(current: readonly number[], id: number): number[] {
  if (current.includes(id)) return [...current];
  if (current.length >= MAX_TAG_COUNT) return [...current];
  return [...current, id];
}

export function removeTagId(current: readonly number[], id: number): number[] {
  return current.filter((existing) => existing !== id);
}

export function hasConditions(
  criteria: ListCriteria,
  tag: readonly number[],
  credits: CreditFilter = {},
): boolean {
  return tag.length > 0 || hasCreditFilter(credits) || hasBaseConditions(criteria);
}

/** Clear tag filters while preserving sort and shuffle seed. */
export function clearConditions(criteria: ListCriteria): {
  criteria: ListCriteria;
  tag: number[];
} {
  return { criteria: clearBaseConditions(criteria), tag: [] };
}
