import { useCallback, useEffect, useState } from "react";

import type { Video } from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import type { UiText } from "../../i18n";
import { useThumbnailUpload } from "../thumbnailUpload/UploadEntry";

export interface VirtualThumbnail {
  /** The page's video while it is a virtual one; undefined for a file video or while loading. */
  video: Video | undefined;
  /** The facts row's one error row, shared by its actions and the frame's upload button. */
  failure: UiText | null;
  report: (failure: UiText | null) => void;
  upload: ReturnType<typeof useThumbnailUpload>;
  /** One upload at a time: the dialog waits while an image is read or sent. */
  busy: boolean;
  /** The upload dialog is open; busy keeps it closed. */
  picking: boolean;
  setPicking: (open: boolean) => void;
  pick: () => void;
}

/**
 * Upload state for a virtual video's page, which has two entries to one upload dialog: `Change
 * thumbnail` in the facts row and `Upload an image…` in the media frame. It stays idle for a file
 * video.
 */
export function useVirtualThumbnail(
  page: Video | undefined,
  onChanged: (video: Video, mark: DetailMark) => void,
  onStale: () => void,
): VirtualThumbnail {
  const video = page?.virtual === true ? page : undefined;
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [video?.id]);
  const upload = useThumbnailUpload(video, setFailure, onChanged, onStale);
  const busy = upload.sending || upload.preparing;
  const [picking, setPicking] = useState(false);
  const pick = useCallback(() => {
    if (!busy) setPicking(true);
  }, [busy]);
  return {
    video,
    failure,
    report: setFailure,
    upload,
    busy,
    picking: picking && !busy,
    setPicking,
    pick,
  };
}
