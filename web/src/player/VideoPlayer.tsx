import { useEffect, useRef } from "react";
import videojs from "video.js";
import "video.js/dist/video-js.css";

import { streamUrl, type Video } from "../api/client";
import { liveSource } from "./liveOffset";
import {
  createPlaybackAttempt,
  fallbackToTranscode,
  updatePosition,
  type PlaybackAttempt,
} from "./playbackAttempt";

const saveIntervalMs = 5000;

interface Props {
  video: Video;
  initialPositionMs: number;
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  onResumed: (positionMs: number) => void;
  onError: () => void;
}

export default function VideoPlayer({
  video,
  initialPositionMs,
  onPosition,
  onProgress,
  onResumed,
  onError,
}: Props) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const callbacks = useRef({ onPosition, onProgress, onResumed, onError });

  useEffect(() => {
    callbacks.current = { onPosition, onProgress, onResumed, onError };
  }, [onError, onPosition, onProgress, onResumed]);

  useEffect(() => {
    const host = hostRef.current;
    const initialAttempt = createPlaybackAttempt(video, initialPositionMs);
    if (host === null || initialAttempt === null) return;

    const element = document.createElement("video-js");
    element.classList.add("video-js", "vjs-big-play-centered");
    host.appendChild(element);

    const player = videojs(element, {
      controls: true,
      fill: true,
      playsinline: true,
      poster: video.thumbnailUrl,
      preload: "metadata",
    });
    let attempt: PlaybackAttempt = initialAttempt;
    let switchingSource = false;
    let resumeReported = false;

    const logicalPositionMs = () => {
      if (attempt.state === "loading") return attempt.logicalPositionMs;
      const seconds = player.currentTime();
      const position = typeof seconds === "number" ? seconds * 1000 : 0;
      attempt = updatePosition(attempt, position);
      return attempt.logicalPositionMs;
    };
    const reportPosition = () => {
      const position = logicalPositionMs();
      callbacks.current.onPosition(position);
      return position;
    };
    const setLiveSource = (positionMs: number) => {
      player.src(
        liveSource(video.id, attempt.durationMs, positionMs, (seconds) => {
          attempt = {
            ...updatePosition(attempt, seconds * 1000),
            sourceOffsetMs: seconds * 1000,
          };
          callbacks.current.onPosition(attempt.logicalPositionMs);
        }),
      );
    };

    player.on("loadedmetadata", () => {
      attempt = { ...attempt, state: "ready" };
      if (resumeReported || initialPositionMs <= 0) return;
      resumeReported = true;
      if (attempt.route === "direct") player.currentTime(initialPositionMs / 1000);
      callbacks.current.onResumed(initialPositionMs);
      callbacks.current.onPosition(initialPositionMs);
    });
    player.on("timeupdate", reportPosition);
    player.on("play", () => {
      attempt = { ...attempt, state: "playing", playIntended: true };
    });
    player.on("pause", () => {
      if (switchingSource || player.error() !== null) return;
      attempt = { ...attempt, playIntended: false };
      callbacks.current.onProgress(reportPosition(), true);
    });
    player.on("ended", () => callbacks.current.onProgress(reportPosition(), true));
    player.on("error", () => {
      const position = logicalPositionMs();
      const fallback = fallbackToTranscode(
        attempt,
        position,
        attempt.playIntended || !player.paused(),
      );
      if (fallback === null) {
        attempt = { ...attempt, state: "failed" };
        callbacks.current.onError();
        return;
      }

      attempt = fallback;
      switchingSource = true;
      player.error(null);
      setLiveSource(attempt.sourceOffsetMs);
      player.one("canplay", () => {
        switchingSource = false;
        if (attempt.playIntended) void player.play()?.catch(() => undefined);
        else player.pause();
      });
    });

    if (attempt.route === "direct") {
      player.src({ src: streamUrl(video.id), type: directContentType(video) });
    } else {
      setLiveSource(attempt.sourceOffsetMs);
    }

    const timer = window.setInterval(() => {
      if (!player.paused() && !player.ended()) {
        callbacks.current.onProgress(reportPosition(), false);
      }
    }, saveIntervalMs);

    return () => {
      window.clearInterval(timer);
      attempt = { ...attempt, state: "disposed" };
      if (!player.isDisposed()) player.dispose();
    };
  }, [initialPositionMs, video]);

  return <div ref={hostRef} className="vv-video-player absolute inset-0" />;
}

function directContentType(video: Video): string {
  return video.container === "webm" ? "video/webm" : "video/mp4";
}

export function canStartPlayback(video: Video): boolean {
  return createPlaybackAttempt(video, 0) !== null;
}
