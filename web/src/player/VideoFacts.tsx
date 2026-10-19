import {
  AlertCircle,
  CalendarPlus,
  Camera,
  Clock,
  Copy,
  ExternalLink,
  FileClock,
  HardDrive,
  Image,
  ImageUp,
  LoaderCircle,
  type LucideIcon,
  PencilLine,
  WandSparkles,
  X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  isAborted,
  openVideoFile,
  RequestFailed,
  revertThumbnailImage,
  setVideoThumbnailPosition,
  type Video,
} from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, formatDate, formatDateTime, t, type UiText } from "../i18n";
import { copyText } from "../lib/clipboard";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration, isNarrowVideo } from "../lib/format";
import AddToGroupButton from "../groups/AddToGroupButton";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { useToast } from "../ui/Toast";
import ActionButton from "./ActionButton";
import { factTrigger } from "./factTrigger";
import { technicalSummary } from "./properties";
import UploadEntry, { useThumbnailUpload } from "./thumbnailUpload/UploadEntry";
import UploadPreview from "./thumbnailUpload/UploadPreview";
import DetailsEntry, { type DetailsEntryProps } from "./detailsEditor/DetailsEntry";
import FetchEntry from "./workInfoFetch/FetchEntry";
import VersionsFact, { type VersionsNavigation } from "./VersionsFact";
import MediaInfoGroups from "./MediaInfoGroups";
import WorkInfo from "./WorkInfo";

export function useOpenFile(videoId: number): {
  open: () => void;
  failure: UiText | null;
} {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [videoId]);
  const open = useOpenFileRequest(videoId, setFailure);
  return { open, failure };
}

/** Report null when starting and the error on failure, allowing callers to share one error row. */
function useOpenFileRequest(
  videoId: number,
  report: (failure: UiText | null) => void,
): () => void {
  const request = useRef<AbortController | null>(null);

  useEffect(() => () => request.current?.abort(), [videoId]);

  return useCallback(() => {
    report(null);
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    void openVideoFile(videoId, controller.signal).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      report(t.player.facts.openFailed(errorText(error)));
    });
  }, [report, videoId]);
}

/** Serialize thumbnail mutations; refresh stale videos on 404. Return successful video data with the detail mark captured before submission. */
function useThumbnailPosition(
  videoId: number,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: "capture" | "clear" | "revert" | null;
  send: (positionMs: number | null | "revert") => void;
} {
  const [sending, setSending] = useState<"capture" | "clear" | "revert" | null>(null);
  // A ref also blocks two clicks within the same render.
  const busy = useRef(false);
  const request = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      request.current?.abort();
      busy.current = false;
      setSending(null);
    },
    [videoId],
  );

  const send = (positionMs: number | null | "revert") => {
    if (busy.current) return;
    busy.current = true;
    const controller = new AbortController();
    request.current = controller;
    const mark = detailMark();
    report(null);
    setSending(
      positionMs === "revert" ? "revert" : positionMs === null ? "clear" : "capture",
    );
    (positionMs === "revert"
      ? revertThumbnailImage(videoId, controller.signal)
      : setVideoThumbnailPosition(videoId, positionMs, controller.signal)
    )
      .then(
        (video) => {
          if (controller.signal.aborted) return;
          onChanged?.(video, mark);
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          report(t.player.facts.thumbnailFailed(errorText(error)));
          if (error instanceof RequestFailed && error.status === 404) onStale?.();
        },
      )
      .finally(() => {
        if (request.current !== controller) return;
        request.current = null;
        busy.current = false;
        setSending(null);
      });
  };

  return { sending, send };
}

/** Read logical playback milliseconds at click time; pause playback before editing. */
export interface ThumbnailCapture {
  enabled: boolean;
  positionMs: () => number | null;
  pause: () => void;
}

function Fact({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: UiText;
  value: string;
}) {
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      <span className="sr-only">{label} </span>
      {value}
    </li>
  );
}

