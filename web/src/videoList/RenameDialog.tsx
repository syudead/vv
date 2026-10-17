import { CircleAlert } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";

/** RenameDialogTexts は名前を変える窓の文言である。 */
export interface RenameDialogTexts {
  title: UiText;
  submit: UiText;
  submitting: UiText;
  failed: (reason: string) => UiText;
  nameLabel: UiText;
  nameRule: UiText;
}

/**
 * RenameDialog はプレイリストやグループの名前を変える窓である。
 * 今の名前を入れて選んだ状態で開く。`normalize` は送る名前（前後を削ったもの）を返し、規則に
 * 合わなければ null を返す。削った名前が今の名前と同じ間は「Save」を押せない。
 *
 * `rename` が値を返せば `onRenamed`、`404` なら `onGone` を呼ぶ。`400` は名前の規則を欄の誤りと
 * して出し、ほかの失敗は窓の中の `Alert` に出す。閉じるのは呼び出し元に任せる。送信中は閉じない。
 */
export default function RenameDialog<T>({
  currentName,
  texts,
  normalize,
  rename,
  onClose,
  onRenamed,
  onGone,
}: {
  currentName: string;
  texts: RenameDialogTexts;
  normalize: (text: string) => string | null;
  rename: (name: string) => Promise<T>;
  onClose: () => void;
  onRenamed: (renamed: T) => void;
  onGone: () => void;
}) {
  const nameId = useId();
  const errorId = useId();
  const input = useRef<HTMLInputElement>(null);
  const selected = useRef(false);
  const [text, setText] = useState(currentName);
  const [invalid, setInvalid] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);
  const unchanged = normalize(text) === currentName;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || unchanged) return;
    const name = normalize(text);
    if (name === null) {
      setInvalid(true);
      setFailure(null);
      input.current?.focus();
      return;
    }
    setPending(true);
    setFailure(null);
    rename(name).then(
      (renamed) => {
        setPending(false);
        onRenamed(renamed);
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 404) {
          onGone();
          return;
        }
        if (error instanceof RequestFailed && error.status === 400) {
          setInvalid(true);
          input.current?.focus();
          return;
        }
        setFailure(texts.failed(errorText(error)));
      },
    );
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={texts.title}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? texts.submitting : texts.submit}
        </>
      }
      pending={pending}
      submitDisabled={unchanged}
      initialFocus={input}
    >
      <Field data-invalid={invalid || undefined}>
        <FieldLabel htmlFor={nameId}>{texts.nameLabel}</FieldLabel>
        <Input
          ref={input}
          id={nameId}
          type="text"
          autoComplete="off"
          value={text}
          onFocus={(event) => {
            // 開いて最初にフォーカスを受けたときだけ、今の名前を選ぶ。
            if (selected.current) return;
            selected.current = true;
            event.currentTarget.select();
          }}
          onChange={(event) => {
            setText(event.target.value);
            setInvalid(false);
          }}
          aria-invalid={invalid || undefined}
          aria-describedby={invalid ? errorId : undefined}
          className="h-8"
        />
        {invalid && <FieldError id={errorId}>{texts.nameRule}</FieldError>}
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
