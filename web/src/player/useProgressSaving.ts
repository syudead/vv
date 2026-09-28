import { useCallback, useEffect, useRef } from "react";

import { beaconProgress, saveProgress } from "../api/client";

/**
 * useProgressSaving は再生位置の保存である。`rememberProgress` は最新の位置を覚え、
 * `savePlayerProgress` は覚えたうえで保存を送る。画面を離れる・隠れるときは、覚えた
 * 最新の位置を beacon で送る。
 */
export function useProgressSaving(id: number, owner: boolean) {
  const lastSent = useRef<{ videoId: number; positionMs: number } | null>(null);
  const latestPosition = useRef<{ videoId: number; positionMs: number } | null>(null);

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
      if (leaving) {
        beaconProgress(id, rounded);
        return;
      }
      void saveProgress(id, rounded).catch(() => undefined);
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

  return { rememberProgress, savePlayerProgress };
}
