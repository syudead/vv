import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// Owner-only operations are atomic and return a single-use undo token valid for ten minutes.

export type OrganizeResult = components["schemas"]["OrganizeResult"];
export type OrganizeRef = components["schemas"]["OrganizeRef"];

const jsonHeaders = { "Content-Type": "application/json" };

async function postOperation(
  path: string,
  body: unknown,
  signal: AbortSignal | undefined,
): Promise<OrganizeResult> {
  const result = await request<OrganizeResult>(path, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
  clearListSnapshot();
  return result;
}

/** Move items in display order after after, or to the front for null; missing targets fail atomically. */
export async function reorderGroup(
  groupId: number,
  itemIds: readonly number[],
  after: number | null,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = {
    groupId,
    itemIds: Array.from(itemIds),
    ...(after === null ? {} : { after }),
  };
  return postOperation("/api/organize/reorder", body, signal);
}

/** Create target-first groups, replacing the target's position inside from when supplied; missing targets fail atomically. */
export async function groupVideos(
  videoIds: readonly number[],
  target: number,
  from: number | null,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = {
    items: videoIds.map((id) => ({ kind: "video" as const, id })),
    target,
    ...(from === null ? {} : { from }),
  };
  return postOperation("/api/organize/group", body, signal);
}

export interface MoveTarget {
  from: number | null;
  to: number | null;
}

/** Move cards in display order; null to removes only. Invalid destinations, missing targets, and cycles fail atomically. */
export async function moveCards(
  items: readonly OrganizeRef[],
  { from, to }: MoveTarget,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = {
    items: items.map(({ kind, id }) => ({ kind, id })),
    ...(from === null ? {} : { from }),
    ...(to === null ? {} : { to }),
  };
  return postOperation("/api/organize/move", body, signal);
}

/** Merge contents and inherited references atomically under one undo token, removing source cards from from when supplied. */
export async function mergeGroups(
  items: readonly OrganizeRef[],
  into: number,
  from: number | null,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  const body = { items: Array.from(items), into, ...(from === null ? {} : { from }) };
  return postOperation("/api/organize/merge", body, signal);
}

/** Bundle onto target as representative, expanding existing bundles; validate the expanded selection atomically. */
export async function bundleOnto(
  videoIds: readonly number[],
  target: number,
  signal?: AbortSignal,
): Promise<OrganizeResult> {
  return postOperation(
    "/api/organize/bundle",
    { videoIds: Array.from(videoIds), target },
    signal,
  );
}

/** Undo fails atomically on changed rows or missing, used, or expired tokens. */
export async function undoOrganize(token: string, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch("/api/organize/undo", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ token }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  clearListSnapshot();
}
