import videojs from "video.js";

import { transcodeUrl } from "../api/client";
import { clampTranscodeStart } from "./playbackAttempt";

const reloadDelayMs = 200;

export interface LiveSource {
  src: string;
  type: "video/mp4";
  vvLive: true;
  vvVideoId: number;
  vvDurationSeconds: number;
  vvOffsetSeconds: number;
  vvOffsetChanged?: (seconds: number) => void;
}

interface SourceObject {
  src: string;
  type?: string;
  vvLive?: boolean;
  vvVideoId?: number;
  vvDurationSeconds?: number;
  vvOffsetSeconds?: number;
  vvOffsetChanged?: (seconds: number) => void;
}

interface OffsetTech {
  buffered(): TimeRanges;
  currentTime(): number;
  setSource(source: SourceObject): void;
  playbackRate(): number;
  setPlaybackRate(rate: number): void;
  paused(): boolean;
  pause(): void;
  play(): Promise<void> | undefined;
  one(event: string, callback: () => void): void;
  trigger(event: string): void;
}

type Player = ReturnType<typeof videojs>;

export function liveSource(
  videoId: number,
  durationMs: number,
  startMs: number,
  offsetChanged?: (seconds: number) => void,
): LiveSource {
  const safeStart = clampTranscodeStart(startMs, durationMs);
  return {
    src: transcodeUrl(videoId, safeStart),
    type: "video/mp4",
    vvLive: true,
    vvVideoId: videoId,
    vvDurationSeconds: durationMs / 1000,
    vvOffsetSeconds: safeStart / 1000,
    vvOffsetChanged: offsetChanged,
  };
}

export function createLiveOffsetMiddleware(player: Player) {
  let tech: OffsetTech | undefined;
  let source: SourceObject = { src: "" };
  let offsetSeconds: number | undefined;
  let pendingOffsetSeconds: number | undefined;
  let reloadTimer: ReturnType<typeof setTimeout> | undefined;
  let playIntended = !player.paused();
  let reloadGeneration = 0;
  let disposed = false;

  const clearReload = () => {
    if (reloadTimer !== undefined) clearTimeout(reloadTimer);
    reloadTimer = undefined;
  };
  const requestPlayback = () => {
    void tech?.play()?.catch(() => undefined);
  };
  player.one("dispose", () => {
    disposed = true;
    reloadGeneration += 1;
    clearReload();
  });

  const reloadAt = (seconds: number) => {
    if (
      tech === undefined ||
      source.vvVideoId === undefined ||
      source.vvDurationSeconds === undefined
    ) {
      return;
    }
    clearReload();
    pendingOffsetSeconds = seconds;
    const generation = ++reloadGeneration;
    reloadTimer = setTimeout(() => {
      reloadTimer = undefined;
      if (disposed || tech === undefined || generation !== reloadGeneration) return;
      const shouldResume = playIntended || !tech.paused();
      playIntended = shouldResume;
      const playbackRate = tech.playbackRate();
      offsetSeconds = seconds;
      pendingOffsetSeconds = undefined;
      source.vvOffsetChanged?.(seconds);
      source = liveSource(
        source.vvVideoId as number,
        (source.vvDurationSeconds as number) * 1000,
        seconds * 1000,
        source.vvOffsetChanged,
      );
      tech.one("canplay", () => {
        if (disposed || tech === undefined || generation !== reloadGeneration) return;
        if (playIntended) {
          if (tech.paused()) requestPlayback();
        } else {
          tech.pause();
        }
      });
      tech.setSource(source);
      tech.setPlaybackRate(playbackRate);
      tech.trigger("timeupdate");
      // A paused HTML media element with preload=metadata may not consume enough
      // of an open-ended fragmented MP4 to reach canplay. Start the request now;
      // the canplay handler restores a paused seek immediately when required.
      requestPlayback();
    }, reloadDelayMs);
  };

  return {
    setTech(nextTech: unknown) {
      tech = nextTech as OffsetTech;
    },
    setSource(
      nextSource: SourceObject,
      next: (error: null, source: SourceObject) => void,
    ) {
      clearReload();
      reloadGeneration += 1;
      source = nextSource;
      offsetSeconds = nextSource.vvLive ? (nextSource.vvOffsetSeconds ?? 0) : undefined;
      pendingOffsetSeconds = undefined;
      next(null, nextSource);
    },
    duration(seconds: number) {
      return source.vvLive && source.vvDurationSeconds !== undefined
        ? source.vvDurationSeconds
        : seconds;
    },
    currentTime(seconds: number) {
      if (pendingOffsetSeconds !== undefined) return pendingOffsetSeconds;
      return (offsetSeconds ?? 0) + seconds;
    },
    buffered(ranges: TimeRanges) {
      if (offsetSeconds === undefined) return ranges;
      const shifted: [number, number][] = [];
      for (let index = 0; index < ranges.length; index += 1) {
        shifted.push([
          ranges.start(index) + offsetSeconds,
          ranges.end(index) + offsetSeconds,
        ]);
      }
      const createTimeRanges = videojs.time.createTimeRanges as unknown as (
        values: [number, number][],
      ) => TimeRanges;
      return createTimeRanges(shifted);
    },
    setCurrentTime(seconds: number) {
      if (offsetSeconds === undefined || tech === undefined) return seconds;
      const relative = seconds - offsetSeconds;
      const ranges = tech.buffered();
      for (let index = 0; index < ranges.length; index += 1) {
        if (ranges.start(index) <= relative && relative <= ranges.end(index)) {
          clearReload();
          reloadGeneration += 1;
          pendingOffsetSeconds = undefined;
          return relative;
        }
      }
      const duration = source.vvDurationSeconds ?? seconds;
      const target = clampTranscodeStart(seconds * 1000, duration * 1000) / 1000;
      reloadAt(target);
      // Keep the current source on its current frame until the replacement is
      // ready. Returning zero here visibly rewinds the old source on every seek.
      return tech.currentTime();
    },
    callPlay() {
      playIntended = true;
    },
    callPause() {
      playIntended = false;
    },
  };
}

videojs.use("*", createLiveOffsetMiddleware);
