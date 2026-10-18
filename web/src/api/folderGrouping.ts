import { t, type UiText } from "../i18n";
import { type FolderRef, RequestFailed, request } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

export type FolderGrouping = components["schemas"]["FolderGrouping"];
export type FolderGroupingMode = components["schemas"]["FolderGroupingMode"];

function groupingPath(folder: FolderRef, suffix: string): string {
  const query = new URLSearchParams();
  if (folder.path !== "") query.set("path", folder.path);
  const search = query.size === 0 ? "" : `?${query.toString()}`;
  return `/api/folders/${String(folder.rootId)}/grouping${suffix}${search}`;
}

/** Invalidate snapshots on success and 409 because another tab's grouping change also makes cached cards stale. */
async function sendGrouping<T>(path: string, init: RequestInit): Promise<T> {
  try {
    const result = await request<T>(path, init);
    clearListSnapshot();
    return result;
  } catch (failure) {
    if (failure instanceof RequestFailed && failure.status === 409) clearListSnapshot();
    throw failure;
  }
}

export async function setFolderGrouping(
  folder: FolderRef,
  mode: FolderGroupingMode,
  signal?: AbortSignal,
): Promise<FolderGrouping> {
  return sendGrouping<FolderGrouping>(groupingPath(folder, ""), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode }),
    signal,
  });
}

/** A 409 requires refreshing current state; notFoundMessage supplies the 404 wording. */
export function groupingFailure(
  failure: unknown,
  {
    name,
    tagging,
    notFoundMessage,
  }: {
    name: string;
    tagging: boolean;
    notFoundMessage: UiText;
  },
): { message: UiText; conflict: boolean } {
  if (failure instanceof RequestFailed) {
    if (tagging && failure.status === 400 && failure.code === "invalid_request") {
      return { message: t.folderGrouping.cannotTag(name), conflict: false };
    }
    if (failure.status === 409) {
      return { message: t.folderGrouping.notGroup, conflict: true };
    }
    if (failure.status === 404) return { message: notFoundMessage, conflict: false };
  }
  return { message: t.folderGrouping.changeFailed, conflict: false };
}
