import {
  Camera,
  ImageUp,
  LoaderCircle,
  type LucideIcon,
  WandSparkles,
} from "lucide-react";
import { type ChangeEvent, useRef, useState } from "react";

import type { Video } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { Button } from "../../ui/shadcn/button";
import { Input } from "../../ui/shadcn/input";
import type { ThumbnailCapture } from "../playerLayers";
import type { useThumbnailUpload } from "../thumbnailUpload/UploadEntry";
import { UPLOAD_TYPES } from "../thumbnailUpload/uploadRules";
import { FailureLine, PanelSection } from "./panelParts";
import type { useThumbnailPosition } from "./useThumbnailPosition";

type Way = "frame" | "automatic" | "upload" | "touchUp";

export interface ThumbnailPanelProps {
  video: Video;
  /** Supplied only for owners while the player is visible. */
  capture: ThumbnailCapture | undefined;
  position: ReturnType<typeof useThumbnailPosition>;
  upload: ReturnType<typeof useThumbnailUpload>;
  failure: UiText | null;
}

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

function originText(video: Video): UiText {
  const text = t.player.tabs;
  if (video.thumbnailImage === "uploaded") return text.uploadedOrigin;
  if (video.thumbnailImage === "generated") return text.generatedOrigin;
  if (video.thumbnailPositionMs !== undefined) {
    return t.player.thumbnailEdit.frameAt(formatDuration(video.thumbnailPositionMs));
  }
  return text.automaticOrigin;
}

/**
 * The current thumbnail with its origin and the three ways to replace it. The requests and the
 * preview belong to the tabs container, so a send started here finishes after a tab switch.
 */
export default function ThumbnailPanel({
  video,
  capture,
  position,
  upload,
  failure,
}: ThumbnailPanelProps) {
  const fileInput = useRef<HTMLInputElement | null>(null);
  const sending = sendingWay(position, upload);
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

  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file !== undefined) upload.choose(file);
  };

  return (
    <div className="flex flex-col gap-6">
      <PanelSection title={text.currentThumbnail}>
        <div className="flex flex-col items-start gap-2">
          <CurrentImage url={video.thumbnailUrl} />
          <p className="text-sm text-muted-foreground tabular-nums">
            {originText(video)}
          </p>
          {!automatic && (
            <WayButton
              variant="ghost"
              label={text.backToAutomatic}
              {...action("automatic", () => position.send(null))}
            />
          )}
        </div>
      </PanelSection>
      <PanelSection title={text.replaceWith}>
        <div className="flex flex-col gap-2">
          {frameShown && (
            <WayRow
              icon={Camera}
              label={text.useThisFrame}
              note={text.useThisFrameNote}
              {...action("frame", () => {
                const positionMs = framePosition(capture);
                if (positionMs !== null) position.send(positionMs);
              })}
            />
          )}
          <WayRow
            icon={ImageUp}
            label={text.uploadImage}
            note={text.uploadImageNote}
            {...action("upload", () => fileInput.current?.click())}
          />
          <WayRow
            icon={WandSparkles}
            label={text.touchUp}
            note={text.touchUpNote}
            {...action("touchUp", () =>
              upload.chooseSource(
                { source: "thumbnail" },
                t.player.thumbnailEdit.currentThumbnail,
              ),
            )}
          />
        </div>
        <Input
          ref={fileInput}
          type="file"
          accept={UPLOAD_TYPES}
          tabIndex={-1}
          aria-hidden="true"
          className="hidden"
          data-testid="thumbnail-upload-input"
          onChange={choose}
        />
      </PanelSection>
      <FailureLine failure={failure} />
    </div>
  );
}

/** The stored thumbnail at the right column's card width; the empty frame while there is none. */
function CurrentImage({ url }: { url: string | undefined }) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  return (
    <VideoThumbnail className="max-w-card-0">
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

interface WayState {
  sending: boolean;
  /** Another way is sending, so this one does nothing when pressed. */
  waiting: boolean;
  onPress: () => void;
}

function WayButton({
  variant,
  icon: Icon,
  label,
  sending,
  waiting,
  onPress,
}: WayState & { variant: "outline" | "ghost"; icon?: LucideIcon; label: UiText }) {
  return (
    <Button
      variant={variant}
      size="sm"
      aria-disabled={waiting || undefined}
      aria-busy={sending || undefined}
      onClick={onPress}
    >
      {sending ? (
        <LoaderCircle
          aria-hidden="true"
          className="animate-spin motion-reduce:animate-none"
        />
      ) : (
        Icon !== undefined && <Icon aria-hidden="true" />
      )}
      {label}
    </Button>
  );
}

/** A way's framed button with its muted note after it, under it below `sm`. */
function WayRow({
  note,
  ...button
}: WayState & { icon: LucideIcon; label: UiText; note: UiText }) {
  return (
    <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:gap-3">
      <WayButton variant="outline" {...button} />
      <span className="text-xs text-muted-foreground">{note}</span>
    </div>
  );
}
