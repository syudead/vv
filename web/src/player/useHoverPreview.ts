import { type PointerEvent, useCallback, useLayoutEffect, useRef, useState } from "react";

import type { Video } from "../api/client";

export const hoverPreviewDelayMs = 400;

export interface HoverPreview {
  active: boolean;
  playing: boolean;
  previewUrl: string | undefined;
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: () => void;
  videoRef: (element: HTMLVideoElement | null) => void;
  onPlaying: () => void;
  onError: () => void;
  /** Cancel the hover delay and pause without discarding the element or source. */
  suspendPreview: () => void;
  /** Restart the hover delay, then resume from the paused frame. */
  resumePreview: () => void;
}

/** Match library cards: play ready previews after 400ms of mouse hover; touch and pen never start them. */
export function useHoverPreview(video: Video): HoverPreview {
  const eligible = video.previewState === "done" && video.previewUrl !== undefined;
  const [active, setActive] = useState(false);
  const [playing, setPlaying] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const element = useRef<HTMLVideoElement | null>(null);
  const suspended = useRef(false);
  const waiting = useRef(false);

  const release = useCallback(() => {
    suspended.current = false;
    waiting.current = false;
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    const current = element.current;
    if (current !== null) {
      current.pause();
      current.removeAttribute("src");
      current.load();
    }
    setActive(false);
    setPlaying(false);
  }, []);

  // Stop and release loading before React removes the element during navigation.
  useLayoutEffect(() => () => release(), [release]);

  const scheduleStart = useCallback(() => {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      timer.current = null;
      setActive(true);
    }, hoverPreviewDelayMs);
  }, []);

  const onPointerEnter = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (!eligible || event.pointerType !== "mouse") return;
      scheduleStart();
    },
    [eligible, scheduleStart],
  );

  const videoRef = useCallback((node: HTMLVideoElement | null) => {
    element.current = node;
    if (node === null || suspended.current) return;
    play(node);
  }, []);

  const suspendPreview = useCallback(() => {
    if (suspended.current) return;
    suspended.current = true;
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
      waiting.current = true;
    }
    element.current?.pause();
  }, []);

  const resumePreview = useCallback(() => {
    if (!suspended.current) return;
    suspended.current = false;
    if (waiting.current) {
      waiting.current = false;
      scheduleStart();
      return;
    }
    if (element.current !== null) play(element.current);
  }, [scheduleStart]);

  return {
    active: active && eligible,
    playing: playing && active,
    previewUrl: video.previewUrl,
    onPointerEnter,
    onPointerLeave: release,
    videoRef,
    onPlaying: () => setPlaying(true),
    onError: release,
    suspendPreview,
    resumePreview,
  };
}

function play(node: HTMLVideoElement) {
  try {
    node.play()?.catch(() => undefined);
  } catch {
    // Ignore environments such as jsdom without play support.
  }
}
