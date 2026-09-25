import { useCallback, useLayoutEffect, useRef } from "react";

import type { Zoom } from "../preferences/viewPreferences";

/** 位置の目印にするカード（動画のカードとフォルダのカード）。 */
const ANCHOR_SELECTOR = "[data-video-id],[data-folder-path]";

function topOffset(): number {
  return (
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-navbar"),
    ) || 48
  );
}

/**
 * Anchor は目印にしたカードである。動画のカードは id で探し直し、フォルダのカード
 * （最上位では path が重なる）は一覧の中の順番で探し直す。
 */
type Anchor = { videoId: string } | { index: number };

/** topmostAnchor は画面上端に最も近いカードを返す。 */
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
 * useZoomAnchor は表示倍率を変えたときに、読んでいた位置を保つ。
 * `listRef` をカードを包む要素に付け、倍率を変える直前に `capture` を呼ぶ。
 * 画面上端に最も近かったカードを、倍率を変えた後も上端へ戻す。
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
