import { request } from "./client";
import type { components } from "./gen/openapi";
import { applyVisibilityToListSnapshot, holdListSnapshot } from "./listSnapshot";
import {
  afterSmartFolderMembersChanged,
  type VideoTargetOptions,
  smartFolderField,
} from "./smartFolderChanges";

//
export type VideoVisibilityResponse = components["schemas"]["VideoVisibilityResponse"];

type Listener = (videoIds: readonly number[], isPublic: boolean) => void;

const listeners = new Set<Listener>();

/** Partial application cannot identify changed videos; request authoritative refreshes instead of applying optimistic values. */
type StaleListener = (videoIds: readonly number[]) => Promise<void> | undefined;

const staleListeners = new Set<StaleListener>();

let sent = 0;

/** Track applied sequences defensively so stale responses cannot roll back later toggles. */
const applied = new Map<number, number>();

/** Overlay mutations onto older GETs; retain one latest result per video, bounded by library size. */
let changes = 0;
const latest = new Map<number, { isPublic: boolean; change: number }>();

/** Serialize requests so server arrival order matches the user's final toggle. */
let tail: Promise<void> | undefined;

function recordApplied(
  videoIds: readonly number[],
  isPublic: boolean,
  sequence: number,
): void {
  const fresh = videoIds.filter((videoId) => {
    if (sequence <= (applied.get(videoId) ?? 0)) return false;
    applied.set(videoId, sequence);
    return true;
  });
  if (fresh.length === 0) return;
  changes += 1;
  for (const videoId of fresh) latest.set(videoId, { isPublic, change: changes });
  applyVisibilityToListSnapshot(fresh, isPublic);
  for (const listener of listeners) listener(fresh, isPublic);
}

/** Discard uncertain overlays and snapshots; block new snapshots until subscriber refreshes settle. */
function recordUncertain(videoIds: readonly number[], sequence: number): void {
  const fresh = videoIds.filter((videoId) => {
    if (sequence <= (applied.get(videoId) ?? 0)) return false;
    applied.set(videoId, sequence);
    latest.delete(videoId);
    return true;
  });
  if (fresh.length === 0) return;
  const release = holdListSnapshot();
  for (const listener of staleListeners) {
    const refetch = listener(fresh);
    if (refetch === undefined) continue;
    const releaseOne = holdListSnapshot();
    void refetch.then(releaseOne, releaseOne);
  }
  release();
}

/** List subscribers return a promise settling after refresh or unmount; snapshots remain blocked until then. */
export function subscribeVideoVisibilityStale(listener: StaleListener): () => void {
  staleListeners.add(listener);
  return () => {
    staleListeners.delete(listener);
  };
}

export function subscribeVideoVisibility(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Capture before fetching and pass to withVisibilitySince when the response arrives. */
export function visibilityMark(): number {
  return changes;
}

/** Overlay only mutations newer than the read's mark. */
export function withVisibilitySince<T extends { id: number; public: boolean }>(
  video: T,
  mark: number,
): T {
  const change = latest.get(video.id);
  if (change === undefined || change.change <= mark) return video;
  if (video.public === change.isPublic) return video;
  return { ...video, public: change.isPublic };
}

/** Serialize toggles and publish confirmed results; partial application requests refresh because affected IDs are unknown. */
export function updateVideoVisibility(
  videoIds: readonly number[],
  isPublic: boolean,
  { smartFolderIds = [], signal }: VideoTargetOptions = {},
): Promise<VideoVisibilityResponse> {
  sent += 1;
  const sequence = sent;
  const send = async () => {
    const result = await request<VideoVisibilityResponse>("/api/video-visibility", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        videoIds: Array.from(videoIds),
        ...smartFolderField(smartFolderIds),
        public: isPublic,
      }),
      signal,
    });
    const unique = Array.from(new Set(videoIds));
    // Applied also counts smart-folder members, so it cannot prove every explicit video changed.
    if (smartFolderIds.length > 0 || result.applied < unique.length) {
      recordUncertain(unique, sequence);
    } else {
      recordApplied(videoIds, isPublic, sequence);
    }
    afterSmartFolderMembersChanged(smartFolderIds);
    return result;
  };
  // Send immediately when idle so pending state begins with the click.
  const run = tail === undefined ? send() : tail.then(send);
  const settled = run.then(
    () => undefined,
    () => undefined,
  );
  tail = settled;
  void settled.then(() => {
    if (tail === settled) tail = undefined;
  });
  return run;
}
