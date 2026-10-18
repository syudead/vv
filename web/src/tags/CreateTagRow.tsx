import { useEffect, useRef } from "react";

import { t, type UiText } from "../i18n";
import { isComposingKeyEvent } from "../lib/ime";
import { Button } from "../ui/shadcn/button";
import { Field, FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { TableCell, TableRow } from "../ui/shadcn/table";
import type { NameRules } from "../ui/NameCommand";
import { useTagNameField, type TagFieldError } from "./tagNameField";

export default function CreateTagRow({
  columnCount,
  pending,
  error,
  onCancel,
  onSubmit,
  onDraftChange,
  rules,
  label = t.tags.create.label,
  placeholder = t.tags.create.placeholder,
}: {
  columnCount: number;
  pending: boolean;
  error: TagFieldError | null;
  onCancel: () => void;
  onSubmit: (name: string) => void;
  onDraftChange?: () => void;
  /** Defaults to tag-name rules. */
  rules?: NameRules;
  label?: UiText;
  placeholder?: UiText;
}) {
  const field = useTagNameField("", onDraftChange, rules);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling !== null) onSubmit(spelling);
  }

  function cancel() {
    // Keep pending rows open so late responses cannot update removed or replacement rows.
    if (pending) return;
    onCancel();
  }

  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={columnCount} className="whitespace-normal">
        <Field data-invalid={field.reason !== null || error !== null || undefined}>
          <div className="flex flex-wrap items-center gap-2">
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
              placeholder={placeholder}
              aria-label={label}
              aria-describedby={
                field.reason !== null
                  ? "tag-create-reason"
                  : error !== null
                    ? "tag-create-error"
                    : undefined
              }
              aria-busy={pending || undefined}
              aria-invalid={field.reason !== null || error?.kind === "taken" || undefined}
              className="h-8 basis-full sm:flex-1 sm:basis-auto"
            />
            <div className="flex w-full items-center justify-end gap-2 sm:w-auto">
              <Button size="sm" onClick={submit} disabled={pending}>
                {pending && <Spinner aria-hidden="true" />}
                {pending ? t.tags.create.submitting : t.tags.create.submit}
              </Button>
              <Button variant="outline" size="sm" onClick={cancel} disabled={pending}>
                {t.common.cancel}
              </Button>
            </div>
          </div>
          {field.reason !== null && (
            <FieldError id="tag-create-reason" role={undefined}>
              {field.reason}
            </FieldError>
          )}
          {field.reason === null && error !== null && error.kind === "taken" && (
            <FieldError id="tag-create-error" role={undefined}>
              {error.message}
            </FieldError>
          )}
          {field.reason === null && error !== null && error.kind === "other" && (
            <FieldError id="tag-create-error" className="text-sm">
              {error.message}
            </FieldError>
          )}
        </Field>
      </TableCell>
    </TableRow>
  );
}
