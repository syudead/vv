import { useCallback, useEffect, useRef, useState } from "react";

import { getOllamaSettings, isAborted } from "../api/client";
import { errorText, type UiText } from "../i18n";

/**
 * OllamaConnection は Ollama の接続先を読んだ結果である。
 * - `loading`: 読んでいる。
 * - `saved` / `unsaved`: 持ち主が接続先を保存したか（`OllamaSettings.saved`）。
 * - `failed`: 読めなかった。保存したかは分からない。
 */
export type OllamaConnection =
  | { status: "loading" }
  | { status: "saved" }
  | { status: "unsaved" }
  | { status: "failed"; reason: UiText };

/**
 * useOllamaConnection は、接続先に頼るページ（「Auto-tagging」と「Library」）が開いたときに
 * `GET /api/settings/ollama` を 1 回読み、保存したかを返す。`retry` は読み直す。
 */
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
