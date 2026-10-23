import { FileVideo } from "lucide-react";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { cn } from "../lib/cn";

const titleType = "text-xl leading-snug font-semibold sm:text-video-title";

/**
 * The title, and for the owner of a renamed video the file name under it. The name is edited in
 * the details editor, so the title has no edit control of its own.
 */
export default function VideoTitle({ video, owner }: { video: Video; owner: boolean }) {
  const heading = (
    <h1 className={cn(titleType, "min-w-0 wrap-anywhere")}>{video.title}</h1>
  );
  if (!owner || video.displayName === undefined || video.fileTitle === undefined) {
    return heading;
  }
  return (
    <div className="flex flex-col gap-1">
      {heading}
      <p
        title={t.player.title.fileNameTitle(video.fileTitle)}
        className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground sm:text-sm"
      >
        <FileVideo className="size-4 shrink-0" aria-hidden="true" />
        <span className="sr-only">{t.player.title.fileName} </span>
        <span className="truncate">{video.fileTitle}</span>
      </p>
    </div>
  );
}
