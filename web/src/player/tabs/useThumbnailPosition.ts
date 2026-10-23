import { useEffect, useRef, useState } from "react";

import {
  isAborted,
  RequestFailed,
  setVideoThumbnailPosition,
  type Video,
} from "../../api/client";
import { detailMark, type DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";

export type ThumbnailSending = "capture" | "clear" | null;

/**
 * Serialize thumbnail position changes; refresh stale videos on 404. A null position clears both
 * the applied image and the recorded position in one request. Successful video data comes back
 * with the detail mark captured before submission.
 */
export function useThumbnailPosition(
  videoId: number,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: ThumbnailSending;
  send: (positionMs: number | null) => void;
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

  const send = (positionMs: number | null) => {
    if (busy.current) return;
    busy.current = true;
    const controller = new AbortController();
    request.current = controller;
    const mark = detailMark();
    report(null);
    setSending(positionMs === null ? "clear" : "capture");
    setVideoThumbnailPosition(videoId, positionMs, controller.signal)
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
