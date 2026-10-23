import { FileImage, Film, ImageUp, LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
  discardThumbnailEdit,
  getThumbnailEditFrame,
  isAborted,
  RequestFailed,
  startThumbnailEdit,
  type ThumbnailEditStart,
  uploadThumbnailImage,
  type Video,
} from "../../api/client";
import { detailMark, type DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration, isNarrowVideo } from "../../lib/format";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import ActionButton from "../ActionButton";
import type { ThumbnailCapture } from "../VideoFacts";
import {
  encodeComposition,
  type LoadResult,
  loadUploadImage,
  type UploadImage,
} from "./composeImage";
import { type Fit, frameAspect } from "./framing";
import UploadPreview from "./UploadPreview";
import { UPLOAD_TYPES, uploadRefusal } from "./uploadRules";

export interface UploadPreviewState {
  videoId: number;
  image: UploadImage;
  name: string;
  aspect: number | null;
  /** Apply only to the content version captured when opening the preview. */
  contentVersion: string;
}

/** Validate and preview before uploading with the captured contentVersion. Uploads continue after leaving the page or the video, but their responses must not update what is shown now. */
export function useThumbnailUpload(
  video: Video | undefined,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: boolean;
  preparing: boolean;
  applying: boolean;
  preview: UploadPreviewState | null;
  choose: (file: File) => void;
  chooseSource: (start: ThumbnailEditStart, name: string) => void;
  apply: (fit: Fit, replaced: UploadImage | null) => void;
  cancel: () => void;
} {
  const [sending, setSending] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [applying, setApplying] = useState(false);
  const [preview, setPreview] = useState<UploadPreviewState | null>(null);
  // A ref also blocks two clicks within the same render.
  const busy = useRef(false);
  // Bumped when the page leaves or the video changes, so an earlier read, composition or upload
  // neither blocks nor updates the video shown now; the page keeps this hook across videos.
  const generation = useRef(0);

  useEffect(
    () => () => {
      generation.current += 1;
    },
    [],
  );

  useEffect(() => {
    generation.current += 1;
    busy.current = false;
    setSending(false);
    setPreparing(false);
    setApplying(false);
    setPreview((current) => {
      current?.image.bitmap.close();
      return null;
    });
  }, [video?.id]);

  const text = t.player.thumbnailUpload;

  /** Decode in the background and open the preview unless the page or the video changed meanwhile. */
  const open = (
    current: Video,
    name: string,
    load: Promise<LoadResult>,
    failed: (error: unknown) => void,
  ) => {
    const started = generation.current;
    const isCurrent = () => generation.current === started;
    busy.current = true;
    report(null);
    setPreparing(true);
    const opened = {
      videoId: current.id,
      aspect: frameAspect(current),
      contentVersion: current.contentVersion ?? "",
    };
    load
      .then((loaded) => {
        if (!isCurrent()) {
          if ("image" in loaded) loaded.image.bitmap.close();
          return;
        }
        if ("refusal" in loaded) {
          report(text.failed(t.errors.reason[loaded.refusal]));
          busy.current = false;
          return;
        }
        setPreview({ image: loaded.image, name, ...opened });
      })
      .catch((error: unknown) => {
        if (!isCurrent()) return;
        if (!isAborted(error)) failed(error);
        busy.current = false;
      })
      .finally(() => {
        if (isCurrent()) setPreparing(false);
      });
  };

  const choose = (file: File) => {
    if (busy.current || video === undefined) return;
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      report(refusal === "type" ? text.refusedType : text.refusedSize);
      return;
    }
    open(video, file.name, loadUploadImage(file), () =>
      report(text.failed(t.errors.reason.thumbnail_upload_invalid)),
    );
  };

  /** A frame or the current thumbnail, read through a short-lived edit session at its full size. */
  const chooseSource = (start: ThumbnailEditStart, name: string) => {
    if (busy.current || video === undefined) return;
    open(video, name, loadSource(video.id, start), (error) => {
      report(t.player.thumbnailEdit.startFailed(errorText(error)));
      if (staleAfter(error)) onStale?.();
    });
  };

  const close = (current: UploadPreviewState) => {
    current.image.bitmap.close();
    setPreview(null);
  };

  const cancel = () => {
    if (preview === null || applying) return;
    close(preview);
    busy.current = false;
  };

  /** replaced, a touch-up result standing for the picked image, is closed with it. */
  const apply = (fit: Fit, replaced: UploadImage | null) => {
    if (preview === null || applying) return;
    const current = preview;
    const started = generation.current;
    setApplying(true);
    encodeComposition(replaced ?? current.image, current.aspect, fit)
      .then((composed) => {
        replaced?.bitmap.close();
        if (generation.current !== started) return;
        close(current);
        send(new File([composed], "thumbnail.png", { type: "image/png" }), current);
      })
      .catch(() => {
        replaced?.bitmap.close();
        if (generation.current !== started) return;
        close(current);
        busy.current = false;
        report(text.failed(t.errors.unexpected));
      })
      .finally(() => {
        if (generation.current === started) setApplying(false);
      });
  };

  const send = (image: File, current: UploadPreviewState) => {
    const mark = detailMark();
    const started = generation.current;
    const shown = () => generation.current === started;
    setSending(true);
    uploadThumbnailImage(current.videoId, image, current.contentVersion)
      .then(
        (saved) => {
          if (shown()) onChanged?.(saved, mark);
        },
        (error: unknown) => {
          if (isAborted(error) || !shown()) return;
          report(text.failed(errorText(error)));
          if (staleAfter(error)) onStale?.();
        },
      )
      .finally(() => {
        if (!shown()) return;
        busy.current = false;
        setSending(false);
      });
  };

  return { sending, preparing, applying, preview, choose, chooseSource, apply, cancel };
}

