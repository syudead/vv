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

/** NameDialogWords は名前を聞く窓の文言である。 */
export interface NameDialogWords {
  title: UiText;
  submit: UiText;
  submitting: UiText;
  failed: (reason: string) => UiText;
}

/**
 * NameDialog は名前を 1 つ聞いて送る窓である。呼び出し元は開いている間だけ描き、閉じるたびに
 * 入れた名前を捨てる。プレイリストとグループの名前は同じ規則である。
 *
 * 名前は送る前に数え、空か 100 文字を超えるなら送らずに規則を欄の誤りとして出す（入れて
 * いる間は出さない）。送れたら `onDone` を呼び、閉じるのは呼び出し元に任せる。`400` は名前の
 * 誤りとして欄に、ほかの失敗は窓の中の `Alert` に出し、`onFailed` にも渡す。送信中は閉じない。
 */
export default function NameDialog<T>({
  words,
  description,
  nameLabel,
  nameRule,
  send,
  onClose,
  onDone,
  onFailed,
}: {
  words: NameDialogWords;
  description?: ReactNode;
  nameLabel: UiText;
  nameRule: UiText;
  send: (name: string) => Promise<T>;
  onClose: () => void;
  onDone: (result: T) => void;
  onFailed?: (error: unknown) => void;
}) {
  const nameId = useId();
  const errorId = useId();
  const input = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [invalid, setInvalid] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const name = playlistName(text);
    if (name === null) {
      setInvalid(true);
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
          setInvalid(true);
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
      <Field data-invalid={invalid || undefined}>
        <FieldLabel htmlFor={nameId}>{nameLabel}</FieldLabel>
        <Input
          ref={input}
          id={nameId}
          type="text"
          autoComplete="off"
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            setInvalid(false);
          }}
          aria-invalid={invalid || undefined}
          aria-describedby={invalid ? errorId : undefined}
          className="h-8"
        />
        {invalid && <FieldError id={errorId}>{nameRule}</FieldError>}
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
