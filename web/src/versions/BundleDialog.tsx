import { LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useEffectEvent, useId, useRef, useState } from "react";

import {
  bundleVideos,
  getVideo,
  isAborted,
  RequestFailed,
  type Video,
  type VideoVersions,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import { ModalFrame } from "../ui/ModalFrame";
import Skeleton from "../ui/Skeleton";
import { useToast } from "../ui/Toast";
import { VersionDetailsLine, versionDetails, versionDetailsText } from "./VersionDetails";

type Rows =
  { kind: "loading" } | { kind: "ready"; videos: readonly Video[] } | { kind: "failed" };

/** 行が 6 本を超えたら一覧の中だけをスクロールさせる（ui-design.md「Bundle dialog」）。 */
const scrollAfterRows = 6;

/**
 * BundleDialog は選んだ動画を 1 つの集まり（同じ動画の別バージョン）に束ね、一覧に出す代表を
 * 選ばせる窓である（specs/030-video-versions/ui-design.md「Bundle dialog」）。選択バーの
 * 「Bundle as versions」と候補の画面の「Same video…」が同じ窓を使い、代表の選び方を 2 か所で
 * 別にしない。
 *
 * 呼び出し元は束ねる id の並び（`videoIds`）を渡し、行の中身をこの窓が `GET /api/videos/{id}` で
 * 取る。中身を既に持つ呼び出し元は `videos` も渡し、最初の取得を省く。行の並びは `videoIds` の順。
 * 代表は既定で選ばない。成功したらトーストで伝えて `onBundled` を呼び、閉じるのは呼び出し元に
 * 任せる。失敗は窓の中の 1 行で伝え、選んだ代表を残す。
 */
export default function BundleDialog({
  videoIds,
  videos,
  onClose,
  onBundled,
}: {
  videoIds: readonly number[];
  /** 行の中身を既に持つとき（候補の画面）に渡す。`videoIds` と同じ順に並べる。 */
  videos?: readonly Video[];
  onClose: () => void;
  onBundled: (versions: VideoVersions) => void;
}) {
  const toast = useToast();
  const name = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  const submitButton = useRef<HTMLButtonElement>(null);
  const [rows, setRows] = useState<Rows>(() =>
    videos === undefined ? { kind: "loading" } : { kind: "ready", videos },
  );
  const [representative, setRepresentative] = useState<number | null>(null);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const request = useRef<AbortController | null>(null);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);

  const load = useCallback(() => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setRows({ kind: "loading" });
    Promise.all(videoIds.map((id) => getVideo(id, controller.signal))).then(
      (loaded) => {
        if (controller.signal.aborted) return;
        setRows({ kind: "ready", videos: loaded });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        // 1 本でも取れなければ（404 を含む）、どれが代表の候補かを示せないので束ねさせない。
        setRows({ kind: "failed" });
      },
    );
  }, [videoIds]);

  // 中身を渡されたときは最初の取得を省く。取り直し（Retry・404）は常に取る。
  const given = useRef(videos !== undefined);
  useEffect(() => {
    if (given.current) return;
    load();
    return () => request.current?.abort();
  }, [load]);

  // 取り直した行に選んだ代表がもう無ければ、選び直させる。
  const chosen =
    rows.kind === "ready" && rows.videos.some((video) => video.id === representative)
      ? representative
      : null;

  // 送信の失敗の直後は、送る間 disabled にしていた「Bundle」がフォーカスを失い body に
  // 落ちるので、まだ押せる操作（代表が残っていれば「Bundle」）へ戻す（014 の統合の窓と同じ）。
  const restoreFocus = useEffectEvent(() => {
    (chosen !== null && rows.kind === "ready" ? submitButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (failure !== null) restoreFocus();
  }, [failure]);

  /** 窓を閉じるすべての経路（×・Cancel・Esc）が通る。送っている間は閉じない。 */
  function handleClose() {
    if (pending) return;
    onClose();
  }

  function submit() {
    if (pending || chosen === null || rows.kind !== "ready") return;
    setFailure(null);
    setPending(true);
    bundleVideos(videoIds, chosen).then(
      (versions) => {
        if (!alive.current) return;
        setPending(false);
        const shown =
          versions.items.find((video) => video.id === versions.representativeId) ??
          versions.items[0];
        toast(t.versions.bundle.bundled(versions.items.length, shown?.title ?? ""));
        onBundled(versions);
      },
      (error: unknown) => {
        if (!alive.current) return;
        setPending(false);
        setFailure(t.versions.bundle.failed(errorText(error)));
        // 選んだ中に消えた動画がある。行を出したうえで一覧を取り直す。
        if (error instanceof RequestFailed && error.status === 404) load();
      },
    );
  }

  const count = videoIds.length;

  return (
    <ModalFrame
      title={t.versions.bundle.title}
      onClose={handleClose}
      initialFocus={cancel}
      width="sm:max-w-lg"
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4 sm:p-5">
        <p className="text-sm text-fg-muted">{t.versions.bundle.description(count)}</p>
        {rows.kind === "loading" && (
          <div className="flex flex-col gap-1" aria-busy="true">
            {Array.from({ length: Math.min(count, scrollAfterRows) }, (_, index) => (
              <Skeleton key={index} className="h-12" />
            ))}
          </div>
        )}
        {rows.kind === "failed" && (
          <div className="flex items-center gap-2">
            <p role="alert" className="text-sm text-danger">
              {t.versions.bundle.loadFailed}
            </p>
            <Button variant="ghost" size="sm" onClick={load}>
              {t.common.retry}
            </Button>
          </div>
        )}
        {rows.kind === "ready" && (
          <div
            role="radiogroup"
            aria-label={t.versions.bundle.representative}
            className={cn(
              "divide-y divide-border",
              rows.videos.length > scrollAfterRows && "max-h-80 overflow-y-auto",
            )}
          >
            {rows.videos.map((video) => (
              <RepresentativeRow
                key={video.id}
                video={video}
                name={name}
                checked={chosen === video.id}
                disabled={pending}
                onChoose={() => {
                  setRepresentative(video.id);
                  setFailure(null);
                }}
              />
            ))}
          </div>
        )}
      </div>
      {failure !== null && (
        <p role="alert" className="shrink-0 px-4 pb-3 text-sm text-danger sm:px-5">
          {failure}
        </p>
      )}
      <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={handleClose} disabled={pending}>
          {t.common.cancel}
        </Button>
        <Button
          ref={submitButton}
          variant="primary"
          onClick={submit}
          disabled={pending || chosen === null || rows.kind !== "ready"}
        >
          {pending && (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          )}
          {t.versions.bundle.submit}
        </Button>
      </div>
    </ModalFrame>
  );
}