async function loadSource(
  videoId: number,
  start: ThumbnailEditStart,
): Promise<LoadResult> {
  const session = await startThumbnailEdit(videoId, start);
  try {
    return await loadUploadImage(await getThumbnailEditFrame(session.id));
  } finally {
    void discardThumbnailEdit(session.id).catch(() => undefined);
  }
}

function staleAfter(error: unknown): boolean {
  if (!(error instanceof RequestFailed)) return false;
  return error.status === 404 || error.reason === "content_changed";
}

/** One entry for every way to change the thumbnail: the frame on screen, the current thumbnail or an image file, each opening the preview; an image file is chosen, dropped or pasted in an ImageUploadDialog. The frame's position is read when the menu opens. Locked actions retain focus but never open the menu. */
export default function ThumbnailEntry({
  sending,
  locked,
  capture,
  thumbnailUrl,
  onFile,
  onSource,
}: {
  sending: boolean;
  locked: boolean;
  capture?: ThumbnailCapture;
  thumbnailUrl: string | undefined;
  onFile: (file: File) => void;
  onSource: (start: ThumbnailEditStart, name: string) => void;
}) {
  const text = t.player.thumbnailUpload;
  const edit = t.player.thumbnailEdit;
  const [uploading, setUploading] = useState(false);
  const [positionMs, setPositionMs] = useState<number | null>(null);
  const [open, setOpen] = useState(false);
  const disabled = sending || locked;

  return (
    <>
      <DropdownMenu
        open={open && !disabled}
        onOpenChange={(next) => {
          if (next) setPositionMs(framePosition(capture));
          setOpen(next);
        }}
      >
        <DropdownMenuTrigger asChild>
          <ActionButton
            label={text.entry}
            aria-disabled={disabled || undefined}
            aria-busy={sending || undefined}
            className="aria-disabled:cursor-default data-[state=open]:bg-accent"
          >
            {sending ? (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            ) : (
              <ImageUp aria-hidden="true" />
            )}
          </ActionButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            disabled={positionMs === null}
            onSelect={() => {
              if (positionMs === null) return;
              setOpen(false);
              capture?.pause();
              const name = edit.frameAt(formatDuration(positionMs));
              onSource({ source: "frame", positionMs }, name);
            }}
          >
            <Film aria-hidden="true" />
            {positionMs === null ? (
              edit.frameOnScreen
            ) : (
              <span className="tabular-nums">
                {edit.frameAt(formatDuration(positionMs))}
              </span>
            )}
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              setOpen(false);
              onSource({ source: "thumbnail" }, edit.currentThumbnail);
            }}
          >
            <ThumbnailImage url={thumbnailUrl} />
            {edit.currentThumbnail}
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              setOpen(false);
              setUploading(true);
            }}
          >
            <FileImage aria-hidden="true" />
            {text.chooseFile}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ThumbnailUploadDialog
        open={uploading && !disabled}
        onOpenChange={setUploading}
        onFile={onFile}
      />
    </>
  );
}

/** The ImageUploadDialog every thumbnail entry opens; it refuses a wrong type or size in place and closes on an accepted file. */
export function ThumbnailUploadDialog({
  open,
  onOpenChange,
  onFile,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onFile: (file: File) => void;
}) {
  const text = t.player.thumbnailUpload;
  const take = (file: File) => {
    const refusal = uploadRefusal(file);
    if (refusal !== null) return refusal === "type" ? text.refusedType : text.refusedSize;
    onOpenChange(false);
    onFile(file);
    return null;
  };
  return (
    <ImageUploadDialog
      open={open}
      onOpenChange={onOpenChange}
      title={text.uploadTitle}
      description={text.uploadHint}
      accept={UPLOAD_TYPES}
      onFile={take}
    />
  );
}

/** Page shortcuts wait while the upload preview is open. */
export function usePreviewEditing(
  previewOpen: boolean,
  onEditingChange: ((open: boolean) => void) | undefined,
): void {
  const editingChange = useRef(onEditingChange);
  editingChange.current = onEditingChange;
  useEffect(() => {
    if (!previewOpen) return;
    editingChange.current?.(true);
    return () => editingChange.current?.(false);
  }, [previewOpen]);
}

/** The preview for the upload in progress. framingOnly leaves out the AI touch-up, which starts a thumbnail edit. */
export function UploadPreviewDialog({
  video,
  upload,
  framingOnly = false,
}: {
  video: Video;
  upload: ReturnType<typeof useThumbnailUpload>;
  framingOnly?: boolean;
}) {
  if (upload.preview === null) return null;
  return (
    <UploadPreview
      videoId={video.id}
      image={upload.preview.image}
      name={upload.preview.name}
      aspect={upload.preview.aspect}
      narrow={isNarrowVideo(video)}
      applying={upload.applying}
      framingOnly={framingOnly}
      onApply={upload.apply}
      onCancel={upload.cancel}
    />
  );
}

function framePosition(capture: ThumbnailCapture | undefined): number | null {
  if (capture === undefined || !capture.enabled) return null;
  const raw = capture.positionMs();
  if (raw === null || !Number.isFinite(raw)) return null;
  return Math.max(0, Math.round(raw));
}

/** Use empty alt because the menu item supplies the name; preserve an empty box while loading. */
function ThumbnailImage({ url }: { url: string | undefined }) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  return (
    <span className="aspect-video h-6 shrink-0 overflow-hidden rounded-sm bg-card">
      {url !== undefined && url !== brokenUrl && (
        <img
          src={url}
          alt=""
          onError={() => setBrokenUrl(url)}
          className="size-full object-cover"
        />
      )}
    </span>
  );
}
