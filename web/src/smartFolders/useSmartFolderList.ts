import { useCallback, useEffect, useRef, useState } from "react";

import type { SmartFolderSummary } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { listSmartFolders } from "../api/smartFolders";
import { errorText } from "../i18n";

export type SmartFolderList =
  | { kind: "loading" }
  | { kind: "content"; folders: readonly SmartFolderSummary[] }
  | { kind: "failed"; reason: string };

const loading: SmartFolderList = { kind: "loading" };

/**
 * The owner's whole smart folder list in the server's name order, read again in place on every
 * `smartFolder` and `peerItems` event and on reconnection: a favorite change names every folder,
 * and a link that connects or drops changes the merged list without a `smartFolder` event.
 */
export function useSmartFolderList(enabled: boolean) {
  const [state, setState] = useState<SmartFolderList>(loading);
  const current = useRef<AbortController | null>(null);

  const read = useCallback(() => {
    current.current?.abort();
    const controller = new AbortController();
    current.current = controller;
    listSmartFolders(controller.signal).then(
      (folders) => {
        if (controller.signal.aborted) return;
        setState({ kind: "content", folders });
      },
      (error: unknown) => {
        if (controller.signal.aborted) return;
        setState({ kind: "failed", reason: errorText(error) });
      },
    );
  }, []);

  useEffect(() => {
    if (!enabled) {
      setState(loading);
      return;
    }
    read();
    const unsubscribe = subscribeServerEvents({
      smartFolder: read,
      peerItems: read,
      open: (reconnected) => {
        if (reconnected) read();
      },
    });
    return () => {
      unsubscribe();
      current.current?.abort();
      current.current = null;
    };
  }, [enabled, read]);

  /** Back to the loading state, then read; a re-read after an event keeps the shown list. */
  const reload = useCallback(() => {
    setState(loading);
    read();
  }, [read]);

  return { state, read, reload };
}
