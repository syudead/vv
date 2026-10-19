import { CircleAlert } from "lucide-react";
import { type FormEvent, type ReactNode, useId, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { playlistName } from "../playlists/playlistName";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";

export interface NameDialogWords {
  title: UiText;
  submit: UiText;
  submitting: UiText;
  failed: (reason: string) => UiText;
}

/** Mount only while open so drafts reset on close; validate on submit and block closing while pending. */
export default function NameDialog<T>({
  words,
  description,
  nameLabel,
  nameRule,
  validate,
  send,
  onClose,
  onDone,
  onFailed,
}: {
  words: NameDialogWords;
  description?: ReactNode;
  nameLabel: UiText;
  nameRule: UiText;
  /** Replaces the playlist name rule: why a name is refused, or null; a server refusal then shows its own reason. */
  validate?: (text: string) => UiText | null;
  send: (name: string) => Promise<T>;
  onClose: () => void;
  onDone: (result: T) => void;
  onFailed?: (error: unknown) => void;
}) {
  const nameId = useId();
  const errorId = useId();
  const input = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [invalid, setInvalid] = useState<UiText | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const refusal = validate === undefined ? null : validate(text);
    const name =
      validate === undefined ? playlistName(text) : refusal === null ? text.trim() : null;
    if (name === null) {
      setInvalid(refusal ?? nameRule);
      setFailure(null);
      input.current?.focus();
      return;
    }
    setPending(true);
    setFailure(null);
    send(name).then(
      (result) => {
        setPending(false);
        onDone(result);
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 400) {
          setInvalid(validate === undefined ? nameRule : errorText(error));
          input.current?.focus();
          return;
        }
        setFailure(words.failed(errorText(error)));
        onFailed?.(error);
      },
    );
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={words.title}
      description={description}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? words.submitting : words.submit}
        </>
      }
      pending={pending}
      initialFocus={input}
    >
      <Field data-invalid={invalid !== null || undefined}>
        <FieldLabel htmlFor={nameId}>{nameLabel}</FieldLabel>
        <Input
          ref={input}
          id={nameId}
          type="text"
          autoComplete="off"
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            setInvalid(null);
          }}
          aria-invalid={invalid !== null || undefined}
          aria-describedby={invalid !== null ? errorId : undefined}
          className="h-8"
        />
        {invalid !== null && <FieldError id={errorId}>{invalid}</FieldError>}
      </Field>
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}
