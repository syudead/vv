import { type FormEvent, type ReactNode, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, type UiText } from "../i18n";
import { playlistName } from "../playlists/playlistName";
import {
  CreateDialogFrame,
  DialogTextField,
  type NameDialogWords,
} from "./NameDialogParts";

export type { NameDialogWords } from "./NameDialogParts";

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
    <CreateDialogFrame
      words={words}
      description={description}
      pending={pending}
      failure={failure}
      initialFocus={input}
      onSubmit={submit}
      onClose={onClose}
    >
      <DialogTextField
        label={nameLabel}
        value={text}
        invalid={invalid}
        inputRef={input}
        onChange={(value) => {
          setText(value);
          setInvalid(null);
        }}
      />
    </CreateDialogFrame>
  );
}
