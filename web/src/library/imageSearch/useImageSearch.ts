import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";

import { isAborted } from "../../api/client";
import { searchByImage } from "../../api/imageSearch";
import type { UiText } from "../../i18n";
import { returnEntry } from "../../player/pageDecisions";
import {
  forget,
  type ImageSearchPhase,
  recall,
  refuseError,
  refuseFiles,
  rememberCollapsed,
  rememberPhase,
  rememberScroll,
} from "./imageSearchState";

export interface ImageSearch {
  /** The history entry the search is kept under; a result passes it to the player to come back to. */
  entry: string;
  /** The search the page shows instead of the library, or null for the library. */
  phase: ImageSearchPhase | null;
  /** The scroll position to restore when the search came back from history. */
  restoredScrollY: number | undefined;
  /** The result rows collapsed when the search came back from history, by row key. */
  restoredCollapsed: ReadonlySet<string> | undefined;
  /** Searches with the first file, or refuses the files without a request. */
  search: (files: readonly File[]) => void;
  /** The dialog's `onFile`: a file refused in the browser keeps the dialog open with the reason. */
  takeDialogFile: (file: File) => UiText | null;
  /** Ends the image search and aborts its request; the library shows again. */
  clear: () => void;
  retry: () => void;
  /** Records the scroll position before a result opens the player. */
  keepScroll: () => void;
  /** Records which result rows are collapsed, so coming back from the player shows them the same. */
  keepCollapsed: (collapsed: ReadonlySet<string>) => void;
  dialogOpen: boolean;
  setDialogOpen: (open: boolean) => void;
}

interface Shown {
  key: string;
  phase: ImageSearchPhase | null;
  scrollY: number | undefined;
  collapsed: ReadonlySet<string> | undefined;
}

function recalled(key: string, enabled: boolean): Shown {
  const entry = enabled ? recall(key) : undefined;
  return {
    key,
    phase: entry?.phase ?? null,
    scrollY: entry?.scrollY,
    collapsed: entry?.collapsed,
  };
}

/**
 * The owner's image search on the library page. It lives in memory under the history entry, so
 * returning from the player restores it without a request, and leaving the page or the entry aborts
 * a running request. Closing the player pushes a new entry that names the one it came from.
 */
export function useImageSearch(enabled: boolean): ImageSearch {
  const location = useLocation();
  const key = returnEntry(location.state) ?? location.key;
  const [shown, setShown] = useState(() => recalled(key, enabled));
  if (shown.key !== key) setShown(recalled(key, enabled));
  const [dialogOpen, setDialogOpen] = useState(false);
  const running = useRef<AbortController | null>(null);

  const abort = useCallback(() => {
    running.current?.abort();
    running.current = null;
  }, []);
  useEffect(() => abort, [abort, key]);

  const show = useCallback(
    (phase: ImageSearchPhase | null) => {
      if (phase === null || phase.kind === "searching") forget(key);
      else rememberPhase(key, phase);
      setShown({ key, phase, scrollY: undefined, collapsed: undefined });
    },
    [key],
  );

  const send = useCallback(
    (file: File) => {
      abort();
      const controller = new AbortController();
      running.current = controller;
      show({ kind: "searching", file });
      searchByImage(file, controller.signal).then(
        (result) => {
          if (controller.signal.aborted) return;
          running.current = null;
          show({ kind: "results", file, result });
        },
        (error: unknown) => {
          if (controller.signal.aborted || isAborted(error)) return;
          running.current = null;
          show({ kind: "refused", file, refusal: refuseError(error) });
        },
      );
    },
    [abort, show],
  );

  const search = useCallback(
    (files: readonly File[]) => {
      const first = files[0];
      if (first === undefined) return;
      const refusal = refuseFiles(files);
      if (refusal === null) {
        send(first);
        return;
      }
      abort();
      show({ kind: "refused", file: first, refusal });
    },
    [abort, send, show],
  );

  const takeDialogFile = useCallback(
    (file: File) => {
      const refusal = refuseFiles([file]);
      if (refusal !== null) return refusal.title;
      setDialogOpen(false);
      send(file);
      return null;
    },
    [send],
  );

  const clear = useCallback(() => {
    abort();
    show(null);
  }, [abort, show]);

  const { phase } = shown;
  const retry = useCallback(() => {
    if (phase !== null) send(phase.file);
  }, [phase, send]);
  const keepScroll = useCallback(() => rememberScroll(key, window.scrollY), [key]);
  const keepCollapsed = useCallback(
    (collapsed: ReadonlySet<string>) => rememberCollapsed(key, collapsed),
    [key],
  );

  return {
    entry: key,
    phase,
    restoredScrollY: shown.scrollY,
    restoredCollapsed: shown.collapsed,
    search,
    takeDialogFile,
    clear,
    retry,
    keepScroll,
    keepCollapsed,
    dialogOpen,
    setDialogOpen,
  };
}
