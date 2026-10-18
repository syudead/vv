/** An unlabeled subtitle uses an empty label. */
export interface SubtitlePreference {
  enabled: boolean;
  label: string;
}

const storageKey = "vv.subtitles.v1";

export const defaultSubtitlePreference: SubtitlePreference = {
  enabled: false,
  label: "",
};

/** Return off if storage is absent, invalid or unreadable. */
export function readSubtitlePreference(storage?: Storage): SubtitlePreference {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(storageKey);
  } catch {
    return { ...defaultSubtitlePreference };
  }
  if (raw === null) return { ...defaultSubtitlePreference };

  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return { ...defaultSubtitlePreference };
    }
    const value = parsed as Record<string, unknown>;
    if (typeof value.enabled !== "boolean" || typeof value.label !== "string") {
      return { ...defaultSubtitlePreference };
    }
    return { enabled: value.enabled, label: value.label };
  } catch {
    return { ...defaultSubtitlePreference };
  }
}

export function writeSubtitlePreference(
  value: SubtitlePreference,
  storage?: Storage,
): void {
  try {
    (storage ?? window.localStorage).setItem(
      storageKey,
      JSON.stringify({ enabled: value.enabled, label: value.label }),
    );
  } catch {
    // Storage failures must not block the current player's subtitle selection.
  }
}