function DateFact({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: UiText;
  value: string;
}) {
  const date = formatDate(value);
  const dateTime = formatDateTime(value);
  return (
    <li className="flex items-center whitespace-nowrap">
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            // Set the accessible name explicitly so child whitespace cannot change it.
            aria-label={`${label} ${date}`}
            title={t.player.facts.dateDetail(label, dateTime)}
            className={factTrigger}
          >
            <Icon aria-hidden="true" />
            <span className="sr-only">{label} </span>
            {date}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          className="block w-auto px-3 py-2 text-sm whitespace-nowrap"
          // With no focusable content, opening the popover leaves focus on its trigger.
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <span>{label}</span>{" "}
          <span className="text-muted-foreground tabular-nums">{dateTime}</span>
        </PopoverContent>
      </Popover>
    </li>
  );
}

type ThumbnailSending = ReturnType<typeof useThumbnailPosition>["sending"];

function ThumbnailFacts({
  video,
  sending,
  locked,
  onSend,
}: {
  video: Video;
  sending: ThumbnailSending;
  locked: boolean;
  onSend: ReturnType<typeof useThumbnailPosition>["send"];
}) {
  if (video.thumbnailImage !== undefined) {
    return (
      <ImageThumbnailFact
        origin={video.thumbnailImage}
        url={video.thumbnailUrl}
        reverting={sending === "revert"}
        locked={locked}
        onRevert={() => onSend("revert")}
      />
    );
  }
  if (video.thumbnailPositionMs === undefined) return null;
  return (
    <ThumbnailFact
      url={video.thumbnailUrl}
      positionMs={video.thumbnailPositionMs}
      clearing={sending === "clear"}
      locked={locked}
      onClear={() => onSend(null)}
    />
  );
}

function ListActions({
  video,
  favorite,
  onFavorite,
  onFailed,
}: {
  video: Video;
  favorite: boolean;
  onFavorite: (favorite: boolean) => Promise<void>;
  onFailed: (reason: UiText) => void;
}) {
  return (
    <>
      <FavoriteToggle
        variant="page"
        favorite={favorite}
        label={t.player.facts.favorite}
        onToggle={() => onFavorite(!favorite)}
        onFailed={onFailed}
      />
      <AddToPlaylistButton
        variant="page"
        label={t.playlists.add.label}
        videoIds={[video.id]}
      />
      <AddToGroupButton
        variant="page"
        label={t.list.card.addToGroup(video.title)}
        videoIds={[video.id]}
      />
    </>
  );
}

