import { CircleAlert } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { renamePlaylist, type PlaylistSummary } from "../api/playlists";
import { errorText, t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { playlistName } from "./playlistName";

/**
 * RenamePlaylistDialog はプレイリストの名前を変える窓である
 * （specs/044-playlists/ui-design.md「Renaming」）。名前の欄と規則は新規作成と同じで、
 * 今の名前を入れて選んだ状態で開く。トリムした名前が今の名前と同じ間は「Save」を押せない。
 *
 * `200` で `onRenamed`、`404` で `onGone` を呼ぶ。閉じるのは呼び出し元に任せる。送信中は閉じない。
 */
export default function RenamePlaylistDialog({
  playlistId,
  currentName,
  onClose,
  onRenamed,
  onGone,
}: {
  playlistId: number;
  currentName: string;
  onClose: () => void;
  onRenamed: (playlist: PlaylistSummary) => void;
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
  const unchanged = playlistName(text) === currentName;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || unchanged) return;
    const name = playlistName(text);
    if (name === null) {
      setInvalid(true);
      setFailure(null);
      input.current?.focus();
      return;
    }
    setPending(true);
    setFailure(null);
    renamePlaylist(playlistId, name).then(
      (playlist) => {
        setPending(false);
        onRenamed(playlist);
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
        setFailure(t.playlists.renameDialog.failed(errorText(error)));
      },
    );
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={t.playlists.renameDialog.title}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending
            ? t.playlists.renameDialog.submitting
            : t.playlists.renameDialog.submit}
        </>
      }
      pending={pending}
      submitDisabled={unchanged}
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
