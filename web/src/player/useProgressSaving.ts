import { useCallback, useEffect, useRef } from "react";

import { beaconProgress, saveProgress } from "../api/client";
import { newPlaybackId } from "./playbackId";

export interface PlaybackProgressStatus {
  positioned: boolean;
  ended: boolean;
}

/** Attach the watch ID only after the first identified save has started. */
interface Playback {
  videoId: number;
  playbackId: string;
  started: boolean;
}

/** Create a watch on play and send its first save after positioning. Preserve its ID across player recovery, but release it after the ended save or video changes. */
export function useProgressSaving(
  id: number,
  owner: boolean,
  { positioned, ended }: PlaybackProgressStatus,
) {
  const lastSent = useRef<{ videoId: number; positionMs: number } | null>(null);
  // Positions reported before the first play, such as a seek on the pre-play control bar, are not progress.
  const latestPosition = useRef<{
    videoId: number;
    positionMs: number;
    saved: boolean;
  } | null>(null);
  const playedVideo = useRef<number | null>(null);
  const playback = useRef<Playback | null>(null);
  const positionedRef = useRef(positioned);
  positionedRef.current = positioned;

  const send = useCallback(
    (positionMs: number, leaving: boolean, force = false) => {
      // Progress belongs to the owner; guests never save it.
      if (!owner) return;
      if (!Number.isFinite(positionMs) || positionMs < 0) return;
      const rounded = Math.round(positionMs);
      if (
        !leaving &&
        !force &&
        lastSent.current?.videoId === id &&
        Math.abs(rounded - lastSent.current.positionMs) < 1000
      ) {
        return;
      }
      lastSent.current = { videoId: id, positionMs: rounded };
      const current = playback.current;
      const playbackId =
        current?.videoId === id && current.started ? current.playbackId : undefined;
      if (leaving) {
        beaconProgress(id, rounded, playbackId);
        return;
      }
      void saveProgress(id, rounded, playbackId).catch(() => undefined);
    },
    [id, owner],
  );

  // The first identified save sets the server's watch timestamp; wait until its position is known.
  const startPlayback = useCallback(() => {
    const current = playback.current;
    if (current?.videoId !== id || current.started) return false;
    const latest = latestPosition.current;
    if (latest?.videoId !== id) return false;
    current.started = true;
    send(latest.positionMs, false, true);
    return true;
  }, [id, send]);

  const rememberProgress = useCallback(
    (positionMs: number) => {
      latestPosition.current = { videoId: id, positionMs, saved: false };
      if (positionedRef.current) startPlayback();
    },
    [id, startPlayback],
  );

  const savePlayerProgress = useCallback(
    (positionMs: number, immediate: boolean) => {
      latestPosition.current = { videoId: id, positionMs, saved: true };
      if (positionedRef.current && startPlayback()) return;
      send(positionMs, false, immediate);
    },
    [id, send, startPlayback],
  );

  const markPlayed = useCallback(() => {
    playedVideo.current = id;
    if (!owner) return;
    if (playback.current?.videoId !== id) {
      playback.current = { videoId: id, playbackId: newPlaybackId(), started: false };
    }
    // Saving before resume positioning would overwrite existing progress with zero.
    if (positionedRef.current) startPlayback();
  }, [id, owner, startPlayback]);

  useEffect(() => {
    if (positioned) startPlayback();
  }, [positioned, startPlayback]);

  // The synchronous ended save retains the watch ID before this post-render cleanup releases it.
  const markEnded = useCallback(() => {
    if (playback.current?.videoId === id) playback.current = null;
  }, [id]);

  useEffect(() => {
    if (ended) markEnded();
  }, [ended, markEnded]);

  // Navigation cleanup already sent the old watch's beacon; returning creates a new watch.
  useEffect(() => {
    if (playback.current !== null && playback.current.videoId !== id) {
      playback.current = null;
    }
  }, [id]);

  useEffect(() => {
    const sendLatest = () => {
      const latest = latestPosition.current;
      if (latest?.videoId !== id) return;
      if (latest.saved || playedVideo.current === id) send(latest.positionMs, true);
    };
    const onHidden = () => {
      if (document.visibilityState === "hidden") sendLatest();
    };
    document.addEventListener("visibilitychange", onHidden);
    window.addEventListener("pagehide", sendLatest);
    return () => {
      document.removeEventListener("visibilitychange", onHidden);
      window.removeEventListener("pagehide", sendLatest);
      sendLatest();
    };
  }, [id, send]);

  return { rememberProgress, savePlayerProgress, markPlayed };
}
