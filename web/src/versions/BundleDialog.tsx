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

/** Scroll only the list after six rows. */
const scrollAfterRows = 6;

/**
 * Keep videoIds order and leave the representative unselected initially; supplied videos skip only
 * the initial read. Failures preserve the selection and callers own closing after onBundled.
 */
export default function BundleDialog({
  videoIds,
  videos,
  onClose,
  onBundled,
}: {
  videoIds: readonly number[];
  /** Must follow videoIds order. */
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
        // A failed candidate read, including 404, blocks bundling because the representative choices are incomplete.
        setRows({ kind: "failed" });
      },
    );
  }, [videoIds]);

  // Supplied videos skip only the initial read; Retry and 404 recovery always fetch.
  const given = useRef(videos !== undefined);
  useEffect(() => {
    if (given.current) return;
    load();
    return () => request.current?.abort();
  }, [load]);

  // Require a new representative if refresh removes the selected video.
  const chosen =
    rows.kind === "ready" && rows.videos.some((video) => video.id === representative)
      ? representative
      : null;

  // Restore focus after failure because the disabled Bundle button may have sent focus to body.
  const restoreFocus = useEffectEvent(() => {
    (chosen !== null && rows.kind === "ready" ? submitButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (failure !== null) restoreFocus();
  }, [failure]);

  /** All close paths pass here; reject closing while submitting. */
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
        // A selected video disappeared; show the failure and refresh candidates.
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
  // This owner-only dialog may expose the absolute path in title.
  const detailsTitle = versionDetailsText(
    details,
    " · ",
    video.location?.path ?? details.place,
  );
  // Preview manual-tag effects before bundling; folder-derived tags are unchanged and omitted.
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
