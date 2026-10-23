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
import {
  encodeComposition,
  type LoadResult,
  loadUploadImage,
  type UploadImage,
} from "./composeImage";
import { type Fit, frameAspect } from "./framing";
import { uploadRefusal } from "./uploadRules";

/** Which way opened the current preview: a picked file or a stored image read through a session. */
export type UploadFrom = "file" | "source";

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
  /** The way that started the last preparation or send, kept until the next one starts. */
  from: UploadFrom | null;
  preview: UploadPreviewState | null;
  choose: (file: File) => void;
  chooseSource: (start: ThumbnailEditStart, name: string) => void;
  apply: (fit: Fit, replaced: UploadImage | null) => void;
  cancel: () => void;
} {
  const [sending, setSending] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [applying, setApplying] = useState(false);
  const [from, setFrom] = useState<UploadFrom | null>(null);
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
    way: UploadFrom,
    name: string,
    load: Promise<LoadResult>,
    failed: (error: unknown) => void,
  ) => {
    busy.current = true;
    setFrom(way);
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
    open("file", file.name, loadUploadImage(file), () =>
      report(text.failed(t.errors.reason.thumbnail_upload_invalid)),
    );
  };

  /** A frame or the current thumbnail, read through a short-lived edit session at its full size. */
  const chooseSource = (start: ThumbnailEditStart, name: string) => {
    if (busy.current) return;
    open("source", name, loadSource(video.id, start), (error) => {
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

  return {
    sending,
    preparing,
    applying,
    from,
    preview,
    choose,
    chooseSource,
    apply,
    cancel,
  };
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
