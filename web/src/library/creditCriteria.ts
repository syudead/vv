import { useMemo } from "react";

import type { CreditKind } from "../api/credits";

/** Keep library-only credit filters out of shared ListCriteria and folder URLs. */

export const PERSON_PARAM = "person";

export const ORGANIZATION_PARAM = "organization";

export const TAG_KIND_PARAM = "tagKind";

/** Owner-only; keeps videos bundled with other versions. */
export const VERSIONS_PARAM = "versions";

export const creditKinds: readonly CreditKind[] = ["person", "organization"];

export interface CreditFilter {
  person?: number;
  organization?: number;
  tagKind?: number;
  /** True or absent; smart folders cannot save it, so it is not a chip or a saved condition. */
  hasVersions?: true;
}

function creditParam(kind: CreditKind): string {
  return kind === "person" ? PERSON_PARAM : ORGANIZATION_PARAM;
}

/** Use the first valid positive ID; ignore invalid or duplicate values. */
export function parseCreditParam(raw: readonly string[]): number | undefined {
  for (const value of raw) {
    if (!/^\d+$/.test(value)) continue;
    const id = Number(value);
    if (Number.isSafeInteger(id) && id > 0) return id;
  }
  return undefined;
}

export function parseCreditFilter(search: URLSearchParams): CreditFilter {
  return {
    person: parseCreditParam(search.getAll(PERSON_PARAM)),
    organization: parseCreditParam(search.getAll(ORGANIZATION_PARAM)),
    tagKind: parseCreditParam(search.getAll(TAG_KIND_PARAM)),
    ...(search.get(VERSIONS_PARAM) === "1" ? { hasVersions: true } : {}),
  };
}

export function hasCreditFilter(filter: CreditFilter): boolean {
  return (
    filter.person !== undefined ||
    filter.organization !== undefined ||
    filter.tagKind !== undefined
  );
}

/** Keep identity stable; guests receive no filters, and useListCriteria clears stale URL values. */
export function useCreditFilter(search: string, owner: boolean): CreditFilter {
  const parsed = parseCreditFilter(new URLSearchParams(search));
  const person = owner ? parsed.person : undefined;
  const organization = owner ? parsed.organization : undefined;
  const tagKind = owner ? parsed.tagKind : undefined;
  const hasVersions = owner && parsed.hasVersions === true;
  return useMemo(
    () => ({ person, organization, tagKind, ...(hasVersions ? { hasVersions } : {}) }),
    [person, organization, tagKind, hasVersions],
  );
}

export function creditSignature(filter: CreditFilter): string {
  return [filter.person, filter.organization, filter.tagKind, filter.hasVersions]
    .map((id) => String(id ?? ""))
    .join("\u0000");
}

export function hasFilterChips(tagIds: readonly number[], filter: CreditFilter): boolean {
  return tagIds.length > 0 || hasCreditFilter(filter);
}

/** Remove only the requested filter, preserving unrelated URL parameters. */
export function withoutCredit(kind: CreditKind): Record<string, string[]> {
  return { [creditParam(kind)]: [] };
}

export const withoutTagKind: Readonly<Record<string, string[]>> = {
  [TAG_KIND_PARAM]: [],
};

export function withTagKind(id: number): Record<string, string[]> {
  return { [TAG_KIND_PARAM]: [String(id)] };
}

export function withVersions(on: boolean): Record<string, string[]> {
  return { [VERSIONS_PARAM]: on ? ["1"] : [] };
}

export const clearedCredits: Readonly<Record<string, string[]>> = {
  [PERSON_PARAM]: [],
  [ORGANIZATION_PARAM]: [],
  [TAG_KIND_PARAM]: [],
  [VERSIONS_PARAM]: [],
};
