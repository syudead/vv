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

export function resumePosition(video: Video): number {
  const progress = video.progress;
  return progress === undefined || progress.completed || progress.positionMs < minResumeMs
    ? 0
    : progress.positionMs;
}
