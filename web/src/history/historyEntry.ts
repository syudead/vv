import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";
import { formatDuration } from "../lib/format";

export interface EntryPosition {
  value: number;
  max: number;
  text: UiText;
}

/**
 * Completed videos show a full bar even when the saved position precedes the end; keep the actual
 * position text.
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

export type EntryAction = "resume" | "startOver";

/** Without a saved position, omit the action and let the row open playback. */
export function entryAction(video: Pick<Video, "progress">): EntryAction | null {
  if (video.progress === undefined) return null;
  return video.progress.completed ? "startOver" : "resume";
}

export function folderLine(video: Pick<Video, "folder">): string | null {
  const path = video.folder?.path ?? "";
  if (path === "") return null;
  return path.split("/").join(" / ");
}
