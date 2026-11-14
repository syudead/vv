import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../../api/client";
import { searchByImage } from "../../api/imageSearch";
import {
  errorPhase,
  type ImageSearchPhase,
  isRunning,
  recall,
  refuseFiles,
  rememberPhase,
  type RunningPhase,
  type SearchEntry,
} from "./imageSearchState";
import { fileKind } from "./useFilePreview";

export interface ImageSearch {
  /** The search's token; a result passes it to the player to come back to. Undefined while Idle. */
  token: string | undefined;
  /** The search the screen shows, or null for Idle. */
  phase: ImageSearchPhase | null;
  /** The kept view when the search came back from history with its answer, else undefined. */
  restored: SearchEntry | undefined;
  /** Sends the same file again: Retry after a failure, Search again after a stop. */
  retry: () => void;
  /** Stops the running search, which then shows as stopped. */
  cancel: () => void;
}

interface Shown {
  token: string | undefined;
  phase: ImageSearchPhase | null;
  restored: SearchEntry | undefined;
}

/** A video is uploaded with its progress; an image goes straight to searching, so no bar flashes by. */
function startPhase(file: File, startedAt: number): RunningPhase {
  return fileKind(file) === "video"
    ? { kind: "uploading", file, startedAt, sent: 0 }
    : { kind: "searching", file, startedAt };
}

/** The first phase of given files: a refusal decided in the browser, else the search of the first file. */
function firstPhase(files: readonly File[], startedAt: number): ImageSearchPhase | null {
  const file = files[0];
  if (file === undefined) return null;
  const refusal = refuseFiles(files);
  return refusal === null
    ? startPhase(file, startedAt)
    : { kind: "refused", file, refusal };
}

function opened(token: string | undefined): Shown {
  const entry = token === undefined ? undefined : recall(token);
  if (entry === undefined) return { token, phase: null, restored: undefined };
  if (entry.phase !== undefined) return { token, phase: entry.phase, restored: entry };
  return { token, phase: firstPhase(entry.files, Date.now()), restored: undefined };
}

/**
 * The upload's progress as phases: the whole percent while it changes, then searching once every byte
 * is sent. Undefined for a search that does not upload with progress.
 */
function uploadProgress(
  start: RunningPhase,
  show: (phase: ImageSearchPhase) => void,
): ((sent: number) => void) | undefined {
  if (start.kind !== "uploading") return undefined;
  const { file, startedAt } = start;
  let percent = 0;
  let sentAll = false;
  return (sent) => {
    if (sentAll) return;
    if (sent >= 1) {
      sentAll = true;
      show({ kind: "searching", file, startedAt });
      return;
    }
    const next = Math.floor(sent * 100);
    if (next === percent) return;
    percent = next;
    show({ kind: "uploading", file, startedAt, sent });
  };
}

/**
 * The search an `/image-search` entry shows. It lives in memory under the entry's token, so coming back
 * from the player shows it without a request; a reload or a direct visit has no token and shows Idle.
 * Cancel, leaving the entry or leaving the screen aborts a running request.
 */
export function useImageSearch(token: string | undefined): ImageSearch {
  const [shown, setShown] = useState(() => opened(token));
  if (shown.token !== token) setShown(opened(token));
  const [attempt, setAttempt] = useState(0);
  const running = useRef<AbortController | null>(null);

  useEffect(() => {
    const entry = token === undefined ? undefined : recall(token);
    if (token === undefined || entry === undefined || entry.phase !== undefined) return;
    const startedAt = Date.now();
    const first = firstPhase(entry.files, startedAt);
    if (first === null) return;
    if (!isRunning(first)) {
      rememberPhase(token, first);
      return;
    }
    const controller = new AbortController();
    running.current = controller;
    const show = (next: ImageSearchPhase) => {
      if (controller.signal.aborted) return;
      rememberPhase(token, next);
      setShown({ token, phase: next, restored: undefined });
    };
    const { file } = first;
    searchByImage(file, controller.signal, uploadProgress(first, show)).then(
      (result) => show({ kind: "results", file, result, tookMs: Date.now() - startedAt }),
      (error: unknown) => {
        if (!isAborted(error)) show(errorPhase(file, error));
      },
    );
    return () => controller.abort();
  }, [token, attempt]);

  const { phase } = shown;
  const retry = useCallback(() => {
    if (token === undefined || phase === null) return;
    const start = startPhase(phase.file, Date.now());
    rememberPhase(token, start);
    setShown({ token, phase: start, restored: undefined });
    setAttempt((count) => count + 1);
  }, [phase, token]);

  const cancel = useCallback(() => {
    if (token === undefined || phase === null || !isRunning(phase)) return;
    running.current?.abort();
    const stopped: ImageSearchPhase = { kind: "stopped", file: phase.file };
    rememberPhase(token, stopped);
    setShown({ token, phase: stopped, restored: undefined });
  }, [phase, token]);

  return { token, phase, restored: shown.restored, retry, cancel };
}
