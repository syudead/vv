import { CircleAlert, RotateCw } from "lucide-react";
import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";

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
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
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
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      title={t.versions.bundle.title}
      description={t.versions.bundle.description(count)}
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {t.versions.bundle.submit}
        </>
      }
      pending={pending}
      submitDisabled={chosen === null || rows.kind !== "ready"}
      submitRef={submitButton}
      cancelRef={cancel}
      initialFocus={cancel}
    >
      {rows.kind === "loading" && (
        <div className="flex flex-col gap-1" aria-busy="true">
          {Array.from({ length: Math.min(count, scrollAfterRows) }, (_, index) => (
            <Skeleton key={index} className="h-12" />
          ))}
        </div>
      )}
      {rows.kind === "failed" && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{t.versions.bundle.loadFailed}</AlertTitle>
          <AlertAction>
            <Button variant="outline" size="sm" onClick={load}>
              <RotateCw aria-hidden="true" />
              {t.common.retry}
            </Button>
          </AlertAction>
        </Alert>
      )}
      {rows.kind === "ready" && (
        <RadioGroup
          aria-label={t.versions.bundle.representative}
          value={chosen === null ? "" : String(chosen)}
          onValueChange={(next) => {
            setRepresentative(Number(next));
            setFailure(null);
          }}
          disabled={pending}
          className={cn(
            "grid-cols-1 gap-0 divide-y divide-border",
            rows.videos.length > scrollAfterRows && "max-h-popover-wide overflow-y-auto",
          )}
        >
          {rows.videos.map((video) => (
            <RepresentativeRow key={video.id} video={video} disabled={pending} />
          ))}
        </RadioGroup>
      )}
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}

function RepresentativeRow({ video, disabled }: { video: Video; disabled: boolean }) {
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
      <RadioGroupItem
        value={String(video.id)}
        aria-label={t.versions.bundle.row(video.title, versionDetailsText(details, " "))}
        className="mt-0.5"
      />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm text-foreground" title={video.title}>
          {video.title}
        </span>
        <VersionDetailsLine details={details} title={detailsTitle} />
        {note !== "" && (
          <span className="truncate text-xs text-muted-foreground" title={note}>
            {note}
          </span>
        )}
      </span>
    </label>
  );
}
