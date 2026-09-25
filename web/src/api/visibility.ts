import { request } from "./client";
import type { components } from "./gen/openapi";
import { applyVisibilityToListSnapshot } from "./listSnapshot";

// 型は api/openapi.yaml からの生成物を使う
// （specs/016-single-account-auth/contracts/guest-api.md §4）。
export type VideoVisibilityResponse = components["schemas"]["VideoVisibilityResponse"];

/**
 * 公開・非公開の切り替えの結果を、タグの付け外し（videoTagsEvents）と同じ形で
 * 画面へ知らせる。読み込み済みの一覧の項目・listSnapshot の控え・再生画面の
 * 1件の `public` を、取り直さずに差し替える（issue 305「切り替えの後の一覧」）。
 */
type Listener = (videoIds: readonly number[], isPublic: boolean) => void;

const listeners = new Set<Listener>();

/** sent は要求を送る直前に払い出す通し番号で、送った順に増える。 */
let sent = 0;

/**
 * applied は動画ごとに、反映済みの要求の通し番号を持つ。同じ動画への切り替えが
 * 2つ飛び、応答が送った順と違う順で届いても、後から送った切り替えの結果を
 * 先に届いた古い応答で巻き戻さない（videoTagsEvents と同じ仕組み）。
 */
const applied = new Map<number, number>();

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
  applyVisibilityToListSnapshot(fresh, isPublic);
  for (const listener of listeners) listener(fresh, isPublic);
}

/** subscribeVideoVisibility は切り替えの結果を受け取る。戻り値で購読をやめる。 */
export function subscribeVideoVisibility(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * updateVideoVisibility は `videoIds` の公開フラグを `isPublic` にそろえる
 * （PUT /api/video-visibility、contracts/guest-api.md §4）。再生画面の1本も、
 * 選択バーの複数本も、これを使う。成功したら、受け付けた結果を画面へ知らせる。
 */
export async function updateVideoVisibility(
  videoIds: readonly number[],
  isPublic: boolean,
  signal?: AbortSignal,
): Promise<VideoVisibilityResponse> {
  sent += 1;
  const sequence = sent;
  const result = await request<VideoVisibilityResponse>("/api/video-visibility", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoIds: Array.from(videoIds), public: isPublic }),
    signal,
  });
  recordApplied(videoIds, isPublic, sequence);
  return result;
}
