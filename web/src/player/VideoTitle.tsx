import { AlertCircle, FileVideo, LoaderCircle, Pencil } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";

import { isAborted, RequestFailed, setVideoDisplayName, type Video } from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/** 題名の文字。ページの題の段（`text-xl`、web/registry/rules/foundations.md の Type）。 */
const titleType = "text-xl font-semibold";

/**
 * VideoTitle は再生画面の題名（`h1`）である。所有者には、その場で表示名を編集する入口と、
 * 表示名があるときの元のファイル名の行を添える
 * （specs/029-video-overrides/ui-design.md「Title editing」「File name line」）。
 * ゲストには今の `h1` だけを出す。
 *
 * 保存は `PUT /api/videos/{id}/display-name` を 1 回送り、応答の動画を、送る直前に取った
 * detailMark と一緒に `onSaved` で渡す（呼び出し側が手元の動画を差し替える）。失敗したら編集を続け、入力の下に理由を
 * 1 行出す。404 では `onStale` で動画を取り直させる。
 *
 * 別の動画へ移ったら編集と失敗の行を持ち越さないよう、呼び出し側が動画の id を `key` に
 * して作り直す。
 */
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
  const [failure, setFailure] = useState<UiText | null>(null);
  const editButton = useRef<HTMLButtonElement | null>(null);
  const input = useRef<HTMLInputElement | null>(null);
  // 編集を始めるたび・終えるたびに進める。取り消した後に届いた応答は、動画の差し替えに
  // だけ使い、編集の状態（失敗の行・編集の終わり）には触れない。
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
    // ファイル名と同じ表示名は作らない。前後に空白のあるファイル名の題名は、変えずに
    // 保存しても送らない（送るとサーバーが空白を除いた表示名を作る）。
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
    setVideoDisplayName(video.id, draft, controller.signal).then(
      (saved) => {
        onSaved(saved, mark);
        if (session.current === mine) finish();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        if (session.current === mine) {
          setSending(false);
          setFailure(t.player.title.saveFailed(errorText(error)));
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
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <Input
              ref={input}
              type="text"
              aria-label={t.player.title.input}
              value={draft}
              placeholder={video.fileTitle}
              readOnly={sending}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onInputKeyDown}
              // 文字の左端を題名の左端にそろえる（Input の左の余白 px-3 の分だけ左へ出す）。
              className={cn(titleType, "-ml-3 sm:flex-1")}
            />
            <div className="flex shrink-0 items-center gap-3">
              <Button
                type="submit"
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
              <Button variant="ghost" onClick={finish}>
                {t.player.title.cancel}
              </Button>
            </div>
          </div>
          {failure !== null && (
            <p
              role="alert"
              className="flex items-center gap-1.5 text-base text-destructive"
            >
              <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
              {failure}
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
                // 縦の中心を題名の 1 行目（行の高さ 1.75rem）にそろえる。
                className="-mt-0.5 shrink-0 text-muted-foreground"
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
          className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground"
        >
          <FileVideo className="size-4 shrink-0" aria-hidden="true" />
          <span className="sr-only">{t.player.title.fileName} </span>
          <span className="truncate">{video.fileTitle}</span>
        </p>
      )}
    </div>
  );
}
