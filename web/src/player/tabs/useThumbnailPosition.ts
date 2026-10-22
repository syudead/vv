import { useEffect, useRef, useState } from "react";

import {
  isAborted,
  RequestFailed,
  revertThumbnailImage,
  setVideoThumbnailPosition,
  type Video,
} from "../../api/client";
import { detailMark, type DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";

export type ThumbnailSending = "capture" | "clear" | "revert" | null;

/** Serialize thumbnail mutations; refresh stale videos on 404. Return successful video data with the detail mark captured before submission. */
export function useThumbnailPosition(
  videoId: number,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: ThumbnailSending;
  send: (positionMs: number | null | "revert") => void;
} {
  const [sending, setSending] = useState<ThumbnailSending>(null);
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
