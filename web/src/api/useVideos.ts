import { useCallback, useEffect, useRef, useState } from "react";

import {
  errorMessage,
  isAborted,
  listVideos,
  type Video,
  type VideoSort,
} from "./client";

/** VideosState は一覧の状態である。 */
export interface VideosState {
  items: Video[];
  total: number;
  /** hasMore は次のページがあるかどうか。 */
  hasMore: boolean;
  /** loading は最初の1ページを待っている間だけ true になる。 */
  loading: boolean;
  /** loadingMore は続きを読んでいる間 true になる。 */
  loadingMore: boolean;
  error: string | null;
  /** loadMore は次のページを読む。無限スクロールの観測点から呼ぶ。 */
  loadMore: () => void;
  /** reload は先頭から読み直す。取り込みのあとに使う。 */
  reload: () => void;
}

/**
 * useVideos は一覧を1ページずつ読む。query を与えると題名で絞り込む。
 *
 * 最初の表示は1ページ（60 件）だけを待つ。1万件でも最初の画面が 2 秒以内に
 * 出る（SC-003）のは、全件を読まないことによる（R-114）。
 */
export function useVideos(sort: VideoSort, query: string): VideosState {
  const [items, setItems] = useState<Video[]>([]);
  const [total, setTotal] = useState(0);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [generation, setGeneration] = useState(0);

  // 読み込み中の要求を覚えておく。並び順を変えた直後に古い応答が届いても、
  // 新しい一覧を上書きしないようにする。
  const inFlight = useRef<AbortController | null>(null);

  const fetchPage = useCallback(
    async (from: string | undefined, replace: boolean) => {
      inFlight.current?.abort();
      const controller = new AbortController();
      inFlight.current = controller;

      if (replace) {
        setLoading(true);
      } else {
        setLoadingMore(true);
      }

      try {
        const page = await listVideos({
          sort,
          query,
          cursor: from,
          signal: controller.signal,
        });
        setItems((current) => (replace ? page.items : [...current, ...page.items]));
        setTotal(page.total);
        setCursor(page.nextCursor);
        setHasMore(page.nextCursor !== undefined);
        setError(null);
      } catch (failure) {
        if (isAborted(failure)) {
          return;
        }
        setError(errorMessage(failure));
        // 続きが読めない状態で観測点を残すと、同じ要求を繰り返してしまう。
        setHasMore(false);
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          setLoadingMore(false);
        }
      }
    },
    [query, sort],
  );

  // 並び順か検索語が変わったら先頭から読み直す。カーソルはその2つに
  // 紐づくので、引き継ぐと境界の意味が変わってしまう。
  //
  // 前の要求は fetchPage が AbortController で打ち切る。入力が連続しても、
  // 古い応答が新しい一覧を上書きすることはない。
  useEffect(() => {
    setItems([]);
    setCursor(undefined);
    setHasMore(true);
    void fetchPage(undefined, true);

    return () => inFlight.current?.abort();
  }, [fetchPage, generation]);

  const loadMore = useCallback(() => {
    if (loading || loadingMore || !hasMore || cursor === undefined) {
      return;
    }
    void fetchPage(cursor, false);
  }, [cursor, fetchPage, hasMore, loading, loadingMore]);

  const reload = useCallback(() => setGeneration((value) => value + 1), []);

  return { items, total, hasMore, loading, loadingMore, error, loadMore, reload };
}
