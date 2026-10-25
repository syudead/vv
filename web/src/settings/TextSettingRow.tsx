import { CircleCheck } from "lucide-react";
import { useRef, useState } from "react";

import { errorText, type UiText } from "../i18n";
import { FormRow } from "../ui/patterns/form-row";
import { FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";

import { SaveStatus } from "./SaveStatus";

export interface TextSettingSave {
  value: string;
  saving: boolean;
  saved: boolean;
  invalid: boolean;
  saveError: UiText | null;
  edit: (next: string) => void;
  commit: () => void;
}

export interface TextSettingOptions {
  initial: string;
  /** Resolves to the value the server stored, which replaces the input. */
  save: (value: string) => Promise<string>;
  /** Refuses a value before it is sent. */
  isValid?: (value: string) => boolean;
  /** A failure that means the value itself was refused, shown under the input. */
  isInvalid: (failure: unknown) => boolean;
  failed: (reason: UiText) => UiText;
}

/**
 * Serialize saves and submit the latest draft after an in-flight commit. Apply normalized
 * responses and errors only while the input still matches the submitted value, preserving
 * concurrent edits.
 */
export function useTextSettingSave({
  initial,
  save,
  isValid = () => true,
  isInvalid,
  failed,
}: TextSettingOptions): TextSettingSave {
  const [value, setValue] = useState(initial);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  // Async saves need the current input independently of render closures.
  const valueRef = useRef(initial);
  const savedValue = useRef(initial);
  const inFlight = useRef(false);
  const pending = useRef(false);

  const showFailure = (failure: unknown) => {
    if (isInvalid(failure)) setInvalid(true);
    else setSaveError(failed(errorText(failure)));
  };

  const send = async (submitted: string) => {
    inFlight.current = true;
    setSaving(true);
    setSaved(false);
    setSaveError(null);
    const unchanged = () => valueRef.current.trim() === submitted;
    try {
      const stored = await save(submitted);
      savedValue.current = stored;
      if (unchanged()) {
        valueRef.current = stored;
        setValue(stored);
        setSaved(true);
      }
    } catch (failure) {
      // Do not show errors for an obsolete submitted value after the input changes.
      if (unchanged()) showFailure(failure);
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
    if (pending.current) {
      pending.current = false;
      commit();
    }
  };

  function commit() {
    if (inFlight.current) {
      pending.current = true;
      return;
    }
    const next = valueRef.current.trim();
    if (next === savedValue.current) return;
    if (!isValid(next)) {
      setInvalid(true);
      return;
    }
    void send(next);
  }

  const edit = (next: string) => {
    valueRef.current = next;
    setValue(next);
    setInvalid(false);
    setSaved(false);
    setSaveError(null);
  };

  return { value, saving, saved, invalid, saveError, edit, commit };
}

export interface TextSettingRowProps {
  id: string;
  label: UiText;
  description: UiText;
  placeholder?: UiText;
  invalidText: UiText;
  savingText: UiText;
  savedText: UiText;
  /** Marks the input aria-disabled while a save is in flight. */
  busyWhileSaving?: boolean;
  setting: TextSettingSave;
}

/** Commit changed values on blur or Enter; there is no Save button. */
export function TextSettingRow({
  id,
  label,
  description,
  placeholder,
  invalidText,
  savingText,
  savedText,
  busyWhileSaving = false,
  setting,
}: TextSettingRowProps) {
  const descriptionId = `${id}-description`;
  const errorId = `${id}-error`;
  return (
    <>
      <FormRow
        label={label}
        description={description}
        descriptionId={descriptionId}
        htmlFor={id}
      >
        <div className="flex w-full flex-col gap-2 sm:w-sm">
          <Input
            id={id}
            type="url"
            autoComplete="off"
            spellCheck={false}
            placeholder={placeholder}
            value={setting.value}
            onChange={(event) => setting.edit(event.target.value)}
            onBlur={setting.commit}
            onKeyDown={(event) => {
              if (event.key !== "Enter") return;
              event.preventDefault();
              setting.commit();
            }}
            aria-disabled={(busyWhileSaving && setting.saving) || undefined}
            aria-invalid={setting.invalid || undefined}
            aria-describedby={
              setting.invalid ? `${descriptionId} ${errorId}` : descriptionId
            }
          />
          {setting.invalid && <FieldError id={errorId}>{invalidText}</FieldError>}
        </div>
      </FormRow>
      <SaveStatus
        saving={setting.saving}
        savingText={savingText}
        error={setting.saveError}
      />
      {setting.saved && (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <CircleCheck aria-hidden="true" className="size-4" />
          {savedText}
        </p>
      )}
    </>
  );
}
