import { useCallback, useEffect, useRef, useState } from "react";

import { openVideoFile } from "../api/client";
import { errorText, t, type UiText } from "../i18n";

export function useOpenFile(videoId: number): {
  open: () => void;
  failure: UiText | null;
} {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [videoId]);
  const open = useOpenFileRequest(videoId, setFailure);
  return { open, failure };
}

/** Report null when starting and the error on failure, allowing callers to share one error row. */
export function useOpenFileRequest(
  videoId: number,
  report: (failure: UiText | null) => void,
): () => void {
  const request = useRef<AbortController | null>(null);

  useEffect(() => () => request.current?.abort(), [videoId]);

  return useCallback(() => {
    report(null);
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    void openVideoFile(videoId, controller.signal).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      report(t.player.facts.openFailed(errorText(error)));
    });
  }, [report, videoId]);
}
