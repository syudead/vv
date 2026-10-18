import { useCallback, useLayoutEffect, useRef } from "react";

import type { Zoom } from "../preferences/viewPreferences";

const ANCHOR_SELECTOR = "[data-video-id],[data-folder-path]";

function topOffset(): number {
  return (
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-navbar"),
    ) || 48
  );
}

/** Find video anchors by ID, folder anchors by list index because root paths may repeat. */
type Anchor = { videoId: string } | { index: number };

function topmostAnchor(list: HTMLElement | null, top: number): Anchor | undefined {
  if (list === null) return undefined;
  const cards = Array.from(list.querySelectorAll<HTMLElement>(ANCHOR_SELECTOR));
  const index = cards.findIndex((card) => card.getBoundingClientRect().bottom > top);
  if (index < 0) return undefined;
  const videoId = cards[index]?.dataset.videoId;
  return videoId === undefined ? { index } : { videoId };
}

function findAnchor(list: HTMLElement | null, anchor: Anchor): Element | undefined {
  if (list === null) return undefined;
  if ("videoId" in anchor) {
    return list.querySelector(`[data-video-id="${anchor.videoId}"]`) ?? undefined;
  }
  return list.querySelectorAll(ANCHOR_SELECTOR)[anchor.index];
}

/**
 * Attach listRef and capture immediately before changing zoom; restore the nearest top card
 * afterward.
 */
export function useZoomAnchor(zoom: Zoom) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const anchor = useRef<Anchor | undefined>(undefined);

  const capture = useCallback(() => {
    anchor.current =
      window.scrollY > 0 ? topmostAnchor(listRef.current, topOffset()) : undefined;
  }, []);

  useLayoutEffect(() => {
    const current = anchor.current;
    if (current === undefined) return;
    anchor.current = undefined;
    const target = findAnchor(listRef.current, current);
    if (target === undefined) return;
    const top = window.scrollY + target.getBoundingClientRect().top - topOffset() - 8;
    window.scrollTo({ top: Math.max(top, 0), behavior: "auto" });
  }, [zoom]);

  return { listRef, capture };
}
