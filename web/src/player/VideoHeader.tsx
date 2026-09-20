import { Check, Clock, HardDrive, History, MonitorPlay } from "lucide-react";

import type { Video } from "../api/client";
import {
  formatBytes,
  formatDuration,
  formatRelative,
  qualityLabel,
  watchState,
} from "../lib/format";
import Chip from "../ui/Chip";

/** VideoHeader は題名と、ひと目で分かる要点をチップで並べる。 */
export default function VideoHeader({
  video,
  resumedFrom,
}: {
  video: Video;
  /** 中断位置から再開したときの位置（m:ss）。再開していなければ null。 */
  resumedFrom: string | null;
}) {
  const duration = formatDuration(video.durationMs);
  const quality = qualityLabel(video);
  const state = watchState(video);
  const codec = [video.videoCodec, video.audioCodec].filter(Boolean).join(" / ");

  return (
    <div className="flex flex-col gap-3 animate-fade-in">
      <h1 className="text-lg leading-snug font-semibold tracking-tight text-fg break-all sm:text-xl">
        {video.title}
      </h1>

      <div className="flex flex-wrap items-center gap-1.5">
        {quality !== "" && (
          <Chip>
            <MonitorPlay />
            {quality}
          </Chip>
        )}
        {codec !== "" && <Chip>{codec.toUpperCase()}</Chip>}
        {duration !== "" && (
          <Chip>
            <Clock />
            {duration}
          </Chip>
        )}
        <Chip>
          <HardDrive />
          {formatBytes(video.sizeBytes)}
        </Chip>
        <span className="mx-1 text-xs text-fg-subtle">
          追加 {formatRelative(video.addedAt)}
        </span>

        {state === "watched" && (
          <Chip tone="success" className="ml-auto">
            <Check strokeWidth={3} />
            視聴済み
          </Chip>
        )}
        {resumedFrom !== null && (
          <Chip tone="accent" className="ml-auto">
            <History />
            {resumedFrom} から再開
          </Chip>
        )}
      </div>
    </div>
  );
}
