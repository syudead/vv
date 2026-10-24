import { CircleAlert } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import { RequestFailed, type Video } from "../api/client";
import { createVirtualVideo } from "../api/virtualVideos";
import { errorText, t, type UiText } from "../i18n";
import { typedLink } from "../lib/typedLink";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";

const words = t.library.newVirtualVideo;

/**
 * Creates from a title, a link or both; a link alone is enough, and its fetch fills the rest after the dialog closes.
 * Mount only while open so drafts reset on close.
 */
export default function NewVirtualVideoDialog({
  onClose,
  onDone,
}: {
  onClose: () => void;
  onDone: (video: Video) => void;
}) {
  const titleId = useId();
  const linkId = useId();
  const titleErrorId = useId();
  const linkErrorId = useId();
  const linkHintId = useId();
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
        onDone(video);
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
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={words.title}
      description={words.description}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? words.submitting : words.submit}
        </>
      }
      pending={pending}
      initialFocus={titleInput}
    >
      <Field data-invalid={titleInvalid !== null || undefined}>
        <FieldLabel htmlFor={titleId}>{words.field}</FieldLabel>
        <Input
          ref={titleInput}
          id={titleId}
          type="text"
          autoComplete="off"
          value={title}
          onChange={(event) => {
            setTitle(event.target.value);
            setTitleInvalid(null);
          }}
          aria-invalid={titleInvalid !== null || undefined}
          aria-describedby={titleInvalid !== null ? titleErrorId : undefined}
          className="h-8"
        />
        {titleInvalid !== null && (
          <FieldError id={titleErrorId}>{titleInvalid}</FieldError>
        )}
      </Field>
      <Field data-invalid={linkInvalid !== null || undefined}>
        <FieldLabel htmlFor={linkId}>{words.linkField}</FieldLabel>
        <Input
          ref={linkInput}
          id={linkId}
          type="text"
          inputMode="url"
          autoComplete="off"
          value={link}
          onChange={(event) => {
            setLink(event.target.value);
            setLinkInvalid(null);
          }}
          aria-invalid={linkInvalid !== null || undefined}
          aria-describedby={linkInvalid !== null ? linkErrorId : linkHintId}
          className="h-8"
        />
        {linkInvalid !== null ? (
          <FieldError id={linkErrorId}>{linkInvalid}</FieldError>
        ) : (
          <FieldDescription id={linkHintId}>{words.linkHint}</FieldDescription>
        )}
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
