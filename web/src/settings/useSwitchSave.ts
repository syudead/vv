import { useState } from "react";

import { errorText, type UiText } from "../i18n";

export interface SwitchSaveOptions<S> {
  /** Null until settings load; toggles are no-ops meanwhile. */
  settings: S | null;
  current: (settings: S) => boolean;
  save: (next: boolean) => Promise<S>;
  saved: (settings: S) => void;
  saveFailed: (reason: UiText) => UiText;
  /** Call before saving to stop overlapping reads. */
  onStart?: () => void;
  /** Call after success or failure. */
  onSettled?: () => void;
}

export interface SwitchSave {
  /** Pending optimistic value; null means not saving. */
  saving: boolean | null;
  saveError: UiText | null;
  /** No-op before settings load or while saving. */
  toggle: () => Promise<void>;
}

/**
 * Display the optimistic value while saving and ignore repeated toggles; clearing saving restores
 * the stored value on failure.
 */
export function useSwitchSave<S>(options: SwitchSaveOptions<S>): SwitchSave {
  const [saving, setSaving] = useState<boolean | null>(null);
  const [saveError, setSaveError] = useState<UiText | null>(null);

  const toggle = async () => {
    const { settings } = options;
    if (settings === null || saving !== null) return;
    const next = !options.current(settings);
    options.onStart?.();
    setSaveError(null);
    setSaving(next);
    try {
      options.saved(await options.save(next));
    } catch (failure) {
      setSaveError(options.saveFailed(errorText(failure)));
    } finally {
      options.onSettled?.();
      setSaving(null);
    }
  };

  return { saving, saveError, toggle };
}
