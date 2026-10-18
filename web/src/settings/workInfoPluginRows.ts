import type {
  WorkInfoPlugin,
  WorkInfoPluginRunFailure,
  WorkInfoPluginSetting,
} from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";

//

export const MAX_PLUGIN_BYTES = 16 * 1024 * 1024;

/** Match server code-point limits. */
export const MAX_SETTING_LENGTH = 2000;

export function pluginName(plugin: Pick<WorkInfoPlugin, "name" | "fileName">): string {
  return plugin.name ?? plugin.fileName;
}

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

/** Count defaults as default and out-of-options stored values as not set. */
export function settingsSummary(plugin: Pick<WorkInfoPlugin, "settings">): UiText | null {
  if (plugin.settings.length === 0) return null;
  return t.settings.workInfoPlugins.settingsSummary(plugin.settings.map(summaryOf));
}

/** Out-of-options values are unavailable; defaults satisfy required settings. */
export function missingRequired(plugin: Pick<WorkInfoPlugin, "settings">): string[] {
  return plugin.settings
    .filter(
      (setting) =>
        setting.required &&
        setting.default === undefined &&
        (!setting.set || setting.staleValue !== undefined),
    )
    .map((setting) => setting.label);
}

export function staleSettings(plugin: Pick<WorkInfoPlugin, "settings">): string[] {
  return plugin.settings
    .filter((setting) => setting.staleValue !== undefined)
    .map((setting) => setting.label);
}

/**
 * Secret drafts contain only new input; forgotten marks explicit deletion. Empty selects
 * default/Not set, while stale option values remain visible for correction.
 */
export interface SettingDraft {
  value: string;
  forgotten: boolean;
}

/** Secret drafts always start empty. */
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

/** Send changed keys only; null clears a value, while an empty secret input preserves it. */
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

export function unixDate(seconds: number): Date {
  return new Date(seconds * 1000);
}

/** Run records do not retain hosts, so blocked-address and network errors use hostless wording. */
export function runFailureText(failure: WorkInfoPluginRunFailure): UiText {
  const { code, reason } = t.errors;
  switch (failure) {
    case "plugin_failed":
      return code.plugin_failed;
    case "timed_out":
      return code.plugin_timed_out;
    case "memory_exceeded":
      return reason.memory_exceeded;
    case "output_too_large":
      return reason.output_too_large;
    case "blocked_address":
      return reason.blocked_address({});
    case "network_error":
      return reason.network_error({});
  }
}
