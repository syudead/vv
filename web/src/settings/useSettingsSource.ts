import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { errorText, type UiText } from "../i18n";

export interface SettingsSource<S> {
  settings: S | null;
  /** Show initial errors only before any successful settings read. */
  loadError: UiText | null;
  /** No-op while saving. */
  load: () => Promise<void>;
  /** Call immediately before saving to invalidate old reads and pause reloads. */
  beginSave: () => void;
  saved: (settings: S) => void;
  /** Apply a saved part to the latest settings, for saves that each change one item; no-op before a read. */
  update: (change: (settings: S) => S) => void;
  /** Call after both successful and failed saves. */
  endSave: () => void;
}

/**
 * Read a settings resource on mount and whenever reloadToken changes; only the latest-started read
 * or save applies its response. read must be stable across renders.
 */
export function useSettingsSource<S>(
  read: (signal: AbortSignal) => Promise<S>,
  reloadToken = 0,
): SettingsSource<S> {
  const [settings, setSettings] = useState<S | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const revision = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // Pause reads while any save runs so stale responses cannot overwrite or invalidate it.
  const savesInFlight = useRef(0);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const load = useCallback(async () => {
    if (savesInFlight.current > 0) return;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    revision.current += 1;
    const mine = revision.current;
    try {
      const next = await read(abort.signal);
      if (mine !== revision.current) return;
      setSettings(next);
      setLoadError(null);
    } catch (failure) {
      if (isAborted(failure) || mine !== revision.current) return;
      // Retain prior settings on background-read failure.
      if (settingsRef.current === null) setLoadError(errorText(failure));
    }
  }, [read]);

  useEffect(() => {
    void load();
  }, [load, reloadToken]);

  useEffect(
    () => () => {
      controller.current?.abort();
    },
    [],
  );

  const beginSave = useCallback(() => {
    // Discard reads started before the save.
    controller.current?.abort();
    revision.current += 1;
    savesInFlight.current += 1;
  }, []);
  const endSave = useCallback(() => {
    savesInFlight.current -= 1;
  }, []);

  const update = useCallback((change: (settings: S) => S) => {
    setSettings((current) => (current === null ? current : change(current)));
  }, []);

  return { settings, loadError, load, beginSave, saved: setSettings, update, endSave };
}
