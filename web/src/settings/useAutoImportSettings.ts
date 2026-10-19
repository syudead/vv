import { useEffect, useRef } from "react";

import { type AutoImportSettings, getAutoImportSettings } from "../api/client";
import { inProgress, useScan } from "../shell/ScanProvider";

import { type SettingsSource, useSettingsSource } from "./useSettingsSource";

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

export type AutoImportSource = SettingsSource<AutoImportSettings>;

export function useAutoImportSettings({
  reloadToken = 0,
  followWatch = false,
}: AutoImportReloads = {}): AutoImportSource {
  const source = useSettingsSource(getAutoImportSettings, reloadToken);
  const { load } = source;

  useEffect(() => {
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [load]);

  useWatchReloads(followWatch, source.settings?.watch.state === "starting", load);

  return source;
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
