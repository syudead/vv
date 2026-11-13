import { useCallback, useEffect, useState } from "react";

import { isAborted } from "../../api/client";
import { searchByImage } from "../../api/imageSearch";
import {
  type ImageSearchPhase,
  recall,
  refuseError,
  refuseFiles,
  rememberPhase,
  type SearchEntry,
} from "./imageSearchState";

export interface ImageSearch {
  /** The search's token; a result passes it to the player to come back to. Undefined while Idle. */
  token: string | undefined;
  /** The search the screen shows, or null for Idle. */
  phase: ImageSearchPhase | null;
  /** The kept view when the search came back from history with its answer, else undefined. */
  restored: SearchEntry | undefined;
  /** Sends the same file again after a failure. */
  retry: () => void;
}

interface Shown {
  token: string | undefined;
  phase: ImageSearchPhase | null;
  restored: SearchEntry | undefined;
}

/** The first phase of given files: a refusal decided in the browser, else searching the first file. */
function firstPhase(files: readonly File[]): ImageSearchPhase | null {
  const file = files[0];
  if (file === undefined) return null;
  const refusal = refuseFiles(files);
  return refusal === null
    ? { kind: "searching", file }
    : { kind: "refused", file, refusal };
}

function opened(token: string | undefined): Shown {
  const entry = token === undefined ? undefined : recall(token);
  if (entry === undefined) return { token, phase: null, restored: undefined };
  if (entry.phase !== undefined) return { token, phase: entry.phase, restored: entry };
  return { token, phase: firstPhase(entry.files), restored: undefined };
}

/**
 * The search an `/image-search` entry shows. It lives in memory under the entry's token, so coming back
 * from the player shows it without a request; a reload or a direct visit has no token and shows Idle.
 * Leaving the entry or the screen aborts a running request.
 */
export function useImageSearch(token: string | undefined): ImageSearch {
  const [shown, setShown] = useState(() => opened(token));
  if (shown.token !== token) setShown(opened(token));
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const entry = token === undefined ? undefined : recall(token);
    if (token === undefined || entry === undefined || entry.phase !== undefined) return;
    const phase = firstPhase(entry.files);
    if (phase === null) return;
    if (phase.kind !== "searching") {
      rememberPhase(token, phase);
      return;
    }
    const controller = new AbortController();
    const show = (next: ImageSearchPhase) => {
      rememberPhase(token, next);
      setShown({ token, phase: next, restored: undefined });
    };
    searchByImage(phase.file, controller.signal).then(
      (result) => {
        if (!controller.signal.aborted)
          show({ kind: "results", file: phase.file, result });
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        show({ kind: "refused", file: phase.file, refusal: refuseError(error) });
      },
    );
    return () => controller.abort();
  }, [token, attempt]);

  const { phase } = shown;
  const retry = useCallback(() => {
    if (token === undefined || phase === null) return;
    const searching: ImageSearchPhase = { kind: "searching", file: phase.file };
    rememberPhase(token, searching);
    setShown({ token, phase: searching, restored: undefined });
    setAttempt((count) => count + 1);
  }, [phase, token]);

  return { token, phase, restored: shown.restored, retry };
}
