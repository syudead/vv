import { FileImage, Film, ImageUp, LoaderCircle } from "lucide-react";
import { type ChangeEvent, useEffect, useRef, useState } from "react";

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
import { formatDuration } from "../../lib/format";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Input } from "../../ui/shadcn/input";
import { ImageDropOverlay, useImageIntake } from "../../ui/ImageDrop";
import ActionButton from "../ActionButton";
import type { ThumbnailCapture } from "../VideoFacts";
import {
  encodeComposition,
  type LoadResult,
  loadUploadImage,
  type UploadImage,
} from "./composeImage";
import { type Fit, frameAspect } from "./framing";
import { UPLOAD_TYPES, uploadRefusal } from "./uploadRules";

export interface UploadPreviewState {
  image: UploadImage;
  name: string;
  aspect: number | null;
  /** Apply only to the content version captured when opening the preview. */
  contentVersion: string;
}

/** Validate and preview before uploading with the captured contentVersion. Uploads continue after leaving, but their responses must not update the departed page. */
export function useThumbnailUpload(
  video: Video,
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
  const live = useRef(true);
  // Do not open a decoded image for a video that is no longer current.
  const videoId = useRef(video.id);
  videoId.current = video.id;

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  useEffect(() => {
    setPreview((current) => {
      if (current !== null) {
        current.image.bitmap.close();
        busy.current = false;
      }
      return null;
    });
  }, [video.id]);

  const text = t.player.thumbnailUpload;

  /** Decode in the background and open the preview unless the page or the video changed meanwhile. */
  const open = (
    name: string,
    load: Promise<LoadResult>,
    failed: (error: unknown) => void,
  ) => {
    busy.current = true;
    report(null);
    setPreparing(true);
    const opened = {
      id: video.id,
      aspect: frameAspect(video),
      contentVersion: video.contentVersion ?? "",
    };
    load
      .then((loaded) => {
        if (!live.current || videoId.current !== opened.id) {
          if ("image" in loaded) loaded.image.bitmap.close();
          busy.current = false;
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
        if (live.current && !isAborted(error)) failed(error);
        busy.current = false;
      })
      .finally(() => {
        if (live.current) setPreparing(false);
      });
  };

  const choose = (file: File) => {
    if (busy.current) return;
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      report(refusal === "type" ? text.refusedType : text.refusedSize);
      return;
    }
    open(file.name, loadUploadImage(file), () =>
      report(text.failed(t.errors.reason.thumbnail_upload_invalid)),
    );
  };

  /** A frame or the current thumbnail, read through a short-lived edit session at its full size. */
  const chooseSource = (start: ThumbnailEditStart, name: string) => {
    if (busy.current) return;
    open(name, loadSource(video.id, start), (error) => {
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
    setApplying(true);
    encodeComposition(replaced ?? current.image, current.aspect, fit)
      .then((composed) => {
        replaced?.bitmap.close();
        if (!live.current) return;
        close(current);
        send(new File([composed], "thumbnail.png", { type: "image/png" }), current);
      })
      .catch(() => {
        replaced?.bitmap.close();
        if (!live.current) return;
        close(current);
        busy.current = false;
        report(text.failed(t.errors.unexpected));
      })
      .finally(() => {
        if (live.current) setApplying(false);
      });
  };

  const send = (image: File, current: UploadPreviewState) => {
    const mark = detailMark();
    setSending(true);
    uploadThumbnailImage(videoId.current, image, current.contentVersion)
      .then(
        (saved) => {
          if (live.current) onChanged?.(saved, mark);
        },
        (error: unknown) => {
          if (isAborted(error) || !live.current) return;
          report(text.failed(errorText(error)));
          if (staleAfter(error)) onStale?.();
        },
      )
      .finally(() => {
        busy.current = false;
        if (live.current) setSending(false);
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

/** One entry for every way to change the thumbnail: the frame on screen, the current thumbnail or an image file, each opening the preview. An image pasted or dropped on the page opens it too. The frame's position is read when the menu opens. Locked actions retain focus but never open the menu. */
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
  const fileInput = useRef<HTMLInputElement | null>(null);
  const [positionMs, setPositionMs] = useState<number | null>(null);
  const [open, setOpen] = useState(false);
  const disabled = sending || locked;
  const { dragging } = useImageIntake({ enabled: !disabled, onFile, scope: "page" });

  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file !== undefined) onFile(file);
  };

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
              fileInput.current?.click();
            }}
          >
            <FileImage aria-hidden="true" />
            {text.chooseFile}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
      <ImageDropOverlay dragging={dragging} label={text.drop} />
    </>
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
