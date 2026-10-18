import { clearListSnapshot } from "./listSnapshot";

/** Smart-folder card values stay server-owned; listeners re-read the named cards. */
type Listener = (smartFolderIds: readonly number[]) => void;

const listeners = new Set<Listener>();

/**
 * Bulk callers announce smart folders whose card values a favorite, tag or visibility change
 * may have altered, since the server emits no event for favorites.
 */
export function noteSmartFolderChanges(smartFolderIds: readonly number[]): void {
  if (smartFolderIds.length === 0) return;
  const ids = Array.from(new Set(smartFolderIds));
  for (const listener of listeners) listener(ids);
}

/** Re-read smart-folder cards changed by a bulk action. */
export function subscribeSmartFolderChanges(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Options for bulk video actions that also target smart folders. */
export interface VideoTargetOptions {
  /** The server adds each folder's current members; the client never expands them. */
  smartFolderIds?: readonly number[];
  signal?: AbortSignal;
}

/** Omit smartFolderIds from a request body when empty so callers without folders keep the minimal body. */
export function smartFolderField(smartFolderIds: readonly number[]): {
  smartFolderIds?: number[];
} {
  return smartFolderIds.length === 0
    ? {}
    : { smartFolderIds: Array.from(smartFolderIds) };
}

/** Members of the folders changed without being known locally: drop list snapshots and re-read the shown cards. */
export function afterSmartFolderMembersChanged(smartFolderIds: readonly number[]): void {
  if (smartFolderIds.length === 0) return;
  clearListSnapshot();
  noteSmartFolderChanges(smartFolderIds);
}
