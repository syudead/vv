import { Link, useNavigate } from "react-router";

import { type ImageSearchItem, matchFramePath } from "../../api/imageSearch";
import { t } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { Badge } from "../../ui/shadcn/badge";
import { SpanBar } from "../../ui/SpanBar";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
} from "../../ui/VideoThumbnail";
import { cardLinkState } from "../../videoList/cardLink";
import { matchPath, playPath, ratePercent } from "./imageSearchState";

export interface ResultCardProps {
  item: ImageSearchItem;
  /** The library URL the player goes back to. */
  backTo: string;
  /** Called before a result opens the player, to keep the scroll position. */
  onOpen: () => void;
}

/** Where in the video the match is: the frame's time, or the matched section. */
function whereText(item: ImageSearchItem): string {
  const { match } = item;
  if (match.startMs === undefined || match.endMs === undefined) {
    return t.library.imageSearch.at(formatDuration(match.positionMs));
  }
  return t.library.imageSearch.range(
    formatDuration(match.startMs),
    formatDuration(match.endMs),
  );
}

/**
 * One result: a link to the player at the match, and for a video query the matched section on the
 * video's length. It has none of a library card's selection, favorite, tags or preview.
 */
export function ResultCard({ item, backTo, onOpen }: ResultCardProps) {
  const navigate = useNavigate();
  const { video, match } = item;
  const text = t.library.imageSearch;
  const duration = formatDuration(video.durationMs);
  const section =
    match.startMs !== undefined && match.endMs !== undefined
      ? { start: match.startMs / 1000, end: match.endMs / 1000 }
      : null;
  return (
    <article data-video-id={video.id} className="flex min-w-0 flex-col gap-2">
      <Link
        to={matchPath(match)}
        state={cardLinkState(backTo, undefined)}
        onClick={onOpen}
        className="flex min-w-0 flex-col gap-2 rounded-md"
      >
        <VideoThumbnail>
          <VideoThumbnailImage src={matchFramePath(match)} />
          {duration !== "" && <VideoThumbnailDuration>{duration}</VideoThumbnailDuration>}
        </VideoThumbnail>
        <div className="flex min-w-0 flex-col gap-1">
          <h3
            title={video.title}
            className="line-clamp-2 text-sm font-medium break-all text-foreground"
          >
            {video.title}
          </h3>
          <p className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1 text-xs">
            <span className="font-medium text-foreground">
              {text.rate(ratePercent(match.rate))}
            </span>
            <span aria-hidden="true" className="text-muted-foreground">
              ·
            </span>
            <span className="text-muted-foreground tabular-nums">{whereText(item)}</span>
            {match.kind !== "frame" && (
              <Badge variant="secondary">
                {match.kind === "whole" ? text.whole : text.part}
              </Badge>
            )}
          </p>
        </div>
      </Link>
      {section !== null && video.durationMs !== undefined && (
        <SpanBar
          lengthSeconds={video.durationMs / 1000}
          spans={[section]}
          label={text.spanLane}
          spanLabel={(span) => text.playFrom(formatDuration(span.start * 1000))}
          onPress={(span) => {
            onOpen();
            void navigate(playPath(match.videoId, span.start * 1000), {
              state: cardLinkState(backTo, undefined),
            });
          }}
        />
      )}
    </article>
  );
}
