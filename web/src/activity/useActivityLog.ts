import { useCallback, useEffect, useRef, useState } from "react";

import {
  type ActivityCounts,
  type ActivityEntry,
  type ActivityFeatureCounts,
  type ActivityPage,
  listActivity,
} from "../api/activity";
import { isAborted } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import {
  type ActivityCriteria,
  activityConditions,
  activityCriteriaKey,
} from "./activityCriteria";

// アクティビティの一覧の読み込み（specs/055-activity/ui-design.md「Cards」「Live updates」
// 「States」）。

export type MoreState = { kind: "idle" } | { kind: "loading" } | { kind: "failed" };

export type ActivityLogState =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: ActivityEntry[];
      /** 続きのページの鍵。無ければもう古い記録は無い。 */
      nextCursor?: string;
      /** status 以外の絞り込みに合う、状態ごとの数（Status のメニュー）。 */
      counts: ActivityCounts;
      /** feature 以外の絞り込みに合う、機能ごとの数（Feature のメニュー）。 */
      features: ActivityFeatureCounts;
      more: MoreState;
    };

export interface ActivityLog {
  state: ActivityLogState;
  /** 次のページを読む。読んでいる間と続きが無いときは何もしない。 */
  loadMore: () => void;
  /** 最初のページから読み直す。読んでいる間は読み込み中の表示にする。 */
  retry: () => void;
}

type Ready = Extract<ActivityLogState, { kind: "ready" }>;

/** readyOf は最初のページの件数と、行と続きの鍵から一覧を作る。 */
function readyOf(
  page: ActivityPage,
  items: ActivityEntry[],
  nextCursor: string | undefined,
): Ready {
  return {
    kind: "ready",
    items,
    ...(nextCursor === undefined ? {} : { nextCursor }),
    counts: page.counts,
    features: page.features,
    more: { kind: "idle" },
  };
}

/**
 * useActivityLog は絞り込みに合う記録を新しい順にページごとに読む。絞り込みが変わると最初の
 * ページから読み直す。`activity` の知らせとつなぎ直しでは、件数と読み込んだのと同じ数の
 * ページを読み直し、届いたら行を差し替える（並びは id の降順なので、読んでいたカードは
 * 動かない）。読み直しが重なったら今の読み直しの後に 1 度だけ読む。読み直しの失敗では今の行を
 * 残す。
 */
export function useActivityLog(criteria: ActivityCriteria): ActivityLog {
  const [state, setState] = useState<ActivityLogState>({ kind: "loading" });
  const stateRef = useRef(state);
  const show = useCallback((next: ActivityLogState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const key = activityCriteriaKey(criteria);
  const criteriaRef = useRef(criteria);
  useEffect(() => {
    criteriaRef.current = criteria;
  });

  // 読み直しのたびに増やし、古い応答を捨てる。
  const generation = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // 読み込んだページの数。読み直しはこの数だけ読む。
  const pages = useRef(0);
  const refreshing = useRef(false);
  const again = useRef(false);

  const begin = useCallback(() => {
    generation.current += 1;
    controller.current?.abort();
    controller.current = new AbortController();
    return { gen: generation.current, signal: controller.current.signal };
  }, []);

  const readPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      listActivity({
        ...activityConditions(criteriaRef.current, new Date()),
        ...(cursor === undefined ? {} : { cursor }),
        signal,
      }),
    [],
  );

  const readFirst = useCallback(() => {
    const { gen, signal } = begin();
    refreshing.current = false;
    again.current = false;
    pages.current = 0;
    show({ kind: "loading" });
    readPage(undefined, signal).then(
      (page) => {
        if (gen !== generation.current) return;
        pages.current = 1;
        show(readyOf(page, page.items, page.nextCursor));
      },
      (error: unknown) => {
        if (isAborted(error) || gen !== generation.current) return;
        show({ kind: "failed" });
      },
    );
  }, [begin, readPage, show]);

  const refresh = useCallback(() => {
    if (stateRef.current.kind !== "ready") return;
    if (refreshing.current) {
      again.current = true;
      return;
    }
    const { gen, signal } = begin();
    refreshing.current = true;
    again.current = false;
    const current = stateRef.current;
    if (current.more.kind === "loading") show({ ...current, more: { kind: "idle" } });
    const wanted = Math.max(1, pages.current);
    const finish = () => {
      refreshing.current = false;
      if (again.current) refresh();
    };
    const read = async () => {
      const first = await readPage(undefined, signal);
      const items = [...first.items];
      let last = first;
      let count = 1;
      while (count < wanted && last.nextCursor !== undefined) {
        last = await readPage(last.nextCursor, signal);
        items.push(...last.items);
        count += 1;
      }
      return { first, last, items, count };
    };
    read().then(
      ({ first, last, items, count }) => {
        if (gen !== generation.current) return;
        pages.current = count;
        show(readyOf(first, items, last.nextCursor));
        finish();
      },
      (error: unknown) => {
        if (isAborted(error) || gen !== generation.current) return;
        finish();
      },
    );
  }, [begin, readPage, show]);

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (
      current.kind !== "ready" ||
      current.nextCursor === undefined ||
      current.more.kind === "loading" ||
      refreshing.current
    )
      return;
    const gen = generation.current;
    const signal = controller.current?.signal ?? new AbortController().signal;
    show({ ...current, more: { kind: "loading" } });
    readPage(current.nextCursor, signal).then(
      (page) => {
        const shown = stateRef.current;
        if (gen !== generation.current || shown.kind !== "ready") return;
        pages.current += 1;
        const known = new Set(shown.items.map((item) => item.id));
        const items = [
          ...shown.items,
          ...page.items.filter((item) => !known.has(item.id)),
        ];
        show({
          ...readyOf(page, items, page.nextCursor),
          counts: shown.counts,
          features: shown.features,
        });
      },
      (error: unknown) => {
        const shown = stateRef.current;
        if (isAborted(error) || gen !== generation.current || shown.kind !== "ready")
          return;
        show({ ...shown, more: { kind: "failed" } });
      },
    );
  }, [readPage, show]);

  // 絞り込みが変わるたびに最初のページから読む。
  useEffect(() => {
    readFirst();
  }, [key, readFirst]);

  useEffect(() => {
    const unsubscribe = subscribeServerEvents({ activity: refresh, open: refresh });
    return () => {
      unsubscribe();
      generation.current += 1;
      controller.current?.abort();
    };
  }, [refresh]);

  return { state, loadMore, retry: readFirst };
}
