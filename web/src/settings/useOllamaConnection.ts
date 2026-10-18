import { useCallback, useEffect, useRef, useState } from "react";

import { getOllamaSettings, isAborted } from "../api/client";
import { errorText, type UiText } from "../i18n";

/** A failed connection read leaves saved/unsaved unknown. */
export type OllamaConnection =
  | { status: "loading" }
  | { status: "saved" }
  | { status: "unsaved" }
  | { status: "failed"; reason: UiText };

/** Read once on page opening; retry explicitly rereads. */
export function useOllamaConnection(): {
  connection: OllamaConnection;
  retry: () => void;
} {
  const [connection, setConnection] = useState<OllamaConnection>({ status: "loading" });
  const controller = useRef<AbortController | null>(null);

  const read = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    try {
      const settings = await getOllamaSettings(abort.signal);
      if (abort.signal.aborted) return;
      setConnection({ status: settings.saved ? "saved" : "unsaved" });
    } catch (failure) {
      if (isAborted(failure)) return;
      setConnection({ status: "failed", reason: errorText(failure) });
    }
  }, []);

  useEffect(() => {
    void read();
    return () => controller.current?.abort();
  }, [read]);

  const retry = useCallback(() => {
    setConnection({ status: "loading" });
    void read();
  }, [read]);
  return { connection, retry };
}
