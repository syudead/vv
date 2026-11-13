import { Play, Target } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router";

import type { ImageSearchItem } from "../../api/imageSearch";
import { t } from "../../i18n";
import { formatDuration } from "../../lib/format";
import type { ReturnState } from "../../player/pageDecisions";
import { ResultRow } from "../../ui/patterns/result-row";
import { RateRing, type RateRingTone } from "../../ui/RateRing";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { SpanBar } from "../../ui/SpanBar";
import { FrameStrip } from "./FrameStrip";
import { matchedSection, matchPath, playPath, ratePercent } from "./imageSearchState";

export interface MatchRowProps {
  item: ImageSearchItem;
  tone: RateRingTone;
  /** The first row of the results, shown above every other: the badge and the `default` Play. */
  best: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  back: ReturnState;
  /** Called before the row opens the player, to keep the scroll position. */
  onOpen: () => void;
}

/** One video of an image search's answer: its rate, title, evidence and the way to play it. */
export function MatchRow({
  item,
  tone,
  best,
  expanded,
  onExpandedChange,
  back,
  onOpen,
}: MatchRowProps) {
  const text = t.library.imageSearch;
  const { video, match } = item;
  const to = matchPath(match);
  const section = match.kind !== "frame";
  return (
    <ResultRow
      mark={<RateRing value={ratePercent(match.rate)} tone={tone} />}
      title={
        <Link
          to={to}
          state={back}
          onClick={onOpen}
          title={video.title}
          className="block truncate rounded-sm hover:underline"
        >
          {video.title}
        </Link>
      }
      badge={
        best ? (
          <Badge variant="soft">
            <Target aria-hidden="true" />
            {text.best}
          </Badge>
        ) : undefined
      }
      meta={section ? <SectionMeta item={item} /> : <FrameMeta item={item} />}
      actions={
        <Button asChild variant={best ? "default" : "secondary"} size="sm">
          <Link to={to} state={back} onClick={onOpen}>
            <Play aria-hidden="true" />
            {text.play}
          </Link>
        </Button>
      }
      expanded={expanded}
      onExpandedChange={onExpandedChange}
      toggleLabel={
        expanded
          ? { tooltip: text.hideFrames, name: text.hideFramesFor(video.title) }
          : { tooltip: text.showFrames, name: text.showFramesFor(video.title) }
      }
    >
      <RowBody item={item} back={back} onOpen={onOpen} />
    </ResultRow>
  );
}

// Below sm the named Play leaves the text column too narrow for one line, so the meta wraps instead of
// hiding the best time.
function FrameMeta({ item }: { item: ImageSearchItem }) {
  const { match } = item;
  return (
    <span className="min-w-0 sm:truncate">
      {t.library.imageSearch.frameMeta(
        match.frameCount,
        formatDuration(match.positionMs),
      )}
    </span>
  );
}

function SectionMeta({ item }: { item: ImageSearchItem }) {
  const text = t.library.imageSearch;
  const { match } = item;
  const range =
    match.startMs === undefined || match.endMs === undefined
      ? undefined
      : text.range(formatDuration(match.startMs), formatDuration(match.endMs));
  return (
    <>
      <Badge variant="secondary">{match.kind === "whole" ? text.whole : text.part}</Badge>
      <span className="min-w-0 tabular-nums sm:truncate">
        {range}
        {match.frameCount > 0 && ` · ${text.sectionFrames(match.frameCount)}`}
      </span>
    </>
  );
}

function RowBody({
  item,
  back,
  onOpen,
}: {
  item: ImageSearchItem;
  back: ReturnState;
  onOpen: () => void;
}): ReactNode {
  const { match } = item;
  const strip =
    match.frames.length > 0 ? (
      <FrameStrip match={match} back={back} onOpen={onOpen} />
    ) : null;
  if (match.kind === "frame") return strip;
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <SectionLane item={item} back={back} onOpen={onOpen} />
      {strip}
    </div>
  );
}

function SectionLane({
  item,
  back,
  onOpen,
}: {
  item: ImageSearchItem;
  back: ReturnState;
  onOpen: () => void;
}) {
  const navigate = useNavigate();
  const text = t.library.imageSearch;
  const { match } = item;
  const section = matchedSection(match, item.video.durationMs);
  if (section === null) return null;
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <SpanBar
        lengthSeconds={section.lengthSeconds}
        spans={[section.span]}
        label={text.spanLane}
        spanLabel={(span) => text.playFrom(formatDuration(span.start * 1000))}
        onPress={(span) => {
          onOpen();
          void navigate(playPath(match.videoId, span.start * 1000), { state: back });
        }}
      />
      <div className="flex justify-between text-2xs text-muted-foreground tabular-nums">
        <span>{formatDuration(0)}</span>
        <span>{formatDuration(section.lengthSeconds * 1000)}</span>
      </div>
    </div>
  );
}
