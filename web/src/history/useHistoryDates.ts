import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listWatchHistoryDates, type WatchHistoryFilter } from "../api/history";

// 日付へ移る一覧の元になる、件のある日の読み込み（specs/043-watch-history/ui-design.md
// 「Jump to date」、research.md R-11）。

export type HistoryDatesState =
  { kind: "loading" } | { kind: "failed" } | { kind: "ready"; days: readonly string[] };

export interface HistoryDates {
  state: HistoryDatesState;
  /** 読み直す。読んでいる間は読み込み中の表示にする。 */
  retry: () => void;
  /** 全件を消した後の、日の無い一覧にする。読んでいる要求は捨てる。 */
  clear: () => void;
}

/**
 * useHistoryDates は今の状態と検索語に合う件のある日を読む。開いたときと、状態か検索語が
 * 変わったときに読み、日（`date`）が変わったときには読まない（一覧がちらつかないように）。
 */
export function useHistoryDates(watch: WatchHistoryFilter, query: string): HistoryDates {
  const [state, setState] = useState<HistoryDatesState>({ kind: "loading" });
  const controller = useRef<AbortController | null>(null);

  const stop = useCallback(() => {
    controller.current?.abort();
    controller.current = null;
  }, []);

  const read = useCallback(() => {
    stop();
    const current = new AbortController();
    controller.current = current;
    setState({ kind: "loading" });
    listWatchHistoryDates({
      ...(watch === "all" ? {} : { watch }),
      ...(query === "" ? {} : { query }),
      signal: current.signal,
    }).then(
      (dates) => {
        if (controller.current !== current) return;
        controller.current = null;
        setState({ kind: "ready", days: dates.days });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.current !== current) return;
        controller.current = null;
        setState({ kind: "failed" });
      },
    );
  }, [query, stop, watch]);

  useEffect(() => {
    read();
    return stop;
  }, [read, stop]);

  const clear = useCallback(() => {
    stop();
    setState({ kind: "ready", days: [] });
  }, [stop]);

  return { state, retry: read, clear };
}
