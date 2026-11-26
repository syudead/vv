import { type FormEvent, useId, useRef, useState } from "react";

import { createTag, type Tag } from "../api/tags";
import { t } from "../i18n";
import { useTagNameField, type TagFieldError } from "../ui/nameField";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { tagFieldError } from "./tagNameField";

/** Create a tag by name; the caller opens the new tag, since a tag sorted by count can land out of view. */
export default function NewTagDialog({
  track,
  onClose,
  onCreated,
}: {
  /** Runs the write as one of the list's own, so a change check waits for it. */
  track: <T>(write: Promise<T>) => Promise<T>;
  onClose: () => void;
  onCreated: (tag: Tag) => void;
}) {
  const nameId = useId();
  const errorId = useId();
  const input = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<TagFieldError | null>(null);
  const [pending, setPending] = useState(false);
  const field = useTagNameField("", () => setError(null));
  const message = field.reason ?? error?.message ?? null;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const name = field.trySpelling();
    if (name === null) {
      input.current?.focus();
      return;
    }
    setPending(true);
    setError(null);
    track(createTag(name)).then(
      (created) => {
        setPending(false);
        onCreated(created);
      },
      (failure: unknown) => {
        setPending(false);
        setError(tagFieldError(failure, { submitted: name }));
        input.current?.focus();
      },
    );
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={t.tags.newTag}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.tags.create.submitting : t.tags.create.submit}
        </>
      }
      pending={pending}
      initialFocus={input}
    >
      <Field data-invalid={message !== null || undefined}>
        <FieldLabel htmlFor={nameId}>{t.tags.create.label}</FieldLabel>
        <Input
          ref={input}
          id={nameId}
          type="text"
          autoComplete="off"
          placeholder={t.tags.create.placeholder}
          value={field.value}
          onChange={(event) => field.setValue(event.target.value)}
          onPaste={field.onPaste}
          onBeforeInput={field.onBeforeInput}
          aria-invalid={message !== null || undefined}
          aria-describedby={message === null ? undefined : errorId}
          className="h-8"
        />
        {message !== null && <FieldError id={errorId}>{message}</FieldError>}
      </Field>
    </FormDialog>
  );
}
