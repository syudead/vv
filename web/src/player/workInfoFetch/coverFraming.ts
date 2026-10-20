import { useEffect, useRef, useState } from "react";

import type { Video } from "../../api/client";
import { getWorkInfoFetchCover, type WorkInfoFetch } from "../../api/workInfoPlugins";
import { loadUploadImage, type UploadImage } from "../thumbnailUpload/composeImage";
import { type Fit, frameAspect, WHOLE_IMAGE } from "../thumbnailUpload/framing";

/** A fetched cover decoded in the page so the owner frames it as an uploaded image. image is null while it loads or when the page cannot decode it; the server then fits the cover itself. */
export interface CoverFraming {
  image: UploadImage | null;
  aspect: number | null;
  fit: Fit;
  adjusting: boolean;
  adjust: () => void;
  /** replaced, a touch-up result, becomes the cover image from then on. */
  keep: (fit: Fit, replaced: UploadImage | null) => void;
  close: () => void;
}

interface Framing {
  key: string | null;
  fit: Fit;
  adjusting: boolean;
}

/** index is the chosen cover's position, or null while no cover is chosen. */
export function useCoverFraming(
  video: Video,
  result: WorkInfoFetch | null,
  index: number | null,
): CoverFraming {
  const videoId = video.id;
  const fetchId =
    result === null || index === null || index >= result.covers.length
      ? null
      : result.fetchId;
  const key =
    fetchId === null ? null : `${String(videoId)}\u0000${fetchId}\u0000${String(index)}`;
  const [loaded, setLoaded] = useState<{ key: string; image: UploadImage } | null>(null);
  // The image to close when the result changes: the decoded cover or a touch-up that replaced it.
  const held = useRef<UploadImage | null>(null);
  const [framing, setFraming] = useState<Framing>({
    key: null,
    fit: WHOLE_IMAGE,
    adjusting: false,
  });

  useEffect(() => {
    if (fetchId === null || index === null) return;
    const loadedKey = `${String(videoId)}\u0000${fetchId}\u0000${String(index)}`;
    const controller = new AbortController();
    getWorkInfoFetchCover(videoId, fetchId, index, controller.signal)
      .then(loadUploadImage)
      .then((decoded) => {
        if (!("image" in decoded)) return;
        if (controller.signal.aborted) {
          decoded.image.bitmap.close();
          return;
        }
        held.current = decoded.image;
        setLoaded({ key: loadedKey, image: decoded.image });
      })
      .catch(() => undefined);
    return () => {
      controller.abort();
      held.current?.bitmap.close();
      held.current = null;
    };
  }, [videoId, fetchId, index]);

  const image = loaded !== null && loaded.key === key ? loaded.image : null;
  // A new result or another cover starts again from the whole image.
  const current: Framing =
    framing.key === key ? framing : { key, fit: WHOLE_IMAGE, adjusting: false };
  return {
    image,
    aspect: frameAspect(video),
    fit: current.fit,
    adjusting: image !== null && current.adjusting,
    adjust: () => setFraming({ ...current, adjusting: true }),
    keep: (fit, replaced) => {
      if (replaced !== null && key !== null) {
        held.current?.bitmap.close();
        held.current = replaced;
        setLoaded({ key, image: replaced });
      }
      setFraming({ key, fit, adjusting: false });
    },
    close: () => setFraming({ ...current, adjusting: false }),
  };
}
