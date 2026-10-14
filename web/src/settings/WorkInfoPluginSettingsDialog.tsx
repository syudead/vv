import { useId, useState } from "react";

import type { WorkInfoPlugin, WorkInfoPluginSetting } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import {
  draftsOf,
  MAX_SETTING_LENGTH,
  pluginName,
  type SettingDraft,
  settingChanges,
} from "./workInfoPluginRows";
import WorkInfoPluginDialog from "./WorkInfoPluginDialog";

function SecretHint({
  setting,
  draft,
  hintId,
  onChange,
}: {
  setting: WorkInfoPluginSetting;
  draft: SettingDraft;
  hintId: string;
  onChange: (draft: SettingDraft) => void;
}) {
  const text = t.settings.workInfoPlugins.settingsDialog;
  if (!setting.set)
    return <FieldDescription id={hintId}>{text.secretNotSaved}</FieldDescription>;
  if (draft.forgotten)
    return (
      <div className="flex flex-wrap items-center gap-2">
        <FieldDescription id={hintId}>{text.forgotten}</FieldDescription>
        <Button
          type="button"
          variant="link"
          size="sm"
          className="h-auto p-0"
          onClick={() => onChange({ value: "", forgotten: false })}
        >
          {text.undo}
        </Button>
      </div>
    );
  return (
    <div className="flex flex-wrap items-center gap-2">
      <FieldDescription id={hintId}>{text.secretSaved}</FieldDescription>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() => onChange({ value: "", forgotten: true })}
      >
        {text.forget}
      </Button>
    </div>
  );
}

function SettingField({
  setting,
  draft,
  onChange,
}: {
  setting: WorkInfoPluginSetting;
  draft: SettingDraft;
  onChange: (draft: SettingDraft) => void;
}) {
  const text = t.settings.workInfoPlugins.settingsDialog;
  const inputId = useId();
  const hintId = `${inputId}-hint`;
  return (
    <Field>
      <FieldLabel htmlFor={inputId}>
        <span className="break-words">{setting.label}</span>
        {setting.required && (
          <span className="font-normal text-muted-foreground">{text.required}</span>
        )}
      </FieldLabel>
      <Input
        id={inputId}
        type={setting.secret ? "password" : "text"}
        autoComplete="off"
        maxLength={MAX_SETTING_LENGTH}
        value={draft.value}
        disabled={draft.forgotten}
        aria-describedby={setting.secret ? hintId : undefined}
        onChange={(event) => onChange({ value: event.target.value, forgotten: false })}
      />
      {setting.secret && (
        <SecretHint setting={setting} draft={draft} hintId={hintId} onChange={onChange} />
      )}
    </Field>
  );
}

/**
 * WorkInfoPluginSettingsDialog は「Settings」が開く設定の値のダイアログである。宣言の順に欄を
 * 並べ、「Save」は変わったキーだけを送る（消す値は null）。秘密の欄は常に空で始まり、保存済みの
 * 値を欄に戻さない（specs/049-work-info-plugins/ui-design.md「Settings dialog」）。
 */
export default function WorkInfoPluginSettingsDialog({
  plugin,
  pending,
  error,
  onClose,
  onSave,
}: {
  plugin: WorkInfoPlugin;
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onSave: (values: Record<string, string | null>) => void;
}) {
  const text = t.settings.workInfoPlugins.settingsDialog;
  const [drafts, setDrafts] = useState(() => draftsOf(plugin.settings));
  const changes = settingChanges(plugin.settings, drafts);
  const changed = Object.keys(changes).length > 0;

  return (
    <WorkInfoPluginDialog
      title={text.title(pluginName(plugin))}
      description={text.description}
      submitLabel={text.submit}
      pending={pending}
      submitDisabled={!changed}
      failure={error}
      onClose={onClose}
      onSubmit={() => onSave(changes)}
    >
      {plugin.settings.map((setting) => (
        <SettingField
          key={setting.key}
          setting={setting}
          draft={drafts[setting.key] ?? { value: "", forgotten: false }}
          onChange={(draft) =>
            setDrafts((current) => ({ ...current, [setting.key]: draft }))
          }
        />
      ))}
    </WorkInfoPluginDialog>
  );
}
