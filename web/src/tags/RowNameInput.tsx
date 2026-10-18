import { useEffect, useRef } from "react";

import type { UiText } from "../i18n";
import { isComposingKeyEvent } from "../lib/ime";
import type { NameRules } from "../ui/NameCommand";
import { Input } from "../ui/shadcn/input";
import { useTagNameField, type TagFieldError } from "./tagNameField";

/** Keep the rename input open while saving; distinguish inline validation from alert-level failures. */
export default function RowNameInput({
  idPrefix,
  rowId,
  name,
  rules,
  label,
  pending,
  error,
  onSubmit,
  onCancel,
  onDraftChange,
}: {
  idPrefix: string;
  rowId: number;
  name: string;
  rules?: NameRules;
  label: UiText;
  pending: boolean;
  error: TagFieldError | null;
  onSubmit: (name: string) => void;
  onCancel: () => void;
  onDraftChange?: () => void;
}) {
  const field = useTagNameField(name, onDraftChange, rules);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling !== null) onSubmit(spelling);
  }

  function cancel() {
    // Block Escape while pending so late responses cannot affect removed or replacement rows.
    if (pending) return;
    onCancel();
  }

  const reasonId = `${idPrefix}-reason-${String(rowId)}`;
  const errorId = `${idPrefix}-error-${String(rowId)}`;

  return (
    <>
      <Input
        ref={inputRef}
        value={field.value}
        onChange={(event) => field.setValue(event.target.value)}
        onPaste={field.onPaste}
        onBeforeInput={field.onBeforeInput}
        onKeyDown={(event) => {
          if (isComposingKeyEvent(event)) return;
          if (event.key === "Enter") {
            event.preventDefault();
            submit();
          } else if (event.key === "Escape") {
            event.preventDefault();
            cancel();
          }
        }}
        aria-label={label}
        aria-describedby={
          field.reason !== null ? reasonId : error !== null ? errorId : undefined
        }
        aria-busy={pending || undefined}
        aria-invalid={field.reason !== null || error?.kind === "taken" || undefined}
        className="h-8"
      />
      {field.reason !== null && (
        <p id={reasonId} className="mt-1 text-xs text-destructive">
          {field.reason}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "taken" && (
        <p id={errorId} className="mt-1 text-xs text-destructive">
          {error.message}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "other" && (
        <p id={errorId} role="alert" className="mt-1 text-sm text-destructive">
          {error.message}
        </p>
      )}
    </>
  );
}
