import videojs from "video.js";

import { getTranscodeStart, transcodeUrl } from "../api/client";
import { clampTranscodeStart } from "./playbackAttempt";

const reloadDelayMs = 200;

export interface LiveSource {
  src: string;
  type: "video/mp4";
  vvLive: true;
  vvVideoId: number;
  vvDurationSeconds: number;
  vvOffsetSeconds: number;
  vvAttempt?: string;
  vvOffsetChanged?: (seconds: number) => void;
}

interface SourceObject {
  src: string;
  type?: string;
  vvLive?: boolean;
  vvVideoId?: number;
  vvDurationSeconds?: number;
  vvOffsetSeconds?: number;
  vvAttempt?: string;
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

/**
 * newAttempt は変換の要求ごとの識別子（`[A-Za-z0-9_-]{1,64}`）を作る。
 * crypto.randomUUID は安全でない文脈（LAN の http）で使えないので、乱数から作る。
 */
function newAttempt(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/**
 * liveSource は元動画の startMs からのライブ変換の source を作る。途中から始めるときは
 * attempt を付け、仲立ちが実際の開始位置を引けるようにする。コピーで始めた変換は
 * 直前のキーフレームから始まり、指定位置とずれるためである
 * （specs/018-live-transcode-seek/contracts/transcode-start-api.md §3）。
 */
export function liveSource(
  videoId: number,
  durationMs: number,
  startMs: number,
  offsetChanged?: (seconds: number) => void,
): LiveSource {
  const safeStart = clampTranscodeStart(startMs, durationMs);
  const attempt = safeStart > 0 ? newAttempt() : undefined;
  return {
    src: transcodeUrl(videoId, safeStart, attempt),
    type: "video/mp4",
    vvLive: true,
    vvVideoId: videoId,
    vvDurationSeconds: durationMs / 1000,
    vvOffsetSeconds: safeStart / 1000,
    vvAttempt: attempt,
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
  let startReport: AbortController | undefined;

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
    startReport?.abort();
  });

  // awaitActualStart は source を設定した直後に呼び、実際の開始位置の報告を取りに行く。
  // 届くまでは現在時刻に指定位置を返し、届いたら offset をその値に置き換える。404 か
  // 誤りなら指定位置のまま（報告の無い変換と同じ表示）にする。source を差し替えたあとに
  // 届いた古い attempt の報告は捨てる。
  const awaitActualStart = (current: SourceObject) => {
    startReport?.abort();
    startReport = undefined;
    const attempt = current.vvAttempt;
    if (!current.vvLive || attempt === undefined || current.vvVideoId === undefined)
      return;
    const requested = current.vvOffsetSeconds ?? 0;
    pendingOffsetSeconds = requested;
    const controller = new AbortController();
    startReport = controller;
    const isCurrent = () =>
      !disposed && !controller.signal.aborted && source.vvAttempt === attempt;
    getTranscodeStart(current.vvVideoId, attempt, controller.signal).then(
      (startMs) => {
        if (!isCurrent()) return;
        startReport = undefined;
        const actual = startMs / 1000;
        offsetSeconds = actual;
        if (pendingOffsetSeconds === requested) pendingOffsetSeconds = undefined;
        if (actual !== requested) source.vvOffsetChanged?.(actual);
        tech?.trigger("timeupdate");
      },
      () => {
        if (!isCurrent()) return;
        startReport = undefined;
        if (pendingOffsetSeconds === requested) pendingOffsetSeconds = undefined;
        tech?.trigger("timeupdate");
      },
    );
  };

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
      awaitActualStart(source);
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
      awaitActualStart(nextSource);
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
