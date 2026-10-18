import { Play, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { getVideo, isAborted, RequestFailed, type Video } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import type { VideoLinkState } from "./pageDecisions";
import { VideoThumbnail, videoLinkLabel } from "./RelatedVideos";
import { Dimmed } from "./StatusOverlays";

export const autoplayNoticeSeconds = 5;

/** Autoplay countdown, cancellation, or removal of the next member. */
export type AutoplayPhase = "notice" | "cancelled" | "gone";

export function autoplayAnnouncement(title: string): UiText {
  return t.player.autoplay.announcement(title, autoplayNoticeSeconds);
}

/** Use an elapsed-time deadline and verify the next member before advancing. Announce once and take focus only from inside the player. */
export default function AutoplayNotice({
  next,
  to = `/videos/${String(next.id)}`,
  linkState,
  takeFocus,
  watchEvents,
  onCancel,
  onPlayNow,
  onAdvance,
  onGone,
}: {
  next: Video;
  /** Includes playlist context when present. */
  to?: string;
  linkState: VideoLinkState;
  takeFocus: boolean;
  /** Guest sessions cannot subscribe to owner-only video events. */
  watchEvents: boolean;
  onCancel: () => void;
  onPlayNow: () => void;
  onAdvance: () => void;
  onGone: () => void;
}) {
  const cancel = useRef<HTMLButtonElement | null>(null);
  const [remaining, setRemaining] = useState(autoplayNoticeSeconds);
  // Start full, then animate the countdown on the next frame.
  const [started, setStarted] = useState(false);
  const latest = useRef({ onAdvance, onGone });
  useEffect(() => {
    latest.current = { onAdvance, onGone };
  }, [onAdvance, onGone]);

  useEffect(() => {
    if (takeFocus) cancel.current?.focus();
  }, [takeFocus]);

  useEffect(() => {
    const frame = requestAnimationFrame(() => setStarted(true));
    // Compare against the deadline: background tabs throttle timers.
    // Recheck on each tick and when the tab becomes visible.
    const deadline = performance.now() + autoplayNoticeSeconds * 1000;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const update = () => {
      const left = deadline - performance.now();
      const seconds = Math.max(Math.ceil(left / 1000), 0);
      setRemaining(seconds);
      clearTimeout(timer);
      if (seconds > 0) timer = setTimeout(update, left - (seconds - 1) * 1000);
    };
    timer = setTimeout(update, 1000);
    document.addEventListener("visibilitychange", update);
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);

  // Cancel verification when the overlay closes or navigation changes.
  const checks = useRef(new Set<AbortController>());
  useEffect(() => {
    const running = checks.current;
    return () => {
      for (const controller of running) controller.abort();
      running.clear();
    };
  }, []);

  const nextId = next.id;
  useEffect(() => {
    const confirm = async (): Promise<boolean> => {
      const controller = new AbortController();
      checks.current.add(controller);
      try {
        await getVideo(nextId, controller.signal);
        return true;
      } catch (failure) {
        if (isAborted(failure) || controller.signal.aborted) return false;
        if (failure instanceof RequestFailed && failure.status === 404) {
          latest.current.onGone();
          return false;
        }
        // A transient error does not prove removal; the destination reports its state.
        return true;
      } finally {
        checks.current.delete(controller);
      }
    };

    if (remaining > 0) return;
    let active = true;
    void confirm().then((present) => {
      if (active && present) latest.current.onAdvance();
    });
    return () => {
      active = false;
    };
  }, [nextId, remaining]);

  useEffect(() => {
    if (!watchEvents) return;
    const confirmGone = () => {
      const controller = new AbortController();
      checks.current.add(controller);
      getVideo(nextId, controller.signal).then(
        () => checks.current.delete(controller),
        (failure: unknown) => {
          checks.current.delete(controller);
          if (controller.signal.aborted || isAborted(failure)) return;
          if (failure instanceof RequestFailed && failure.status === 404) {
            latest.current.onGone();
          }
        },
      );
    };
    return subscribeServerEvents({
      video: (changed) => {
        if (changed === nextId) confirmGone();
      },
      // Reconnection may have missed events.
      open: (reconnected) => {
        if (reconnected) confirmGone();
      },
    });
  }, [nextId, watchEvents]);

  // Animate toward the next second; reduced motion uses discrete steps.
  const reduceMotion =
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const barValue = reduceMotion || !started ? remaining : Math.max(remaining - 1, 0);

  return (
    <Dimmed>
      <span role="status" className="sr-only">
        {autoplayAnnouncement(next.title)}
      </span>
      <div className="pointer-events-auto flex w-full max-w-lg flex-col gap-3 rounded-lg bg-navbar p-5 shadow-elevated">
        <div className="flex items-baseline justify-between gap-3">
          <span className="text-xs font-semibold text-primary">
            {t.player.autoplay.heading}
          </span>
          <span aria-hidden="true" className="text-xs text-muted-foreground tabular-nums">
            {t.player.autoplay.countdown(remaining)}
          </span>
        </div>
        <Link
          to={to}
          state={linkState}
          aria-label={videoLinkLabel(next)}
          className="flex items-start gap-3 rounded-md"
        >
          <VideoThumbnail video={next} className="hidden w-card-0 shrink-0 sm:block" />
          <span className="line-clamp-2 min-w-0 text-base font-semibold wrap-anywhere">
            {next.title}
          </span>
        </Link>
        <Progress
          aria-hidden="true"
          data-countdown=""
          value={barValue}
          max={autoplayNoticeSeconds}
          className="*:duration-1000 *:ease-linear motion-reduce:*:transition-none"
        />
        <div className="flex flex-wrap gap-2">
          <Button ref={cancel} variant="secondary" onClick={onCancel}>
            <X aria-hidden="true" />
            {t.player.autoplay.cancel}
          </Button>
          <Button onClick={onPlayNow}>
            <Play aria-hidden="true" />
            {t.player.autoplay.playNow}
          </Button>
        </div>
      </div>
    </Dimmed>
  );
}
