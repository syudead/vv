import { useCallback, useEffect, useRef, useState } from "react";

import { type ActivityCounts, listActivity } from "./activity";
import { isAborted } from "./client";
import { subscribeServerEvents } from "./serverEvents";

/**
 * useActivityTotals はログ全体の状態ごとの数を、絞り込みの無い要求の `counts` から読む
 * （specs/055-activity/contracts/screen-api.md「GET /api/activity」）。サイドバーの件数と
 * アクティビティの画面のフッターが使う。`activity` の知らせとつなぎ直しで読み直し、読み直しが
 * 重なったら今の取得の後に 1 度だけ取る。読めないうちと失敗した最初の取得は null。
 * `enabled` が false の間（ゲスト）は何も読まない。
 */
export function useActivityTotals(enabled = true): ActivityCounts | null {
  const [counts, setCounts] = useState<ActivityCounts | null>(null);
  const fetching = useRef<AbortController | null>(null);
  const again = useRef(false);

  const load = useCallback(() => {
    if (fetching.current !== null) {
      again.current = true;
      return;
    }
    const controller = new AbortController();
    fetching.current = controller;
    again.current = false;
    const settle = () => {
      fetching.current = null;
      if (again.current) load();
    };
    listActivity({ limit: 1, signal: controller.signal }).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCounts(page.counts);
        settle();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        settle();
      },
    );
  }, []);

  useEffect(() => {
    if (!enabled) return;
    const unsubscribe = subscribeServerEvents({ activity: load, open: load });
    load();
    return () => {
      unsubscribe();
      fetching.current?.abort();
      fetching.current = null;
    };
  }, [enabled, load]);

  return enabled ? counts : null;
}
