import { useCallback, useEffect, useRef } from "react";

import { beaconProgress, saveProgress } from "../api/client";
import { newPlaybackId } from "./playbackId";

/** PlaybackProgressStatus は、保存がプレイヤーの状態から読むもの（PlayerStatus の一部）である。 */
export interface PlaybackProgressStatus {
  /** 続きからの位置を当て終えた（PlayerStatus.positioned）。 */
  positioned: boolean;
  /** 最後まで再生した（PlayerStatus.ended）。 */
  ended: boolean;
}

/**
 * Playback は今の動画の 1 回の視聴である。started は、その識別子を付けた最初の保存を
 * 送ったかどうかで、それまでの保存と beacon には識別子を付けない。
 */
interface Playback {
  videoId: number;
  playbackId: string;
  started: boolean;
}

/**
 * useProgressSaving は再生位置の保存である。`rememberProgress` は最新の位置を覚え、
 * `savePlayerProgress` は覚えたうえで保存を送る。画面を離れる・隠れるときは、覚えた
 * 最新の位置を beacon で送る。
 *
 * 視聴履歴のために、今の動画の視聴の識別子（playback id）も持つ
 * （specs/043-watch-history/research.md R-2、R-3）。`markPlayed` が最初の play で作り、
 * プレイヤーが positioned になったら（すでにそうならすぐ）その識別子を付けた保存を 1 回
 * すぐ送る。以後の保存と beacon は同じ識別子を付ける。最後まで再生したら（ended）、その
 * 保存に識別子を付けたあとで捨て、次の play で新しく作る。動画が変わったときも捨てる。
 * プレイヤーの作り直し（失敗からの回復、画質の切り替え）では捨てない。
 */
export function useProgressSaving(
  id: number,
  owner: boolean,
  { positioned, ended }: PlaybackProgressStatus,
) {
  const lastSent = useRef<{ videoId: number; positionMs: number } | null>(null);
  const latestPosition = useRef<{ videoId: number; positionMs: number } | null>(null);
  const playback = useRef<Playback | null>(null);
  const positionedRef = useRef(positioned);
  positionedRef.current = positioned;

  const send = useCallback(
    (positionMs: number, leaving: boolean, force = false) => {
      // 再生位置は所有者のものなので、ゲストでは保存を送らない。
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

  const rememberProgress = useCallback(
    (positionMs: number) => {
      latestPosition.current = { videoId: id, positionMs };
    },
    [id],
  );

  const savePlayerProgress = useCallback(
    (positionMs: number, immediate: boolean) => {
      latestPosition.current = { videoId: id, positionMs };
      send(positionMs, false, immediate);
    },
    [id, send],
  );

  // startPlayback は、作ったまま送っていない識別子を付けた最初の保存を送る。サーバーは
  // その保存の時刻を視聴の時刻にする。
  const startPlayback = useCallback(() => {
    const current = playback.current;
    if (current?.videoId !== id || current.started) return;
    current.started = true;
    const latest = latestPosition.current;
    if (latest?.videoId === id) send(latest.positionMs, false, true);
  }, [id, send]);

  const markPlayed = useCallback(() => {
    if (!owner) return;
    if (playback.current?.videoId !== id) {
      playback.current = { videoId: id, playbackId: newPlaybackId(), started: false };
    }
    // 続きからの位置を当てる前に送ると、保存した位置を 0 などで上書きしてしまう。
    if (positionedRef.current) startPlayback();
  }, [id, owner, startPlayback]);

  useEffect(() => {
    if (positioned) startPlayback();
  }, [positioned, startPlayback]);

  // markEnded は最後まで再生した視聴を終える。プレイヤーは ended の保存を状態の知らせと
  // 同じ出来事の中で送るので、その保存は、描画のあとに走るこの捨てる処理より先に、
  // 識別子を付けて出ている。
  const markEnded = useCallback(() => {
    if (playback.current?.videoId === id) playback.current = null;
  }, [id]);

  useEffect(() => {
    if (ended) markEnded();
  }, [ended, markEnded]);

  // 別の動画へ移ったら、前の動画の識別子を捨てる（戻ってきたら新しい視聴にする）。前の動画の
  // 離脱時の送信は、この前の後片付けで識別子を付けて出ている。
  useEffect(() => {
    if (playback.current !== null && playback.current.videoId !== id) {
      playback.current = null;
    }
  }, [id]);

  useEffect(() => {
    const sendLatest = () => {
      const latest = latestPosition.current;
      if (latest?.videoId === id) send(latest.positionMs, true);
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
