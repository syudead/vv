import type { Video } from "../api/client";

/** minResumeMs 未満の位置は「見始めたばかり」として先頭から再生する。 */
const minResumeMs = 5000;

/** backTarget は遷移元の一覧 URL。無ければ `/`。外部 URL は受け付けない。 */
export function backTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !from.startsWith("/") || from.startsWith("//")) {
    return "/";
  }
  return from;
}

/** autoplayRequested は「次を再生」から来たか（移った先で再生を始めるか）を返す。 */
export function autoplayRequested(state: unknown): boolean {
  return (state as { autoplay?: unknown } | null)?.autoplay === true;
}

/**
 * resumePosition は再生を始める位置である。集まり（同じ動画の別バージョン）の再生位置は
 * バージョンの間で共有するので、このバージョンの尺以上なら先頭から再生する
 * （specs/030-video-versions/research.md R-11）。
 */
export function resumePosition(video: Video): number {
  const progress = video.progress;
  if (progress === undefined || progress.completed || progress.positionMs < minResumeMs) {
    return 0;
  }
  if (video.durationMs !== undefined && progress.positionMs >= video.durationMs) return 0;
  return progress.positionMs;
}

/** maxTrail は、見てきた動画の id をいくつまで覚えるかである。 */
const maxTrail = 100;

/**
 * videoTrail は、この動画詳細画面へ来るまでに見てきた動画の id を古い順に返す（issue 931）。
 * 最後の 1 つが ‹ で戻る「直前に見ていた動画」である。動画詳細画面から別の動画へ移った
 * ときだけ積まれるので、URL を直接開いたときや一覧から来たときは空になる。形の正しくない
 * 値は捨てる。
 */
export function videoTrail(state: unknown): number[] {
  const trail: unknown = (state as { trail?: unknown } | null)?.trail;
  if (!Array.isArray(trail)) return [];
  return trail.filter(
    (id): id is number => typeof id === "number" && Number.isSafeInteger(id) && id > 0,
  );
}

/** VideoLinkState は、動画詳細画面から別の動画へ移るときに載せる状態である。 */
export interface VideoLinkState {
  /** 閉じる × の戻り先（backTarget）。最初の戻り先を引き継ぐ。 */
  from: string;
  /** 移った先から見た、見てきた動画（videoTrail）。 */
  trail: number[];
  autoplay?: boolean;
}

/**
 * linkStateFrom は、動画 currentId の画面から別の動画へ移るときの状態を返す。見てきた動画の
 * 後ろに今の動画を足す。
 */
export function linkStateFrom(
  backTo: string,
  trail: readonly number[],
  currentId: number,
): VideoLinkState {
  return { from: backTo, trail: [...trail, currentId].slice(-maxTrail) };
}
