import { AlertCircle, FileVideo, LoaderCircle, Pencil } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";

import { isAborted, RequestFailed, setVideoDisplayName, type Video } from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

const titleType = "text-xl leading-snug font-semibold sm:text-video-title";

/** reload offers to re-read a video that changed in another tab. */
interface TitleFailure {
  text: UiText;
  reload?: boolean;
}

function saveFailure(error: unknown): TitleFailure {
  if (error instanceof RequestFailed && error.reason === "content_changed")
    return { text: t.player.virtual.changed, reload: true };
  return { text: t.player.title.saveFailed(errorText(error)) };
}

/** Return saved data with its pre-submit detail mark; refresh on 404. Callers key by video ID to reset editing on navigation. */
export default function VideoTitle({
  video,
  owner,
  onSaved,
  onStale,
}: {
  video: Video;
  owner: boolean;
  onSaved: (video: Video, mark: DetailMark) => void;
  onStale: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [failure, setFailure] = useState<TitleFailure | null>(null);
  const editButton = useRef<HTMLButtonElement | null>(null);
  const input = useRef<HTMLInputElement | null>(null);
  // Responses after cancellation may update video data but must not change the current editing state.
  const session = useRef(0);
  const request = useRef<AbortController | null>(null);
  const returnFocus = useRef(false);

  useEffect(() => () => request.current?.abort(), []);

  useEffect(() => {
    if (editing) {
      input.current?.focus();
      input.current?.select();
    } else if (returnFocus.current) {
      returnFocus.current = false;
      editButton.current?.focus();
    }
  }, [editing]);

  const heading = (
    <h1 className={cn(titleType, "min-w-0 wrap-anywhere", owner && "flex-1")}>
      {video.title}
    </h1>
  );
  if (!owner) return heading;

  const start = () => {
    session.current += 1;
    setDraft(video.title);
    setFailure(null);
    setSending(false);
    setEditing(true);
  };

  const finish = () => {
    session.current += 1;
    returnFocus.current = true;
    setFailure(null);
    setSending(false);
    setEditing(false);
  };

  const save = (event: FormEvent) => {
    event.preventDefault();
    if (sending) return;
    // A virtual video's title is its only name, so an empty one is refused before sending.
    if (video.virtual && draft.trim() === "") {
      setFailure({ text: t.errors.reason.invalid_title });
      return;
    }
    // Avoid creating a display name when unchanged; the server would trim whitespace from the original filename.
    if (
      video.displayName === undefined &&
      (draft === video.title || draft.trim() === video.title)
    ) {
      finish();
      return;
    }
    const mine = session.current;
    const mark = detailMark();
    const controller = new AbortController();
    request.current?.abort();
    request.current = controller;
    setFailure(null);
    setSending(true);
    setVideoDisplayName(video.id, draft, controller.signal, video.contentVersion).then(
      (saved) => {
        onSaved(saved, mark);
        if (session.current === mine) finish();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        if (session.current === mine) {
          setSending(false);
          setFailure(saveFailure(error));
        }
        if (error instanceof RequestFailed && error.status === 404) onStale();
      },
    );
  };

  const onInputKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Escape") return;
    event.preventDefault();
    event.stopPropagation();
    finish();
  };

  return (
    <div className="flex flex-col gap-1">
      {editing ? (
        <form onSubmit={save} className="flex flex-col gap-1">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <Input
              ref={input}
              type="text"
              aria-label={t.player.title.input}
              value={draft}
              placeholder={video.fileTitle}
              readOnly={sending}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onInputKeyDown}
              className={cn(titleType, "-mx-2 h-auto px-2 py-1 sm:mr-0 sm:flex-1")}
            />
            <div className="flex shrink-0 items-center gap-2">
              <Button
                type="submit"
                size="sm"
                aria-disabled={sending || undefined}
                className="aria-disabled:cursor-default aria-disabled:opacity-50"
              >
                {sending && (
                  <LoaderCircle
                    aria-hidden="true"
                    className="animate-spin motion-reduce:animate-none"
                  />
                )}
                {t.player.title.save}
              </Button>
              <Button variant="ghost" size="sm" onClick={finish}>
                {t.player.title.cancel}
              </Button>
            </div>
          </div>
          {failure !== null && (
            <p
              role="alert"
              className="flex items-center gap-1.5 text-sm text-destructive"
            >
              <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
              {failure.text}
              {failure.reload === true && (
                <Button
                  variant="link"
                  size="sm"
                  className="h-auto px-0 py-0"
                  onClick={() => {
                    finish();
                    onStale();
                  }}
                >
                  {t.common.reload}
                </Button>
              )}
            </p>
          )}
        </form>
      ) : (
        <div className="flex items-start gap-2">
          {heading}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                ref={editButton}
                variant="ghost"
                size="icon-sm"
                aria-label={t.player.title.edit}
                onClick={start}
                className="-mt-0.5 shrink-0 text-muted-foreground sm:mt-0.5"
              >
                <Pencil aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t.player.title.edit}</TooltipContent>
          </Tooltip>
        </div>
      )}
      {!editing && video.displayName !== undefined && video.fileTitle !== undefined && (
        <p
          title={t.player.title.fileNameTitle(video.fileTitle)}
          className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground sm:text-sm"
        >
          <FileVideo className="size-4 shrink-0" aria-hidden="true" />
          <span className="sr-only">{t.player.title.fileName} </span>
          <span className="truncate">{video.fileTitle}</span>
        </p>
      )}
    </div>
  );
}
