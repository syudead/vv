import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";
import { formatDuration } from "../lib/format";

// 視聴履歴の行の再生位置と操作（specs/043-watch-history/ui-design.md「Entry row」、
// research.md R-13）。

/** EntryPosition は行の位置の行: バーの値と最大、「16:05 / 42:18」の文字である。 */
export interface EntryPosition {
  value: number;
  max: number;
  text: UiText;
}

/**
 * entryPosition は動画の今の再生位置の行である。位置か長さが無ければ null で、行を出さない。
 * 見終わった動画はサーバーが終わりの少し前から見終わりにするので、バーは満たし、文字は
 * 保存された位置のままにする。
 */
export function entryPosition(
  video: Pick<Video, "progress" | "durationMs">,
): EntryPosition | null {
  const progress = video.progress;
  const duration = video.durationMs;
  if (progress === undefined || duration === undefined || duration <= 0) return null;
  return {
    value: progress.completed ? duration : Math.min(progress.positionMs, duration),
    max: duration,
    text: t.history.position(
      formatDuration(progress.positionMs),
      formatDuration(duration),
    ),
  };
}

/** EntryAction は行の端の再生の操作である。 */
export type EntryAction = "resume" | "startOver";

/**
 * entryAction は途中の動画に「Resume」、見終わった動画に「Start over」を返す。位置の無い
 * 動画は行そのものが動画を開くので、操作を置かない（null）。
 */
export function entryAction(video: Pick<Video, "progress">): EntryAction | null {
  if (video.progress === undefined) return null;
  return video.progress.completed ? "startOver" : "resume";
}

/** folderLine は動画のフォルダの行（「Travel / 2024」）である。登録フォルダの直下なら null。 */
export function folderLine(video: Pick<Video, "folder">): string | null {
  const path = video.folder?.path ?? "";
  if (path === "") return null;
  return path.split("/").join(" / ");
}
