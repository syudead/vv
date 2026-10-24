import { type FormEvent, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { RequestFailed } from "../api/client";
import { createVirtualVideo } from "../api/virtualVideos";
import { errorText, t, type UiText } from "../i18n";
import { typedLink } from "../lib/typedLink";
import { CreateDialogFrame, DialogTextField } from "./NameDialogParts";

const words = t.library.newVirtualVideo;

/**
 * Creates from a title, a link or both; a link alone is enough, and its fetch fills the rest after the dialog closes.
 * Mount while open so drafts reset on close; a created video opens on its page with `backTo` as its return target.
 */
export default function NewVirtualVideoDialog({
  backTo,
  onClose,
}: {
  backTo: string;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const titleInput = useRef<HTMLInputElement>(null);
  const linkInput = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState("");
  const [link, setLink] = useState("");
  const [titleInvalid, setTitleInvalid] = useState<UiText | null>(null);
  const [linkInvalid, setLinkInvalid] = useState<UiText | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const typedTitle = title.trim();
    const typedUrl = link.trim() === "" ? "" : typedLink(link);
    if (typedTitle === "" && typedUrl === "") {
      setTitleInvalid(words.rule);
      titleInput.current?.focus();
      return;
    }
    if (typedUrl === undefined) {
      setLinkInvalid(words.linkRule);
      linkInput.current?.focus();
      return;
    }
    setPending(true);
    setFailure(null);
    createVirtualVideo({ title: typedTitle, url: typedUrl }).then(
      (video) => {
        setPending(false);
        onClose();
        void navigate(`/videos/${String(video.id)}`, { state: { from: backTo } });
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 400) {
          // A refused title carries a reason; a refused link carries none.
          if (typedUrl !== "" && error.reason === undefined) {
            setLinkInvalid(words.linkRule);
            linkInput.current?.focus();
          } else {
            setTitleInvalid(errorText(error));
            titleInput.current?.focus();
          }
          return;
        }
        setFailure(words.failed(errorText(error)));
      },
    );
  }

  return (
    <CreateDialogFrame
      words={words}
      description={words.description}
      pending={pending}
      failure={failure}
      initialFocus={titleInput}
      onSubmit={submit}
      onClose={onClose}
    >
      <DialogTextField
        label={words.field}
        value={title}
        invalid={titleInvalid}
        inputRef={titleInput}
        onChange={(value) => {
          setTitle(value);
          setTitleInvalid(null);
        }}
      />
      <DialogTextField
        label={words.linkField}
        value={link}
        invalid={linkInvalid}
        hint={words.linkHint}
        inputRef={linkInput}
        inputMode="url"
        onChange={(value) => {
          setLink(value);
          setLinkInvalid(null);
        }}
      />
    </CreateDialogFrame>
  );
}
