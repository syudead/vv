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

/** topmostIndex は画面上端に最も近いカードが、一覧の中で何番目かを返す。 */
function topmostIndex(list: HTMLElement | null, top: number): number | undefined {
  if (list === null) return undefined;
  const cards = Array.from(list.querySelectorAll<HTMLElement>(ANCHOR_SELECTOR));
  const index = cards.findIndex((card) => card.getBoundingClientRect().bottom > top);
  return index < 0 ? undefined : index;
}

/**
 * useZoomAnchor は表示倍率を変えたときに、読んでいた位置を保つ。
 * `listRef` をカードを包む要素に付け、倍率を変える直前に `capture` を呼ぶ。
 * 画面上端に最も近かったカードを、倍率を変えた後も上端へ戻す。
 */
export function useZoomAnchor(zoom: Zoom) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const anchor = useRef<number | undefined>(undefined);

  const capture = useCallback(() => {
    anchor.current =
      window.scrollY > 0 ? topmostIndex(listRef.current, topOffset()) : undefined;
  }, []);

  useLayoutEffect(() => {
    const index = anchor.current;
    if (index === undefined) return;
    anchor.current = undefined;
    const target = listRef.current?.querySelectorAll(ANCHOR_SELECTOR)[index];
    if (target === undefined) return;
    const top = window.scrollY + target.getBoundingClientRect().top - topOffset() - 8;
    window.scrollTo({ top: Math.max(top, 0), behavior: "auto" });
  }, [zoom]);

  return { listRef, capture };
}
