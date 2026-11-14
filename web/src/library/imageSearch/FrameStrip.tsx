import { type KeyboardEvent, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { type ImageSearchMatch, matchFramePath } from "../../api/imageSearch";
import { t } from "../../i18n";
import { formatDuration } from "../../lib/format";
import type { ReturnState } from "../../player/pageDecisions";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { playPath, ratePercent } from "./imageSearchState";

export interface FrameStripProps {
  match: ImageSearchMatch;
  /** The state the player goes back with. */
  back: ReturnState;
  /** Called with the frame's index before it opens the player, to keep the view. */
  onOpen: (index: number) => void;
  /** The frame last focused here when the strip came back from history. */
  initialActive?: number;
  onActiveChange?: (index: number) => void;
  /** Names each frame link for focus on the way back from the player. */
  opener: (index: number) => string;
}

/** The frame a strip key moves to from `index` of `count`, or null for a key the strip does not take. */
export function stripTarget(key: string, index: number, count: number): number | null {
  switch (key) {
    case "ArrowLeft":
      return Math.max(0, index - 1);
    case "ArrowRight":
      return Math.min(count - 1, index + 1);
    case "Home":
      return 0;
    case "End":
      return count - 1;
    default:
      return null;
  }
}

/**
 * Scrolls the strip sideways, and only the strip, so the link and its focus ring lie inside its padding box.
 * `scrollIntoView` would also move the page, whose position the way back from the player restores.
 */
function revealInStrip(strip: HTMLElement, link: HTMLElement) {
  const style = getComputedStyle(strip);
  const view = strip.getBoundingClientRect();
  const box = link.getBoundingClientRect();
  const left = view.left + strip.clientLeft + Number.parseFloat(style.paddingLeft);
  const right =
    view.left +
    strip.clientLeft +
    strip.clientWidth -
    Number.parseFloat(style.paddingRight);
  if (box.left < left) strip.scrollLeft -= left - box.left;
  else if (box.right > right) strip.scrollLeft += box.right - right;
}

/**
 * A row's matched frames in one sideways-scrolling line, each a link that plays from its time. The strip
 * is one tab stop, on the frame last focused there; the arrow keys, Home and End move along it.
 */
export function FrameStrip({
  match,
  back,
  onOpen,
  initialActive = 0,
  onActiveChange,
  opener,
}: FrameStripProps) {
  const [active, setActive] = useState(initialActive);
  const list = useRef<HTMLUListElement>(null);
  const links = useRef<(HTMLAnchorElement | null)[]>([]);
  const text = t.library.imageSearch;
  const { frames } = match;
  const current = Math.min(active, frames.length - 1);

  // A strip back from history starts scrolled to its start; bring its restored frame into view.
  const restoredFrame = useRef(current);
  useLayoutEffect(() => {
    const link = links.current[restoredFrame.current] ?? null;
    if (list.current !== null && link !== null) revealInStrip(list.current, link);
  }, []);

  const onKeyDown = (event: KeyboardEvent<HTMLUListElement>) => {
    const next = stripTarget(event.key, current, frames.length);
    if (next === null) return;
    event.preventDefault();
    const link = links.current[next];
    link?.focus();
    link?.scrollIntoView({ block: "nearest", inline: "nearest" });
  };

  return (
    <ul
      ref={list}
      data-slot="frame-strip"
      aria-label={text.framesLabel}
      onKeyDown={onKeyDown}
      // The padding keeps the focus ring of the first and last frame inside the scroll box.
      className="-m-1 flex min-w-0 gap-3 overflow-x-auto p-1"
    >
      {frames.map((frame, index) => {
        const time = formatDuration(frame.positionMs);
        return (
          <li key={frame.positionMs} className="flex w-related-thumb shrink-0">
            <Link
              ref={(node) => {
                links.current[index] = node;
              }}
              to={playPath(match.videoId, frame.positionMs)}
              state={back}
              onClick={() => onOpen(index)}
              onFocus={() => {
                if (index === active) return;
                setActive(index);
                onActiveChange?.(index);
              }}
              data-opener={opener(index)}
              tabIndex={index === current ? 0 : -1}
              aria-label={text.playFrom(time)}
              className="flex w-full min-w-0 flex-col gap-1 rounded-md"
            >
              <VideoThumbnail>
                <VideoThumbnailImage src={matchFramePath(match, frame)} />
              </VideoThumbnail>
              <span className="flex min-w-0 items-center gap-2 text-xs">
                <span className="font-medium text-foreground tabular-nums">
                  {text.percent(ratePercent(frame.rate))}
                </span>
                <span className="ml-auto text-muted-foreground tabular-nums">{time}</span>
              </span>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