/** Show owner-only file, favorite, and thumbnail actions alongside video facts. Serialize thumbnail operations and share one inline error row across actions. */
export default function VideoFacts({
  video,
  owner = false,
  capture,
  onChanged,
  onStale,
  onFavorite,
  versions,
  onEditingChange,
  details,
}: {
  video: Video;
  owner?: boolean;
  /** Supplied only for owners while the player is visible. */
  capture?: ThumbnailCapture;
  onChanged?: (video: Video, mark: DetailMark) => void;
  onStale?: () => void;
  /** Resolve after toggling and refreshing the video; reject on failure. */
  onFavorite?: (favorite: boolean) => Promise<void>;
  versions?: {
    navigation: VersionsNavigation;
    onReplace: (video: Video, mark: DetailMark) => void;
    onRefresh: () => void;
  };
  /** Suspend page shortcuts while thumbnail editing or metadata fetching is open. */
  onEditingChange?: (open: boolean) => void;
  details?: DetailsEntryProps["details"];
}) {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [video.id]);
  const open = useOpenFileRequest(video.id, setFailure);
  const thumbnail = useThumbnailPosition(video.id, setFailure, onChanged, onStale);
  const upload = useThumbnailUpload(video, setFailure, onChanged, onStale);
  const uploadBusy = upload.sending || upload.preparing;
  const previewOpen = upload.preview !== null;
  const editingChange = useRef(onEditingChange);
  editingChange.current = onEditingChange;
  // Page shortcuts wait while the preview is open.
  useEffect(() => {
    if (!previewOpen) return;
    editingChange.current?.(true);
    return () => editingChange.current?.(false);
  }, [previewOpen]);
  const toast = useToast();
  const location = video.location;
  const duration = formatDuration(video.durationMs);
  const technical = technicalSummary(video);
  const actionsRef = useRef<HTMLDivElement | null>(null);
  const versionCount = video.versions?.count ?? 0;
  const favorite = owner && onFavorite !== undefined ? video.favorite : undefined;
  const f = t.player.facts;

  const copyPath = () => {
    if (location === undefined) return;
    setFailure(null);
    copyText(location.path, {
      onCopied: () => toast(f.pathCopied),
      onFailed: () => toast(f.copyFailed),
    });
  };

  const hasActions = location !== undefined || owner || favorite !== undefined;

  return (
    <div className="flex flex-col gap-3 border-t border-border pt-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <ul
          aria-label={f.label}
          className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted-foreground tabular-nums sm:gap-x-5"
        >
          {duration !== "" && <Fact icon={Clock} label={f.duration} value={duration} />}
          <Fact icon={HardDrive} label={f.size} value={formatBytes(video.sizeBytes)} />
          <DateFact icon={CalendarPlus} label={f.added} value={video.addedAt} />
          <DateFact icon={PencilLine} label={f.edited} value={video.updatedAt} />
          <DateFact icon={FileClock} label={f.created} value={video.fileCreatedAt} />
          {versions !== undefined && versionCount >= 2 && (
            <VersionsFact
              video={video}
              count={versionCount}
              owner={owner}
              navigation={versions.navigation}
              onReplace={versions.onReplace}
              onRefresh={versions.onRefresh}
              // Move focus to the first action because this fact disappears.
              onDissolved={() =>
                actionsRef.current?.querySelector<HTMLElement>("button")?.focus()
              }
            />
          )}
          {owner && (
            <ThumbnailFacts
              video={video}
              sending={thumbnail.sending}
              locked={uploadBusy}
              onSend={thumbnail.send}
            />
          )}
        </ul>
        {hasActions && (
          <div ref={actionsRef} className="ml-auto flex shrink-0 items-center">
            {favorite !== undefined && (
              <ListActions
                video={video}
                favorite={favorite}
                onFavorite={async (next) => {
                  setFailure(null);
                  // Confirm favorite state from refreshed data and block further mutations until refresh completes.
                  await onFavorite?.(next);
                }}
                onFailed={(reason) => setFailure(f.favoriteFailed(reason))}
              />
            )}
            {}
            <DetailsEntry video={video} owner={owner} details={details} />
            {}
            {owner && (
              <FetchEntry
                video={video}
                capture={capture}
                onApplied={(applied, mark) => onChanged?.(applied, mark)}
                onDialogChange={onEditingChange}
              />
            )}
            {owner && capture !== undefined && (
              <CaptureButton
                capture={capture}
                sending={thumbnail.sending === "capture"}
                locked={uploadBusy}
                onCapture={(positionMs) => thumbnail.send(positionMs)}
              />
            )}
            {/* Changing the thumbnail remains available without a player. */}
            {owner && (
              <UploadEntry
                sending={uploadBusy}
                locked={thumbnail.sending !== null}
                capture={capture}
                thumbnailUrl={video.thumbnailUrl}
                onFile={upload.choose}
                onSource={upload.chooseSource}
              />
            )}
            {upload.preview !== null && (
              <UploadPreview
                videoId={video.id}
                image={upload.preview.image}
                name={upload.preview.name}
                aspect={upload.preview.aspect}
                narrow={isNarrowVideo(video)}
                applying={upload.applying}
                onApply={upload.apply}
                onCancel={upload.cancel}
              />
            )}
            {location?.openable === true && (
              <ActionButton label={f.openFile} onClick={open}>
                <ExternalLink aria-hidden="true" />
              </ActionButton>
            )}
            {location !== undefined && (
              <ActionButton label={f.copyPath} onClick={copyPath}>
                <Copy aria-hidden="true" />
              </ActionButton>
            )}
          </div>
        )}
      </div>
      {failure !== null && (
        <p
          role="alert"
          className="-mt-1 flex items-center gap-2 text-sm text-destructive"
        >
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          {failure}
        </p>
      )}
      {}
      <WorkInfo info={video.workInfo} />
      <TechnicalLine summary={technical} />
      {}
      <MediaInfoGroups info={video.mediaInfo} />
    </div>
  );
}

