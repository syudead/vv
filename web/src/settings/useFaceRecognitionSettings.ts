import { useEffect } from "react";

import { type FaceRecognitionSettings, getFaceRecognitionSettings } from "../api/client";

import { type SettingsSource, useSettingsSource } from "./useSettingsSource";

export const downloadPollMs = 1000;
export const queuePollMs = 3000;

export type FaceRecognitionSource = SettingsSource<FaceRecognitionSettings>;

/** Poll every second while downloading so the bar moves, and every three seconds while videos wait or run. */
export function facePollMs(settings: FaceRecognitionSettings | null): number | null {
  if (settings === null) return null;
  if (settings.runtime.state === "downloading") return downloadPollMs;
  return settings.counts.queued + settings.counts.running > 0 ? queuePollMs : null;
}

export function useFaceRecognitionSettings(): FaceRecognitionSource {
  const source = useSettingsSource(getFaceRecognitionSettings);
  const { load } = source;
  const interval = facePollMs(source.settings);

  useEffect(() => {
    if (interval === null) return;
    const timer = window.setInterval(() => void load(), interval);
    return () => window.clearInterval(timer);
  }, [interval, load]);

  return source;
}
