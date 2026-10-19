import { useEffect, useRef, useState } from "react";

import {
  discardThumbnailEdit,
  getThumbnailEdit,
  getThumbnailEditReadiness,
  getThumbnailEditResult,
  isAborted,
  runThumbnailEdit,
  startThumbnailEditFromImage,
  type ThumbnailEdit,
} from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { isRunning } from "../thumbnailEdit/editRules";
import { failureReasonText } from "../thumbnailEdit/editSession";
import { loadUploadImage, type UploadImage } from "./composeImage";

export const POLL_MS = 1000;

export type TouchUp =
  | { status: "idle" }
  | { status: "running"; session: ThumbnailEdit | null }
  | { status: "result"; image: UploadImage }
  | { status: "failed"; message: UiText }
  | { status: "cancelled" };

/** A ComfyUI run over the preview's framing: the session lives only for one run and is discarded once its output is read, so the result joins the preview like an uploaded image. */
export function useAiTouchUp(videoId: number): {
  /** null until the readiness answer arrives. */
  ready: boolean | null;
  state: TouchUp;
  /** framing is the drawing still being encoded; a failure to encode fails the run. */
  run: (framing: Promise<Blob>, prompt: string) => void;
  cancel: () => void;
  /** Hand the result to the caller, which then owns its bitmap. */
  take: () => UploadImage | null;
  dismiss: () => void;
} {
  const [ready, setReady] = useState<boolean | null>(null);
  const [state, setState] = useState<TouchUp>({ status: "idle" });
  const controller = useRef<AbortController | null>(null);
  const editId = useRef<string | null>(null);
  // The last start request, which a new one waits for so a late start never replaces it.
  const starts = useRef<Promise<unknown>>(Promise.resolve());
  const result = useRef<UploadImage | null>(null);

  useEffect(() => {
    const readiness = new AbortController();
    getThumbnailEditReadiness(readiness.signal).then(
      (answer) => setReady(answer.connection),
      () => setReady(false),
    );
    return () => {
      readiness.abort();
      controller.current?.abort();
      discard(editId);
      result.current?.bitmap.close();
    };
  }, []);

  const show = (next: TouchUp) => {
    if (next.status !== "result") {
      result.current?.bitmap.close();
      result.current = null;
    }
    setState(next);
  };

  const run = (framing: Promise<Blob>, prompt: string) => {
    controller.current?.abort();
    discard(editId);
    const current = new AbortController();
    controller.current = current;
    show({ status: "running", session: null });
    touchUp(videoId, framing, prompt, current.signal, { editId, starts }, (session) =>
      show({ status: "running", session }),
    )
      .then((outcome) => {
        if (current.signal.aborted) {
          if ("image" in outcome) outcome.image.bitmap.close();
          return;
        }
        if ("image" in outcome) {
          result.current = outcome.image;
          setState({ status: "result", image: outcome.image });
        } else show(outcome);
      })
      .catch((error: unknown) => {
        if (current.signal.aborted || isAborted(error)) return;
        show({
          status: "failed",
          message: t.player.thumbnailEdit.runFailed(errorText(error)),
        });
      })
      .finally(() => {
        if (controller.current === current) discard(editId);
      });
  };

  return {
    ready,
    state,
    run,
    cancel: () => {
      controller.current?.abort();
      discard(editId);
      show({ status: "cancelled" });
    },
    take: () => {
      const image = result.current;
      result.current = null;
      setState({ status: "idle" });
      return image;
    },
    dismiss: () => show({ status: "idle" }),
  };
}

/** Discarding also cancels a run in progress at the provider. */
function discard(editId: { current: string | null }): void {
  if (editId.current === null) return;
  void discardThumbnailEdit(editId.current).catch(() => undefined);
  editId.current = null;
}

async function touchUp(
  videoId: number,
  framing: Promise<Blob>,
  prompt: string,
  signal: AbortSignal,
  refs: { editId: { current: string | null }; starts: { current: Promise<unknown> } },
  onSession: (session: ThumbnailEdit) => void,
): Promise<
  { image: UploadImage } | Extract<TouchUp, { status: "failed" | "cancelled" }>
> {
  const image = await framing;
  await refs.starts.current.catch(() => undefined);
  throwIfAborted(signal);
  // Not aborted with the run: the server may create the session anyway, and only its answer names it for discarding.
  const start = startThumbnailEditFromImage(videoId, image);
  refs.starts.current = start;
  const started = await start;
  if (signal.aborted) {
    void discardThumbnailEdit(started.id).catch(() => undefined);
    throwIfAborted(signal);
  }
  refs.editId.current = started.id;
  let session = await runThumbnailEdit(started.id, { prompt }, signal);
  onSession(session);
  while (isRunning(session.state)) {
    await sleep(POLL_MS, signal);
    session = await getThumbnailEdit(started.id, signal);
    onSession(session);
  }
  const text = t.player.thumbnailEdit;
  if (session.state === "cancelled") return { status: "cancelled" };
  if (session.state !== "succeeded")
    return {
      status: "failed",
      message: text.runFailed(failureReasonText(session.error?.reason)),
    };
  const loaded = await loadUploadImage(await getThumbnailEditResult(started.id, signal));
  if ("refusal" in loaded)
    return { status: "failed", message: text.runFailed(t.errors.reason[loaded.refusal]) };
  return loaded;
}

function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) throw new DOMException("aborted", "AbortError");
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(new DOMException("aborted", "AbortError"));
      },
      { once: true },
    );
  });
}
