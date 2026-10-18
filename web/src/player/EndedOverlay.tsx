import { Play, RotateCcw } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import type { VideoLinkState } from "./pageDecisions";
import { VideoThumbnail, videoLinkLabel } from "./RelatedVideos";
import { Dimmed } from "./StatusOverlays";

/** Show replay and optional next-video actions without autoplay. Take focus only from inside the player. */
export default function EndedOverlay({
  next,
  to,
  linkState,
  takeFocus,
  onReplay,
  onPlayNext,
}: {
  next: Video | undefined;
  /** Includes playlist context when present. */
  to?: string;
  linkState: VideoLinkState;
  takeFocus: boolean;
  onReplay: () => void;
  onPlayNext: () => void;
}) {
  const primary = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (takeFocus) primary.current?.focus();
  }, [takeFocus]);

  return (
    <Dimmed>
      <span role="status" className="sr-only">
        {t.player.ended.announcement}
      </span>
      {next === undefined ? (
        <div className="flex flex-col items-center gap-3 rounded-lg bg-navbar p-5 text-center shadow-elevated">
          <h2 className="text-lg font-semibold">{t.player.ended.announcement}</h2>
          <Button ref={primary} variant="secondary" onClick={onReplay}>
            <RotateCcw aria-hidden="true" />
            {t.player.ended.replay}
          </Button>
        </div>
      ) : (
        <div className="pointer-events-auto flex w-full max-w-lg flex-col gap-3 rounded-lg bg-navbar p-5 shadow-elevated">
          <h2 className="text-lg font-semibold">{t.player.ended.announcement}</h2>
          <span className="text-xs font-semibold text-primary">
            {t.player.ended.next}
          </span>
          <Link
            to={to ?? `/videos/${String(next.id)}`}
            state={linkState}
            aria-label={videoLinkLabel(next)}
            className="flex items-start gap-3 rounded-md"
          >
            <VideoThumbnail video={next} className="hidden w-card-0 shrink-0 sm:block" />
            <span className="line-clamp-2 min-w-0 text-base font-semibold wrap-anywhere">
              {next.title}
            </span>
          </Link>
          <div className="flex flex-wrap gap-2">
            <Button ref={primary} onClick={onPlayNext}>
              <Play aria-hidden="true" />
              {t.player.ended.playNext}
            </Button>
            <Button variant="secondary" onClick={onReplay}>
              <RotateCcw aria-hidden="true" />
              {t.player.ended.replay}
            </Button>
          </div>
        </div>
      )}
    </Dimmed>
  );
}
