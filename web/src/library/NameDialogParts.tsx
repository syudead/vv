import { CircleAlert } from "lucide-react";
import { type FormEvent, type ReactNode, type RefObject, useId } from "react";

import { t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";

export interface NameDialogWords {
  title: UiText;
  submit: UiText;
  submitting: UiText;
  failed: (reason: string) => UiText;
}

/** A create dialog's frame: it cannot close while pending, and a request failure shows under the fields. */
export function CreateDialogFrame({
  words,
  description,
  pending,
  failure,
  initialFocus,
  submitDisabled,
  onSubmit,
  onClose,
  children,
}: {
  words: NameDialogWords;
  description?: ReactNode;
  pending: boolean;
  failure: UiText | null;
  initialFocus: RefObject<HTMLElement | null>;
  submitDisabled?: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={words.title}
      description={description}
      onSubmit={onSubmit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? words.submitting : words.submit}
        </>
      }
      pending={pending}
      submitDisabled={submitDisabled}
      initialFocus={initialFocus}
    >
      {children}
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}

/** One text field whose refusal replaces its hint; editing clears the refusal through onChange. */
export function DialogTextField({
  label,
  value,
  invalid,
  hint,
  inputRef,
  inputMode,
  onChange,
}: {
  label: UiText;
  value: string;
  invalid: UiText | null;
  hint?: UiText;
  inputRef: RefObject<HTMLInputElement | null>;
  inputMode?: "url";
  onChange: (value: string) => void;
}) {
  const inputId = useId();
  const noteId = useId();
  const hasNote = invalid !== null || hint !== undefined;
  return (
    <Field data-invalid={invalid !== null || undefined}>
      <FieldLabel htmlFor={inputId}>{label}</FieldLabel>
      <Input
        ref={inputRef}
        id={inputId}
        type="text"
        inputMode={inputMode}
        autoComplete="off"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        aria-invalid={invalid !== null || undefined}
        aria-describedby={hasNote ? noteId : undefined}
        className="h-8"
      />
      {invalid !== null ? (
        <FieldError id={noteId}>{invalid}</FieldError>
      ) : (
        hint !== undefined && <FieldDescription id={noteId}>{hint}</FieldDescription>
      )}
    </Field>
  );
}
