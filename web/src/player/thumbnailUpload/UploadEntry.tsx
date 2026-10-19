import { ImageUp, LoaderCircle } from "lucide-react";
import { type ChangeEvent, useEffect, useRef, useState } from "react";

import {
  isAborted,
  RequestFailed,
  uploadThumbnailImage,
  type Video,
} from "../../api/client";
import { detailMark, type DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { Input } from "../../ui/shadcn/input";
import ActionButton from "../ActionButton";
import { encodeComposition, loadUploadImage, type UploadImage } from "./composeImage";
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

  const choose = (file: File) => {
    if (busy.current) return;
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      report(refusal === "type" ? text.refusedType : text.refusedSize);
      return;
    }
    busy.current = true;
    report(null);
    setPreparing(true);
    const opened = {
      id: video.id,
      aspect: frameAspect(video),
      contentVersion: video.contentVersion ?? "",
    };
    loadUploadImage(file)
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
        setPreview({ image: loaded.image, name: file.name, ...opened });
      })
      .catch(() => {
        if (live.current) report(text.failed(t.errors.reason.thumbnail_upload_invalid));
        busy.current = false;
      })
      .finally(() => {
        if (live.current) setPreparing(false);
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

  return { sending, preparing, applying, preview, choose, apply, cancel };
}

function staleAfter(error: unknown): boolean {
  if (!(error instanceof RequestFailed)) return false;
  return error.status === 404 || error.reason === "content_changed";
}

/** Reset the file input after selection so the same file can be chosen again. Locked actions retain focus but never open the picker. */
export default function UploadEntry({
  sending,
  locked,
  onFile,
}: {
  sending: boolean;
  locked: boolean;
  onFile: (file: File) => void;
}) {
  const fileInput = useRef<HTMLInputElement | null>(null);
  const disabled = sending || locked;

  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file !== undefined) onFile(file);
  };

  return (
    <>
      <ActionButton
        label={t.player.thumbnailUpload.entry}
        aria-disabled={disabled || undefined}
        onClick={() => {
          if (!disabled) fileInput.current?.click();
        }}
        className="aria-disabled:cursor-default"
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
    </>
  );
}
