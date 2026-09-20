import { Check, History } from "lucide-react";

import type { Video } from "../api/client";
import { formatDateTime, formatResolution, watchState } from "../lib/format";
import Chip from "../ui/Chip";

/**
 * VideoHeader は Stash の scene-header / scene-subheader と同じ構成。
 * 大きめの題名、その下に左が日付・右が太字の解像度。
 */
export default function VideoHeader({
  video,
  resumedFrom,
}: {
  video: Video;
  /** 中断位置から再開したときの位置（m:ss）。再開していなければ null。 */
  resumedFrom: string | null;
}) {
  const resolution = formatResolution(video);
  const state = watchState(video);

  return (
    <div className="flex flex-col gap-2 animate-fade-in">
      <h1 className="text-xl leading-snug font-medium text-fg break-all sm:text-2xl">
        {video.title}
      </h1>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <span className="text-sm text-fg-muted tabular-nums">
          {formatDateTime(video.addedAt)}
        </span>
        <span className="flex items-center gap-2">
          {state === "watched" && (
            <Chip tone="success">
              <Check strokeWidth={3} />
              視聴済み
            </Chip>
          )}
          {resumedFrom !== null && (
            <Chip tone="accent">
              <History />
              {resumedFrom} から再開
            </Chip>
          )}
          {resolution !== "" && (
            <span className="text-sm font-bold text-fg">{resolution}</span>
          )}
        </span>
      </div>
    </div>
  );
}
