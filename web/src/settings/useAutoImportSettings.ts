import { useCallback, useEffect, useRef, useState } from "react";

import { type AutoImportSettings, getAutoImportSettings, isAborted } from "../api/client";
import { errorText, type UiText } from "../i18n";
import { inProgress, useScan } from "../shell/ScanProvider";

const pollMs = 2000;

/** Always reload on opening, window focus and reloadToken changes to pick up other-tab edits. */
export interface AutoImportReloads {
  reloadToken?: number;
  /**
   * State displays follow import completion and poll starting every two seconds; switch-only
   * displays need neither.
   */
  followWatch?: boolean;
}

export interface AutoImportSource {
  settings: AutoImportSettings | null;
  /** Show initial errors only before any successful settings read. */
  loadError: UiText | null;
  /** No-op while saving. */
  load: () => Promise<void>;
  /** Call immediately before saving to invalidate old reads and pause reloads. */
  beginSave: () => void;
  saved: (settings: AutoImportSettings) => void;
  /** Call after both successful and failed saves. */
  endSave: () => void;
}

export function useAutoImportSettings({
  reloadToken = 0,
  followWatch = false,
}: AutoImportReloads = {}): AutoImportSource {
  const [settings, setSettings] = useState<AutoImportSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  // Only the latest-started load or save may apply its response.
  const revision = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // Pause reads during PUT so stale responses cannot overwrite or invalidate the save.
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
      // Retain prior settings on background-read failure.
      if (settingsRef.current === null) setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, reloadToken]);

  useEffect(() => {
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [load]);

  useEffect(
    () => () => {
      controller.current?.abort();
    },
    [],
  );

  useWatchReloads(followWatch, settings?.watch.state === "starting", load);

  const beginSave = useCallback(() => {
    // Discard reads started before the save.
    controller.current?.abort();
    revision.current += 1;
    saveInFlight.current = true;
  }, []);
  const endSave = useCallback(() => {
    saveInFlight.current = false;
  }, []);

  return { settings, loadError, load, beginSave, saved: setSettings, endSave };
}

/** When enabled, reload after import completion and poll every two seconds while starting. */
function useWatchReloads(enabled: boolean, starting: boolean, load: () => Promise<void>) {
  const scan = useScan();

  useEffect(() => {
    if (!enabled || !starting) return;
    const timer = window.setInterval(() => void load(), pollMs);
    return () => window.clearInterval(timer);
  }, [enabled, load, starting]);

  // Manual import completion may clear watcher problems.
  // The initial settings read already covers the first observed value.
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
