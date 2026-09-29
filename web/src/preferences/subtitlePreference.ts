/**
 * SubtitlePreference は、このブラウザで最後に選んだ字幕である（specs/028-sidecar-subtitles
 * research.md R-9）。label はラベルの無い字幕なら `""`。
 */
export interface SubtitlePreference {
  enabled: boolean;
  label: string;
}

const storageKey = "vv.subtitles.v1";

export const defaultSubtitlePreference: SubtitlePreference = {
  enabled: false,
  label: "",
};

/** 保存値が無い・壊れている・読めない場合も、既定値（オフ）を返す。 */
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

/** 字幕の選択をこのブラウザに保存する。保存領域が使えなくても再生は妨げない。 */
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
    // プライベートモードなどで保存できなくても、今のプレイヤーでは選んだ字幕が出る。
  }
}
