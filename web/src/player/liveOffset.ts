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
  setSource(source: SourceObject): void;
  playbackRate(): number;
  setPlaybackRate(rate: number): void;
  paused(): boolean;
  pause(): void;
  play(): void;
  scrubbing(): boolean;
  one(event: string, callback: () => void): void;
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

  const clearReload = () => {
    if (reloadTimer !== undefined) clearTimeout(reloadTimer);
    reloadTimer = undefined;
  };
  player.one("dispose", clearReload);

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
    reloadTimer = setTimeout(() => {
      if (tech === undefined) return;
      const shouldResume = playIntended || !tech.paused();
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
      tech.setSource(source);
      tech.setPlaybackRate(playbackRate);
      tech.one("canplay", () => {
        if (shouldResume && !tech?.scrubbing()) tech?.play();
        else tech?.pause();
      });
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
          pendingOffsetSeconds = undefined;
          return relative;
        }
      }
      const duration = source.vvDurationSeconds ?? seconds;
      const target = clampTranscodeStart(seconds * 1000, duration * 1000) / 1000;
      reloadAt(target);
      return 0;
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
