import { useState, type ClipboardEvent, type FormEvent } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { newlinePattern } from "../lib/tagName";
import type { NameRules } from "../ui/NameCommand";
import { tagNameRules } from "../ui/TagCommand";

/** Name conflicts are inline validation; other failures use alert semantics. */
export type TagFieldError = { kind: "taken" | "other"; message: UiText };

export interface TagFieldContext {
  submitted?: string;
  ownTagName?: string;
}

function takenText(failure: RequestFailed, context: TagFieldContext): UiText {
  const owner = failure.tagName;
  if (owner === undefined) return errorText(failure);
  const own = context.ownTagName !== undefined && owner === context.ownTagName;
  if (failure.reason === "name_is_tag") {
    return own ? t.tags.nameIsOwnName(owner) : t.tags.nameIsTag(owner);
  }
  if (failure.reason === "name_is_synonym" && context.submitted !== undefined) {
    return own
      ? t.tags.nameIsOwnSynonym(context.submitted)
      : t.tags.nameIsSynonym(context.submitted, owner);
  }
  return errorText(failure);
}

/** Classify name conflicts separately and never display raw server messages. */
export function tagFieldError(
  failure: unknown,
  context: TagFieldContext = {},
): TagFieldError {
  if (failure instanceof RequestFailed && failure.code === "tag_name_taken") {
    return { kind: "taken", message: takenText(failure, context) };
  }
  return { kind: "other", message: errorText(failure) };
}

const kindNameReasons: ReadonlySet<string> = new Set([
  "tag_kind_name_taken",
  "tag_name_empty",
  "tag_name_control_characters",
  "tag_name_too_long",
]);

export function kindFieldError(failure: unknown): TagFieldError {
  const byName =
    failure instanceof RequestFailed &&
    failure.reason !== undefined &&
    kindNameReasons.has(failure.reason);
  return { kind: byName ? "taken" : "other", message: errorText(failure) };
}

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
