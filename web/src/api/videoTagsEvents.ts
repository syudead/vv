import type { TagRef } from "./client";
import { applyTagToListSnapshot } from "./listSnapshot";

type Listener = (
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
) => void;

const listeners = new Set<Listener>();

let sent = 0;

/** Track the latest sequence per video/tag pair so out-of-order responses cannot undo newer edits. */
const applied = new Map<string, number>();

function key(videoId: number, tagId: number): string {
  return `${String(videoId)}\0${String(tagId)}`;
}

/** Allocate immediately before sending; response order may differ. */
export function nextVideoTagsSequence(): number {
  sent += 1;
  return sent;
}

/** Notify snapshots and live lists for requested IDs, excluding pairs with newer applied sequences; missing videos are absent from both consumers. */
export function recordAppliedVideoTags(
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
  sequence: number,
): void {
  const fresh = videoIds.filter((videoId) => {
    const k = key(videoId, tag.id);
    if (sequence <= (applied.get(k) ?? 0)) return false;
    applied.set(k, sequence);
    return true;
  });
  if (fresh.length === 0) return;
  applyTagToListSnapshot(fresh, tag, action);
  for (const listener of listeners) listener(fresh, tag, action);
}

export function subscribeVideoTags(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
