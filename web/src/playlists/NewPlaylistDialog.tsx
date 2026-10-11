import { CircleAlert } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { createPlaylist, type PlaylistSummary } from "../api/playlists";
import { errorText, t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { playlistName } from "./playlistName";

/**
 * NewPlaylistDialog は名前を 1 つ聞いて空のプレイリストを作る窓である
 * （specs/044-playlists/ui-design.md「New playlist」）。呼び出し元は開いている間だけ描き、
 * 閉じるたびに入れた名前を捨てる。
 *
 * 名前は送る前に数え、空か 100 文字を超えるなら送らずに規則を欄の誤りとして出す（入れて
 * いる間は出さない）。`201` で `onCreated` を呼び、閉じるのと新しい行は呼び出し元に任せる。
 * 送信中は閉じない。
 */
export default function NewPlaylistDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (playlist: PlaylistSummary) => void;
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
    createPlaylist(name).then(
      (playlist) => {
        setPending(false);
        onCreated(playlist);
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 400) {
          setInvalid(true);
          input.current?.focus();
          return;
        }
        setFailure(t.playlists.newDialog.failed(errorText(error)));
      },
    );
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={t.playlists.newDialog.title}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.playlists.newDialog.submitting : t.playlists.newDialog.submit}
        </>
      }
      pending={pending}
      initialFocus={input}
    >
      <Field data-invalid={invalid || undefined}>
        <FieldLabel htmlFor={nameId}>{t.playlists.nameLabel}</FieldLabel>
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
        {invalid && <FieldError id={errorId}>{t.playlists.nameRule}</FieldError>}
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
