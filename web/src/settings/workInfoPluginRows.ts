import type { WorkInfoPlugin, WorkInfoPluginSetting } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";

// 設定画面の「Work-info plugins」節が行とダイアログに出す値の決まりである
// （specs/049-work-info-plugins/ui-design.md「Plugin row」「Settings dialog」、既定値と選択肢は
// specs/056-plugin-tooling/ui-design.md「Settings dialog」）。

/** MAX_PLUGIN_BYTES は送る前に断るファイルの大きさの上限（16 MiB）である。 */
export const MAX_PLUGIN_BYTES = 16 * 1024 * 1024;

/** MAX_SETTING_LENGTH は設定の値 1 つの上限である。サーバーと同じく符号位置で数える（screen-api.md）。 */
export const MAX_SETTING_LENGTH = 2000;

/** pluginName は行と文言に出す名前である。マニフェストを読めなかったプラグインはファイル名。 */
export function pluginName(plugin: Pick<WorkInfoPlugin, "name" | "fileName">): string {
  return plugin.name ?? plugin.fileName;
}

/** operationTexts はプラグインの操作を宣言の順の文言にする。 */
export function operationTexts(plugin: Pick<WorkInfoPlugin, "operations">): UiText[] {
  return plugin.operations.map(
    (operation) => t.settings.workInfoPlugins.operations[operation],
  );
}

function summaryOf(setting: WorkInfoPluginSetting): UiText {
  const text = t.settings.workInfoPlugins;
  if (setting.staleValue !== undefined) return text.settingNotSet(setting.label);
  if (setting.set) return text.settingSet(setting.label);
  if (setting.default !== undefined) return text.settingDefault(setting.label);
  return text.settingNotSet(setting.label);
}

/**
 * settingsSummary は事実の行の設定の要約である。設定を宣言しないプラグインには無い。既定値で
 * 動く設定は `{label} default`、選択肢の外の保存値は `{label} not set` と数える。
 */
export function settingsSummary(plugin: Pick<WorkInfoPlugin, "settings">): UiText | null {
  if (plugin.settings.length === 0) return null;
  return t.settings.workInfoPlugins.settingsSummary(plugin.settings.map(summaryOf));
}

/**
 * missingRequired は値の無い `required` の設定のラベルを宣言の順に返す。既定値のある設定は
 * 実行を止めないので挙げない（specs/056-plugin-tooling/research.md R-3）。
 */
export function missingRequired(plugin: Pick<WorkInfoPlugin, "settings">): string[] {
  return plugin.settings
    .filter(
      (setting) => setting.required && !setting.set && setting.default === undefined,
    )
    .map((setting) => setting.label);
}

/** staleSettings は保存値が今の版の選択肢に無い設定のラベルを宣言の順に返す（`Choose again`）。 */
export function staleSettings(plugin: Pick<WorkInfoPlugin, "settings">): string[] {
  return plugin.settings
    .filter((setting) => setting.staleValue !== undefined)
    .map((setting) => setting.label);
}

/**
 * SettingDraft は設定のダイアログの欄 1 つである。秘密の欄の value は今入力した値だけで、
 * 保存済みの値は持たない。forgotten は保存済みの秘密を「Forget」で消す印である。選択肢の欄では
 * 空が先頭の項目（既定値か Not set）で、選択肢の外の保存値は value にそのまま残る。
 */
export interface SettingDraft {
  value: string;
  forgotten: boolean;
}

/** draftsOf はダイアログを開いたときの欄である。秘密の欄は常に空で始まる。 */
export function draftsOf(
  settings: readonly WorkInfoPluginSetting[],
): Record<string, SettingDraft> {
  return Object.fromEntries(
    settings.map((setting) => [
      setting.key,
      { value: setting.secret ? "" : (setting.value ?? ""), forgotten: false },
    ]),
  );
}

function changeOf(
  setting: WorkInfoPluginSetting,
  draft: SettingDraft | undefined,
): string | null | undefined {
  if (draft === undefined) return undefined;
  if (setting.secret) {
    if (draft.forgotten) return setting.set ? null : undefined;
    return draft.value === "" ? undefined : draft.value;
  }
  const stored = setting.value ?? "";
  if (draft.value === stored) return undefined;
  if (draft.value === "") return setting.set ? null : undefined;
  return draft.value;
}

/**
 * settingChanges は保存で送る値である。変わったキーだけを持ち、消す値は `null` にする。
 * 空の秘密の欄は保存済みの値を残す。
 */
export function settingChanges(
  settings: readonly WorkInfoPluginSetting[],
  drafts: Readonly<Record<string, SettingDraft>>,
): Record<string, string | null> {
  const changes: Record<string, string | null> = {};
  for (const setting of settings) {
    const change = changeOf(setting, drafts[setting.key]);
    if (change !== undefined) changes[setting.key] = change;
  }
  return changes;
}