function ThumbnailFact({
  url,
  positionMs,
  clearing,
  locked,
  onClear,
}: {
  url: string | undefined;
  positionMs: number;
  clearing: boolean;
  locked: boolean;
  onClear: () => void;
}) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  const label = t.player.facts.thumbnailAt(formatDuration(positionMs));
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Image className="size-4 shrink-0" aria-hidden="true" />
      <span className="aspect-video h-6 shrink-0 overflow-hidden rounded-sm bg-card">
        {url !== undefined && url !== brokenUrl && (
          <img
            src={url}
            alt={label}
            onError={() => setBrokenUrl(url)}
            className="size-full object-cover"
          />
        )}
      </span>
      <span className="sr-only">{label} </span>
      {formatDuration(positionMs)}
      <ActionButton
        label={t.player.facts.useAutomaticThumbnail}
        aria-disabled={clearing || locked || undefined}
        onClick={() => {
          if (!clearing && !locked) onClear();
        }}
        className="size-6 rounded-sm aria-disabled:cursor-default [&_svg]:size-3"
      >
        {clearing ? (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        ) : (
          <X aria-hidden="true" />
        )}
      </ActionButton>
    </li>
  );
}

/** Uploaded and generated thumbnails have no single frame timestamp. */
function ImageThumbnailFact({
  origin,
  url,
  reverting,
  locked,
  onRevert,
}: {
  origin: NonNullable<Video["thumbnailImage"]>;
  url: string | undefined;
  reverting: boolean;
  locked: boolean;
  onRevert: () => void;
}) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  const uploaded = origin === "uploaded";
  const label = uploaded
    ? t.player.thumbnailUpload.uploadedThumbnail
    : t.player.thumbnailEdit.generatedThumbnail;
  const Icon = uploaded ? ImageUp : WandSparkles;
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      <span className="aspect-video h-6 shrink-0 overflow-hidden rounded-sm bg-card">
        {url !== undefined && url !== brokenUrl && (
          <img
            src={url}
            alt={label}
            onError={() => setBrokenUrl(url)}
            className="size-full object-cover"
          />
        )}
      </span>
      <span className="sr-only">{label}</span>
      <ActionButton
        label={t.player.thumbnailEdit.useFrameInstead}
        aria-disabled={reverting || locked || undefined}
        onClick={() => {
          if (!reverting && !locked) onRevert();
        }}
        className="size-6 rounded-sm aria-disabled:cursor-default [&_svg]:size-3"
      >
        {reverting ? (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        ) : (
          <X aria-hidden="true" />
        )}
      </ActionButton>
    </li>
  );
}

/** Capture the logical position at click time without pausing; aria-disabled actions must not submit. */
function CaptureButton({
  capture,
  sending,
  locked,
  onCapture,
}: {
  capture: ThumbnailCapture;
  sending: boolean;
  locked: boolean;
  onCapture: (positionMs: number) => void;
}) {
  const unavailable = !capture.enabled && !sending;
  return (
    <ActionButton
      label={t.player.facts.useCurrentFrame}
      aria-disabled={unavailable || sending || locked || undefined}
      onClick={() => {
        if (!capture.enabled || sending || locked) return;
        const positionMs = capture.positionMs();
        if (positionMs === null || !Number.isFinite(positionMs)) return;
        onCapture(Math.max(0, Math.round(positionMs)));
      }}
      className={cn("aria-disabled:cursor-default", unavailable && "opacity-50")}
    >
      {sending ? (
        <LoaderCircle
          aria-hidden="true"
          className="animate-spin motion-reduce:animate-none"
        />
      ) : (
        <Camera aria-hidden="true" />
      )}
    </ActionButton>
  );
}

function TechnicalLine({ summary }: { summary: ReturnType<typeof technicalSummary> }) {
  const base = "text-xs tracking-wider tabular-nums";
  if (summary.kind === "pending") {
    return (
      <p className={cn(base, "text-muted-foreground")}>
        {t.player.facts.technicalPending}
      </p>
    );
  }
  if (summary.kind === "failed") {
    return <p className={cn(base, "text-warning")}>{t.player.facts.technicalFailed}</p>;
  }
  if (summary.values.length === 0) return null;
  // Clip each row's leading separator by shifting the list left by its padding and border width.
  return (
    <div className="overflow-hidden">
      <div className="-ml-technical-x">
        <ul
          aria-label={t.player.facts.technical}
          lang="en"
          className={cn(
            base,
            "-ml-px flex flex-wrap items-center gap-y-1 text-muted-foreground uppercase",
          )}
        >
          {summary.values.map((value) => (
            <li key={value} className="border-l border-input px-technical-x leading-none">
              {value}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
