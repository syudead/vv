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
 * applied は動画ごとに、反映済みの要求の通し番号を持つ。要求は下の tail で
 * 1つずつ順に送るので応答も送った順に届くが、念のため、後から送った切り替えの
 * 結果を古い応答で巻き戻さない（videoTagsEvents と同じ仕組み）。
 */
const applied = new Map<number, number>();

/**
 * changes は反映した切り替えの数で、latest は動画ごとの直近の切り替えの結果と、
 * それが何番目の反映だったかを持つ。切り替えの前に始まった GET の応答は、
 * サーバーが切り替える前の `public` を読んでいることがあるので、
 * visibilityMark と withVisibilitySince で直近の結果へ補正する（Devin の指摘、PR 328）。
 * 動画ごとに1つだけ持つので、大きさは切り替えた動画の数（ライブラリの本数）を超えない。
 */
let changes = 0;
const latest = new Map<number, { isPublic: boolean; change: number }>();

/**
 * tail は最後に送った（または送る順番を待っている）切り替えが決着する Promise で、
 * 何も送っていなければ undefined である。サーバーは届いた順に切り替えを確定するが、
 * 2つを同時に送ると届く順は送った順と限らない。前の要求が決着してから次を送り、
 * 最後に押した切り替えが最後に確定するようにする（Devin の指摘、PR 328）。
 */
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

/** subscribeVideoVisibility は切り替えの結果を受け取る。戻り値で購読をやめる。 */
export function subscribeVideoVisibility(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * visibilityMark は動画を取りに行く直前に呼び、その時点までに反映した切り替えを
 * 表す印を返す。応答を受けたら、この印と一緒に withVisibilitySince へ渡す。
 */
export function visibilityMark(): number {
  return changes;
}

/**
 * withVisibilitySince は `mark` を取った後に反映した切り替えがあれば、取得した
 * 動画の `public` をその結果へ差し替える。取得の方が新しければそのまま返す。
 */
export function withVisibilitySince<T extends { id: number; public: boolean }>(
  video: T,
  mark: number,
): T {
  const change = latest.get(video.id);
  if (change === undefined || change.change <= mark) return video;
  if (video.public === change.isPublic) return video;
  return { ...video, public: change.isPublic };
}

/**
 * updateVideoVisibility は `videoIds` の公開フラグを `isPublic` にそろえる
 * （PUT /api/video-visibility、contracts/guest-api.md §4）。再生画面の1本も、
 * 選択バーの複数本も、これを使う。前の切り替えが決着するまで送るのを待ち、
 * 成功したら、受け付けた結果を画面へ知らせる。
 */
export function updateVideoVisibility(
  videoIds: readonly number[],
  isPublic: boolean,
  signal?: AbortSignal,
): Promise<VideoVisibilityResponse> {
  sent += 1;
  const sequence = sent;
  const send = async () => {
    const result = await request<VideoVisibilityResponse>("/api/video-visibility", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ videoIds: Array.from(videoIds), public: isPublic }),
      signal,
    });
    recordApplied(videoIds, isPublic, sequence);
    return result;
  };
  // 待っている切り替えが無ければすぐに送る（押した直後に送信中になる）。
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
