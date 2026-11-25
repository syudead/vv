import { useState, type ClipboardEvent, type FormEvent } from "react";

import type { UiText } from "../i18n";
import { newlinePattern } from "../lib/tagName";
import type { NameRules } from "./NameCommand";
import { tagNameRules } from "./TagCommand";

/** Name conflicts are inline validation; other failures use alert semantics. */
export type TagFieldError = { kind: "taken" | "other"; message: UiText };

/** Share name validation and multiline-input rejection across editors. Keep reasons until the value changes, then clear prior submission errors. */
export function useTagNameField(
  initial = "",
  onChange?: () => void,
  rules: NameRules = tagNameRules,
) {
  const [value, setValueState] = useState(initial);
  const [reason, setReason] = useState<UiText | null>(rules.reason(initial));

  function setValue(next: string): void {
    setValueState(next);
    setReason(rules.reason(next));
    onChange?.();
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>): void {
    const text = event.clipboardData.getData("text");
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason(rules.controlCharacters);
    }
  }

  function onBeforeInput(event: FormEvent<HTMLInputElement>): void {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    const text = native.data ?? "";
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason(rules.controlCharacters);
    }
  }

  function trySpelling(): string | null {
    if (reason !== null) return null;
    const trimmed = value.trim();
    if (trimmed === "") {
      setReason(rules.required);
      return null;
    }
    return trimmed;
  }

  return { value, setValue, reason, setReason, onPaste, onBeforeInput, trySpelling };
}
