import { LoaderCircle } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import type { Video } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import { ScrubFrame } from "../../ui/ScrubPreview";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { Button } from "../../ui/shadcn/button";
import { Separator } from "../../ui/shadcn/separator";
import type { ThumbnailCapture } from "../playerLayers";
import type { useThumbnailUpload } from "../thumbnailUpload/UploadEntry";
import { UPLOAD_TYPES, uploadRefusal } from "../thumbnailUpload/uploadRules";
import { FailureLine, PanelSection } from "./panelParts";
import type { useThumbnailPosition } from "./useThumbnailPosition";
import { useSpriteFrame } from "./useSpriteFrame";

type Way = "frame" | "automatic" | "upload" | "touchUp";

export interface ThumbnailPanelProps {
  video: Video;
  /** Supplied only for owners while the player is visible. */
  capture: ThumbnailCapture | undefined;
  position: ReturnType<typeof useThumbnailPosition>;
  upload: ReturnType<typeof useThumbnailUpload>;
  failure: UiText | null;
}

/** How often the frame row follows the player's position. */
const positionPollMs = 250;

/** The way whose request is running, if any; the other ways wait for it. */
function sendingWay(
  position: ThumbnailPanelProps["position"],
  upload: ThumbnailPanelProps["upload"],
): Way | null {
  if (position.sending === "capture") return "frame";
  if (position.sending === "clear") return "automatic";
  if (!upload.sending && !upload.preparing) return null;
  return upload.from === "file" ? "upload" : "touchUp";
}

/** The player's position in whole milliseconds while it shows a frame, otherwise null. */
function framePosition(capture: ThumbnailCapture | undefined): number | null {
  if (capture === undefined || !capture.enabled) return null;
  const raw = capture.positionMs();
  if (raw === null || !Number.isFinite(raw)) return null;
  return Math.max(0, Math.round(raw));
}

/** The player's position, read again while the panel is shown, so the frame row follows playback. */
function usePlayerPosition(capture: ThumbnailCapture | undefined): number | null {
  const [positionMs, setPositionMs] = useState(() => framePosition(capture));
  const latest = useRef(capture);
  latest.current = capture;
  const enabled = capture?.enabled === true;
  useEffect(() => {
    const read = () => setPositionMs(framePosition(latest.current));
    read();
    const timer = window.setInterval(read, positionPollMs);
    return () => window.clearInterval(timer);
  }, [enabled]);
  return positionMs;
}

function originText(video: Video): ReactNode {
  const text = t.player.tabs;
  if (video.thumbnailImage === "uploaded") return text.uploadedOrigin;
  if (video.thumbnailImage === "generated") return text.generatedOrigin;
  if (video.thumbnailPositionMs !== undefined) {
    return t.player.thumbnailEdit.frameAt(formatDuration(video.thumbnailPositionMs));
  }
  return text.automaticOrigin;
}

/**
 * The current thumbnail with its origin, and the three ways to replace it as rows of preview, title
 * and pill; an image file is chosen, dropped or pasted in an ImageUploadDialog. The requests and
 * the preview belong to the tabs container, so a send started here finishes after a tab switch.
 */
