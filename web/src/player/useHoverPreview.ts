import { type PointerEvent, useCallback, useLayoutEffect, useRef, useState } from "react";

import type { Video } from "../api/client";

/** 一覧のカードと同じく、マウスを乗せてから再生を始めるまでの待ち時間。 */
export const hoverPreviewDelayMs = 400;

export interface HoverPreview {
  /** プレビューの video 要素を置くか。 */
  active: boolean;
  /** 再生が始まり、サムネイルの代わりに見せてよいか。 */
  playing: boolean;
  previewUrl: string | undefined;
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: () => void;
  videoRef: (element: HTMLVideoElement | null) => void;
  onPlaying: () => void;
  onError: () => void;
}

/**
 * useHoverPreview は、関連動画のサムネイルにマウスを乗せたときに一覧用プレビューを
 * 流す。一覧のカード（library/VideoCard）と同じ規則にそろえる。
 *
 * - プレビューが作成済み（previewState=done で previewUrl がある）の動画だけ。
 * - マウスだけ。タッチやペンでは流さない。
 * - 乗せて 400ms 後に、音を消して繰り返し流す。離したらすぐ止め、読み込みも捨てる。
 */
export function useHoverPreview(video: Video): HoverPreview {
  const eligible = video.previewState === "done" && video.previewUrl !== undefined;
  const [active, setActive] = useState(false);
  const [playing, setPlaying] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const element = useRef<HTMLVideoElement | null>(null);

  const release = useCallback(() => {
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

  // 画面を移るときも、React が要素を外す前に止めて読み込みを捨てる。
  useLayoutEffect(() => () => release(), [release]);

  const onPointerEnter = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (!eligible || event.pointerType !== "mouse") return;
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        timer.current = null;
        setActive(true);
      }, hoverPreviewDelayMs);
    },
    [eligible],
  );

  const videoRef = useCallback((node: HTMLVideoElement | null) => {
    element.current = node;
    if (node === null) return;
    try {
      // 再生できなかったときは onError か、サムネイルのまま何も起きない。
      node.play()?.catch(() => undefined);
    } catch {
      // jsdom など play を持たない環境では何もしない。
    }
  }, []);

  return {
    active: active && eligible,
    playing: playing && active,
    previewUrl: video.previewUrl,
    onPointerEnter,
    onPointerLeave: release,
    videoRef,
    onPlaying: () => setPlaying(true),
    onError: release,
  };
}
