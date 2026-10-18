import type { Progress } from "./client";
import { applyProgressToListSnapshot } from "./listSnapshot";

type Listener = (videoId: number, progress: Progress) => void;

const listeners = new Set<Listener>();

let sent = 0;
const applied = new Map<number, number>();

/** Assign before sending; compare sequences because responses may arrive out of order. */
export function nextProgressSequence(): number {
  sent += 1;
  return sent;
}

/** Publish server-confirmed progress to snapshots and mounted lists; exit saves may finish after the list remounts. */
export function recordSavedProgress(
  videoId: number,
  progress: Progress,
  sequence: number,
): void {
  // Ignore an older response when a later save already applied.
  if (sequence <= (applied.get(videoId) ?? 0)) return;
  applied.set(videoId, sequence);
  applyProgressToListSnapshot(videoId, progress);
  for (const listener of listeners) listener(videoId, progress);
}

export function subscribeProgress(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