export default function ThumbnailPanel({
  video,
  capture,
  position,
  upload,
  failure,
}: ThumbnailPanelProps) {
  const [uploading, setUploading] = useState(false);
  const sending = sendingWay(position, upload);
  const playerPosition = usePlayerPosition(capture);
  const spriteFrame = useSpriteFrame(video.seekThumbnailUrl, playerPosition);
  const text = t.player.tabs;
  const automatic =
    video.thumbnailImage === undefined && video.thumbnailPositionMs === undefined;
  const frameShown = capture?.enabled === true || sending === "frame";

  /** Every way shares one guard so a press during another send does nothing. */
  const action = (way: Way, run: () => void) => ({
    sending: sending === way,
    waiting: sending !== null && sending !== way,
    onPress: () => {
      if (sending === null) run();
    },
  });

  /** Refuse by type or size inside the dialog; a readable file closes it and opens the preview. */
  const take = (file: File) => {
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      return refusal === "type"
        ? t.player.thumbnailUpload.refusedType
        : t.player.thumbnailUpload.refusedSize;
    }
    setUploading(false);
    upload.choose(file);
    return null;
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-6 md:flex-row">
        <PanelSection title={text.currentThumbnail}>
          <div className="flex flex-col items-start gap-2">
            {/* The wrapper sets the width: cn keeps the thumbnail's w-full beside a named step. */}
            <div className="w-card-0 max-w-full">
              <ThumbnailImage url={video.thumbnailUrl} />
            </div>
            <p className="text-xs text-muted-foreground tabular-nums">
              {originText(video)}
            </p>
            {!automatic && (
              <WayButton
                variant="ghost"
                className="-ml-2"
                label={text.backToAutomatic}
                {...action("automatic", () => position.send(null))}
              />
            )}
          </div>
        </PanelSection>
        <Separator className="md:hidden" />
        <Separator orientation="vertical" className="hidden md:block" />
        <div className="min-w-0 flex-1">
          <PanelSection title={text.replaceWith}>
            <ul className="flex flex-col">
              {frameShown && (
                <WayRow
                  preview={
                    <WayPreview>
                      <ScrubFrame frame={spriteFrame} />
                    </WayPreview>
                  }
                  title={text.frameOnScreen}
                  note={
                    <span className="font-mono">
                      {formatDuration(playerPosition ?? video.thumbnailPositionMs ?? 0)}
                    </span>
                  }
                  label={text.useThisFrame}
                  {...action("frame", () => {
                    const positionMs = framePosition(capture);
                    if (positionMs !== null) position.send(positionMs);
                  })}
                />
              )}
              <WayRow
                preview={<WayPreview dashed />}
                title={text.anImage}
                note={text.uploadImageNote}
                label={text.uploadImage}
                {...action("upload", () => setUploading(true))}
              />
              <WayRow
                preview={
                  <ThumbnailImage
                    url={video.thumbnailUrl}
                    className="w-replace-preview shrink-0 opacity-50"
                  />
                }
                title={text.touchUp}
                note={text.touchUpNote}
                label={text.touchUpButton}
                {...action("touchUp", () =>
                  upload.chooseSource(
                    { source: "thumbnail" },
                    t.player.thumbnailEdit.currentThumbnail,
                  ),
                )}
              />
            </ul>
            <ImageUploadDialog
              open={uploading}
              onOpenChange={setUploading}
              title={t.player.thumbnailUpload.uploadTitle}
              description={t.player.thumbnailUpload.uploadHint}
              accept={UPLOAD_TYPES}
              onFile={take}
            />
          </PanelSection>
        </div>
      </div>
      <FailureLine failure={failure} />
    </div>
  );
}

/** The stored thumbnail; the empty frame while there is none or it fails to load. */
function ThumbnailImage({
  url,
  className,
}: {
  url: string | undefined;
  className?: string;
}) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  return (
    <VideoThumbnail className={className}>
      {url !== undefined && url !== brokenUrl && (
        <VideoThumbnailImage
          src={url}
          alt={t.player.thumbnailEdit.currentThumbnail}
          onError={() => setBrokenUrl(url)}
        />
      )}
    </VideoThumbnail>
  );
}

/** A row's small preview frame; dashed and unfilled for a way that has no image yet. */
function WayPreview({
  dashed = false,
  children,
}: {
  dashed?: boolean;
  children?: ReactNode;
}) {
  return (
    <VideoThumbnail
      aria-hidden="true"
      className={cn(
        "w-replace-preview shrink-0",
        dashed && "border border-dashed border-input bg-transparent",
      )}
    >
      {children}
    </VideoThumbnail>
  );
}

interface WayState {
  sending: boolean;
  /** Another way is sending, so this one does nothing when pressed. */
  waiting: boolean;
  onPress: () => void;
}

function WayButton({
  variant,
  label,
  className,
  sending,
  waiting,
  onPress,
}: WayState & { variant: "secondary" | "ghost"; label: UiText; className?: string }) {
  return (
    <Button
      variant={variant}
      size="sm"
      shape={variant === "secondary" ? "pill" : "default"}
      aria-disabled={waiting || undefined}
      aria-busy={sending || undefined}
      onClick={onPress}
      className={className}
    >
      {sending && (
        <LoaderCircle
          aria-hidden="true"
          className="animate-spin motion-reduce:animate-none"
        />
      )}
      {label}
    </Button>
  );
}

/** A way: its preview, title and note, then its pill; below `sm` the pill moves under the note. */
function WayRow({
  preview,
  title,
  note,
  ...button
}: WayState & { preview: ReactNode; title: UiText; note: ReactNode; label: UiText }) {
  return (
    <li className="flex items-center gap-3 border-b border-border py-2 last:border-b-0">
      {preview}
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-sm text-foreground">{title}</span>
          <span className="text-2xs text-muted-foreground tabular-nums">{note}</span>
        </div>
        <div>
          <WayButton variant="secondary" {...button} />
        </div>
      </div>
    </li>
  );
}
