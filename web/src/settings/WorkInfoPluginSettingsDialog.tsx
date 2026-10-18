import { CircleAlert } from "lucide-react";
import { useId, useState } from "react";

import type { WorkInfoPlugin, WorkInfoPluginSetting } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "../ui/shadcn/field";
import { limitCodePoints } from "../lib/codePointLimit";
import { Input } from "../ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import { cn } from "../lib/cn";
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
  disabled,
  onChange,
}: {
  setting: WorkInfoPluginSetting;
  draft: SettingDraft;
  hintId: string;
  disabled: boolean;
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
          disabled={disabled}
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
        disabled={disabled}
        onClick={() => onChange({ value: "", forgotten: true })}
      >
        {text.forget}
      </Button>
    </div>
  );
}

/** Index-based option values cannot collide with the first item's sentinel. */
const firstItem = "default";

function optionItem(index: number): string {
  return `option-${index}`;
}

function firstItemText(setting: WorkInfoPluginSetting): string {
  const text = t.settings.workInfoPlugins.settingsDialog;
  return setting.default === undefined
    ? text.notSetItem
    : text.defaultItem(setting.default);
}

function isStale(setting: WorkInfoPluginSetting, draft: SettingDraft): boolean {
  return (
    setting.staleValue !== undefined &&
    draft.value !== "" &&
    !(setting.options ?? []).includes(draft.value)
  );
}

/** The first option saves null for default or Not set; out-of-options stored values select nothing. */
function OptionSelect({
  id,
  setting,
  options,
  draft,
  disabled,
  describedBy,
  onChange,
}: {
  id: string;
  setting: WorkInfoPluginSetting;
  options: readonly string[];
  draft: SettingDraft;
  disabled: boolean;
  describedBy: string | undefined;
  onChange: (draft: SettingDraft) => void;
}) {
  const index = options.indexOf(draft.value);
  const following = draft.value === "";
  const value = following ? firstItem : index >= 0 ? optionItem(index) : "";
  const first = firstItemText(setting);
  return (
    <Select
      value={value}
      disabled={disabled}
      onValueChange={(next) =>
        onChange({
          value:
            next === firstItem
              ? ""
              : (options[Number(next.slice("option-".length))] ?? ""),
          forgotten: false,
        })
      }
    >
      <SelectTrigger
        id={id}
        className={cn("w-full", following && "text-muted-foreground")}
        title={index >= 0 ? options[index] : first}
        aria-describedby={describedBy}
      >
        <SelectValue placeholder={first} />
      </SelectTrigger>
      <SelectContent className="max-w-popover">
        <SelectItem
          value={firstItem}
          className="text-muted-foreground *:[span]:last:min-w-0"
        >
          <span className="wrap-anywhere">{first}</span>
        </SelectItem>
        {options.map((option, position) => (
          <SelectItem
            key={option}
            value={optionItem(position)}
            className="*:[span]:last:min-w-0"
          >
            <span className="wrap-anywhere">{option}</span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Show defaults as placeholders, never as entered values. */
function TextInput({
  id,
  setting,
  draft,
  disabled,
  describedBy,
  onChange,
}: {
  id: string;
  setting: WorkInfoPluginSetting;
  draft: SettingDraft;
  disabled: boolean;
  describedBy: string | undefined;
  onChange: (draft: SettingDraft) => void;
}) {
  return (
    <Input
      id={id}
      type={setting.secret ? "password" : "text"}
      autoComplete="off"
      value={draft.value}
      placeholder={setting.default}
      disabled={disabled || draft.forgotten}
      aria-describedby={describedBy}
      onChange={(event) =>
        onChange({
          value: limitCodePoints(
            event.target.value,
            MAX_SETTING_LENGTH,
            draft.value,
            event.target.selectionEnd ?? undefined,
          ),
          forgotten: false,
        })
      }
    />
  );
}

function StaleNote({ setting, id }: { setting: WorkInfoPluginSetting; id: string }) {
  const text = t.settings.workInfoPlugins.settingsDialog;
  const value = setting.staleValue ?? "";
  return (
    <p id={id} className="flex items-start gap-1 text-xs text-warning">
      <CircleAlert aria-hidden="true" className="size-4 shrink-0" />
      <span className="min-w-0 wrap-anywhere">
        {setting.default === undefined
          ? text.staleNoDefault(value)
          : text.staleWithDefault(value)}
      </span>
    </p>
  );
}

/**
 * Use server-compatible code-point limits, not HTML maxLength. Disable fields and helper actions
 * during saving to avoid closing over unsaved edits.
 */
function SettingField({
  setting,
  draft,
  disabled,
  onChange,
}: {
  setting: WorkInfoPluginSetting;
  draft: SettingDraft;
  disabled: boolean;
  onChange: (draft: SettingDraft) => void;
}) {
  const text = t.settings.workInfoPlugins;
  const inputId = useId();
  const hintId = `${inputId}-hint`;
  const staleId = `${inputId}-stale`;
  const stale = isStale(setting, draft);
  const hasDefault = !setting.secret && setting.default !== undefined;
  const describedBy =
    [setting.secret || hasDefault ? hintId : "", stale ? staleId : ""]
      .filter((id) => id !== "")
      .join(" ") || undefined;
  return (
    <Field>
      <FieldLabel htmlFor={inputId}>
        <span className="break-words">{setting.label}</span>
        {setting.required && (
          <span className="font-normal text-muted-foreground">
            {text.settingsDialog.required}
          </span>
        )}
      </FieldLabel>
      {setting.options === undefined ? (
        <TextInput
          id={inputId}
          setting={setting}
          draft={draft}
          disabled={disabled}
          describedBy={describedBy}
          onChange={onChange}
        />
      ) : (
        <OptionSelect
          id={inputId}
          setting={setting}
          options={setting.options}
          draft={draft}
          disabled={disabled}
          describedBy={describedBy}
          onChange={onChange}
        />
      )}
      {hasDefault && (
        <FieldDescription id={hintId} className="wrap-anywhere">
          {text.defaultHint(setting.default ?? "")}
        </FieldDescription>
      )}
      {stale && <StaleNote setting={setting} id={staleId} />}
      {setting.secret && (
        <SecretHint
          setting={setting}
          draft={draft}
          hintId={hintId}
          disabled={disabled}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

/**
 * Submit only changed keys, using null to clear or restore defaults; secret fields always start
 * empty.
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
          disabled={pending}
          onChange={(draft) =>
            setDrafts((current) => ({ ...current, [setting.key]: draft }))
          }
        />
      ))}
    </WorkInfoPluginDialog>
  );
}
