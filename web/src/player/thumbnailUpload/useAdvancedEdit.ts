import { useEffect, useRef, useState } from "react";

import {
  discardThumbnailEdit,
  startThumbnailEditFromImage,
  type ThumbnailEdit,
} from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { loadUploadImage, type UploadImage } from "./composeImage";

/** The ComfyUI editor, with painting and a reference image, opened on the preview's framing; its result comes back to the preview as a decoded image. */
export function useAdvancedEdit(
  videoId: number,
  onResult: (image: UploadImage) => void,
): {
  edit: ThumbnailEdit | null;
  opening: boolean;
  failure: UiText | null;
  open: (framing: Promise<Blob>) => void;
  use: (result: Blob) => void;
  close: () => void;
} {
  const [edit, setEdit] = useState<ThumbnailEdit | null>(null);
  const [opening, setOpening] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  const text = t.player.thumbnailEdit;

  const open = (framing: Promise<Blob>) => {
    if (opening) return;
    setOpening(true);
    setFailure(null);
    framing
      .then((image) => startThumbnailEditFromImage(videoId, image))
      .then(
        (session) => {
          if (live.current) setEdit(session);
          else void discardThumbnailEdit(session.id).catch(() => undefined);
        },
        (error: unknown) => {
          if (live.current) setFailure(text.startFailed(errorText(error)));
        },
      )
      .finally(() => {
        if (live.current) setOpening(false);
      });
  };

  const use = (result: Blob) => {
    loadUploadImage(result).then(
      (loaded) => {
        if (!live.current) {
          if ("image" in loaded) loaded.image.bitmap.close();
          return;
        }
        if ("image" in loaded) onResult(loaded.image);
        else setFailure(text.applyFailed(t.errors.reason[loaded.refusal]));
      },
      () => {
        if (live.current) setFailure(text.applyFailed(t.errors.unexpected));
      },
    );
  };

  return { edit, opening, failure, open, use, close: () => setEdit(null) };
}
