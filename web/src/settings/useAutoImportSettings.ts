import { useCallback, useEffect, useRef, useState } from "react";

import { type AutoImportSettings, getAutoImportSettings, isAborted } from "../api/client";
import { errorText, type UiText } from "../i18n";
import { inProgress, useScan } from "../shell/ScanProvider";

/** pollMs は監視を張っている最中（starting）に状態を読み直す間隔である。 */
const pollMs = 2000;

/**
 * AutoImportReloads は、自動の取り込みの設定を読み直す契機である。開いたときと `reloadToken` が
 * 変わったときは、いつも読み直す。
 */
export interface AutoImportReloads {
  /** 変わるたびに読み直す（メディアフォルダを変えたあとなど）。 */
  reloadToken?: number;
  /**
   * 監視の状態を追う。ウィンドウへ戻ったとき、直近の取り込みが終わったときに読み直し、
   * 監視を張っている最中（`starting`）は 2 秒ごとに読み直す。監視の状態を出す
   * AutoImportState が要る。切り替えだけを出す AutoImportSwitch は要らない。
   */
  followWatch?: boolean;
}

/** AutoImportSource は、自動の取り込みの設定の読み込みと、保存との重なりの扱いである。 */
export interface AutoImportSource {
  /** 読めた設定。読み終えるまでは null。 */
  settings: AutoImportSettings | null;
  /** 最初の読み込みの失敗の文。読めている状態があるあいだは null のまま。 */
  loadError: UiText | null;
  /** 読み直す。保存の最中は何もしない。 */
  load: () => Promise<void>;
  /** 保存を始める直前に呼ぶ。保存より前に始めた読み込みの応答を捨て、保存が終わるまで読み直さない。 */
  beginSave: () => void;
  /** 保存した設定を受け取る。 */
  saved: (settings: AutoImportSettings) => void;
  /** 保存が成否を問わず終わったときに呼ぶ。 */
  endSave: () => void;
}

/**
 * useAutoImportSettings は、自動の取り込みの設定（GET /api/settings/auto-import）を読み、
 * `reloads` の契機で読み直す。監視の状態（AutoImportState）と切り替え（AutoImportSwitch）は、
 * それぞれ要る契機でこれを使う。
 */
export function useAutoImportSettings({
  reloadToken = 0,
  followWatch = false,
}: AutoImportReloads = {}): AutoImportSource {
  const [settings, setSettings] = useState<AutoImportSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  // 読み込みと保存は重なる。いちばん新しく始めたものの応答だけを使う。
  const revision = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // 保存（PUT）の最中は読み直さない。重なった読み込みの古い値が、保存の結果を上書きしたり
  // 保存の応答を捨てさせたりしないよう、保存が終わるまで読み込みを止める。
  const saveInFlight = useRef(false);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const load = useCallback(async () => {
    if (saveInFlight.current) return;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    revision.current += 1;
    const mine = revision.current;
    try {
      const next = await getAutoImportSettings(abort.signal);
      if (mine !== revision.current) return;
      setSettings(next);
      setLoadError(null);
    } catch (failure) {
      if (isAborted(failure) || mine !== revision.current) return;
      // 読めている状態は残す。読み直しの失敗で、区画を壊さない。
      if (settingsRef.current === null) setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, reloadToken]);

  useEffect(
    () => () => {
      controller.current?.abort();
    },
    [],
  );

  useWatchReloads(followWatch, settings?.watch.state === "starting", load);

  const beginSave = useCallback(() => {
    // 保存より前に始めた読み込みの応答は使わない。
    controller.current?.abort();
    revision.current += 1;
    saveInFlight.current = true;
  }, []);
  const endSave = useCallback(() => {
    saveInFlight.current = false;
  }, []);

  return { settings, loadError, load, beginSave, saved: setSettings, endSave };
}

/**
 * useWatchReloads は、監視の状態を追う読み直しである。`enabled` が偽なら何もしない。
 * ウィンドウへ戻ったとき、直近の取り込みが終わったときに読み直し、`starting` のあいだは
 * 2 秒ごとに読み直す。
 */
function useWatchReloads(enabled: boolean, starting: boolean, load: () => Promise<void>) {
  const scan = useScan();

  useEffect(() => {
    if (!enabled) return;
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [enabled, load]);

  useEffect(() => {
    if (!enabled || !starting) return;
    const timer = window.setInterval(() => void load(), pollMs);
    return () => window.clearInterval(timer);
  }, [enabled, load, starting]);

  // 直近の取り込みが終わったら読み直す。問題は、手動の取り込みの完了で消えることがある。
  // 最初に見た値は、開いたときの読み込みが担う。
  const endedId = scan.scan !== null && !inProgress(scan.scan) ? scan.scan.id : null;
  const seenEnded = useRef<number | null | undefined>(undefined);
  useEffect(() => {
    if (!enabled || !scan.loaded) return;
    if (seenEnded.current === undefined) {
      seenEnded.current = endedId;
      return;
    }
    if (endedId !== null && endedId !== seenEnded.current) void load();
    seenEnded.current = endedId;
  }, [enabled, endedId, load, scan.loaded]);
}
