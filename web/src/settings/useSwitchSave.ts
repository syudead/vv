import { useState } from "react";

import { errorText, type UiText } from "../i18n";

/** useSwitchSave に渡す、区画ごとの読み書きである。 */
export interface SwitchSaveOptions<S> {
  /** 今の設定。読み終えるまでは null で、押しても何もしない。 */
  settings: S | null;
  /** 設定のうちスイッチが映す値である。 */
  current: (settings: S) => boolean;
  /** 押した先の値を保存し、保存した設定を返す。 */
  save: (next: boolean) => Promise<S>;
  /** 保存した設定を受け取る。 */
  saved: (settings: S) => void;
  /** 失敗の理由から、区画に出す文を作る。 */
  saveFailed: (reason: UiText) => UiText;
  /** 保存を始める直前に呼ぶ（重なった読み込みを止めるなど）。 */
  onStart?: () => void;
  /** 保存が成否を問わず終わったときに呼ぶ。 */
  onSettled?: () => void;
}

/** useSwitchSave が返す、スイッチの保存の状態と操作である。 */
export interface SwitchSave {
  /** 保存中なら押した先の値、保存していなければ null。スイッチは保存中この値を映す。 */
  saving: boolean | null;
  /** 直近の保存の失敗の文。無ければ null。 */
  saveError: UiText | null;
  /** 押した先の値を保存する。読み終える前と保存中は何もしない。 */
  toggle: () => Promise<void>;
}

/**
 * useSwitchSave は、押すとすぐに保存する設定のスイッチの保存を受け持つ。保存中は押した先の値を
 * 示して二度目の押下に反応せず、失敗すると理由を残す。保存を解くとスイッチは保存した値に戻る。
 * 「Auto-import」「Network」「Duplicate judging」の区画が共有し、表示は SaveStatus が担う。
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