function RepresentativeRow({
  video,
  name,
  checked,
  disabled,
  onChoose,
}: {
  video: Video;
  name: string;
  checked: boolean;
  disabled: boolean;
  onChoose: () => void;
}) {
  const details = versionDetails(video);
  // 所有者だけの窓なので、行の title で絶対パスを読める（「Versions list」と同じ）。
  const detailsTitle = versionDetailsText(
    details,
    " · ",
    video.location?.path ?? details.place,
  );
  // どのタグが集まりのものになり、どれが脇に置かれるかを選ぶ前に見せる。フォルダ名から
  // だけ付いたタグは束ねても変わらないので出さない。
  const tags = video.tags.filter((tag) => tag.manual).map((tag) => tag.name);
  const bundled =
    video.versions !== undefined && video.versions.count > 1
      ? t.versions.bundle.alreadyBundled(video.versions.count)
      : null;
  const note = [...(bundled === null ? [] : [bundled]), ...tags].join(" · ");

  return (
    <label
      className={cn(
        "flex items-start gap-3 py-2",
        disabled ? "cursor-default" : "cursor-pointer",
      )}
    >
      <input
        type="radio"
        name={name}
        value={video.id}
        checked={checked}
        disabled={disabled}
        onChange={onChoose}
        aria-label={t.versions.bundle.row(video.title, versionDetailsText(details, " "))}
        className="mt-0.5 size-4 shrink-0 accent-primary"
      />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm text-fg" title={video.title}>
          {video.title}
        </span>
        <VersionDetailsLine details={details} title={detailsTitle} />
        {note !== "" && (
          <span className="truncate text-xs text-fg-muted" title={note}>
            {note}
          </span>
        )}
      </span>
    </label>
  );
}
