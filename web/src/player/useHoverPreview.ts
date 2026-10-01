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
  /**
   * suspendPreview はスクラブの帯に入ったときに、400ms の待ちを止め、再生中・読み込み中の
   * 動画を src と要素を保ったまま止める（specs/032-card-scrub-preview/research.md R-2）。
   */
  suspendPreview: () => void;
  /**
   * resumePreview は帯からサムネイルの中へ出たときに、待ちを 400ms から数え直し、止めた
   * 動画をその場面から再生し直す。
   */
  resumePreview: () => void;
}

/**
 * useHoverPreview は、関連動画のサムネイルにマウスを乗せたときに一覧用プレビューを
 * 流す。一覧のカード（videoList/VideoCard）と同じ規則にそろえる。
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
  // 帯にいる間の一時停止。waiting は待ちの途中で止めたことを表す。
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

  // 画面を移るときも、React が要素を外す前に止めて読み込みを捨てる。
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
    // 再生できなかったときは onError か、サムネイルのまま何も起きない。
    node.play()?.catch(() => undefined);
  } catch {
    // jsdom など play を持たない環境では何もしない。
  }
}
