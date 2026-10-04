import { AlertCircle, FileVideo, LoaderCircle, Pencil } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";

import { isAborted, RequestFailed, setVideoDisplayName, type Video } from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/legacy/Button";
import IconButton from "../ui/legacy/IconButton";

const titleType = "text-xl leading-snug font-semibold text-fg sm:text-2xl";

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
    <h1 className={cn(titleType, "min-w-0 [overflow-wrap:anywhere]", owner && "flex-1")}>
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
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <input
              ref={input}
              type="text"
              aria-label={t.player.title.input}
              value={draft}
              placeholder={video.fileTitle}
              readOnly={sending}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onInputKeyDown}
              // 文字の左端を題名の左端にそろえる（Group line の -ml-2 と同じ考え方。枠の 1px の分だけ内側の余白を減らす）。
              className={cn(
                titleType,
                "-mx-2 min-w-0 rounded-md border border-border bg-field px-[7px] py-1 placeholder:text-fg-subtle focus:border-primary focus:outline-none sm:mr-0 sm:flex-1",
              )}
            />
            <div className="flex shrink-0 items-center gap-2">
              <Button
                type="submit"
                variant="primary"
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
            <p role="alert" className="flex items-center gap-1.5 text-sm text-danger">
              <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
              {failure}
            </p>
          )}
        </form>
      ) : (
        <div className="flex items-start gap-2">
          {heading}
          <IconButton
            ref={editButton}
            label={t.player.title.edit}
            size="sm"
            onClick={start}
            // 縦の中心を題名の 1 行目にそろえる。
            className="-mt-0.5 text-fg-muted! hover:text-fg! sm:mt-0.5"
          >
            <Pencil aria-hidden="true" />
          </IconButton>
        </div>
      )}
      {!editing && video.displayName !== undefined && video.fileTitle !== undefined && (
        <p
          title={t.player.title.fileNameTitle(video.fileTitle)}
          className="flex min-w-0 items-center gap-1.5 text-xs text-fg-muted sm:text-sm"
        >
          <FileVideo className="size-3.5 shrink-0 text-fg-subtle" aria-hidden="true" />
          <span className="sr-only">{t.player.title.fileName} </span>
          <span className="truncate">{video.fileTitle}</span>
        </p>
      )}
    </div>
  );
}
