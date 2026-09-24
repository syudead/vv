import { useCallback, useEffect, useRef, useState } from "react";

import {
  errorMessage,
  type FolderRef,
  getVideo,
  isAborted,
  listFolderVideos,
  listVideos,
  type Video,
  type VideoSort,
} from "./client";
import { subscribeProgress } from "./progressEvents";
import { subscribeServerEvents } from "./serverEvents";
import { isProcessing } from "./useVideoDetail";

/**
 * mergeRefreshed は取り直した1件を、一覧に出ている項目へ重ねる。
 *
 * 題名と大きさは一覧側の値を残す。フォルダ画面の項目は、そのフォルダにある
 * 所在の題名と大きさを持ち、1件の取得（代表の所在）とは違うことがあるためである。
 */
function mergeRefreshed(current: Video, refreshed: Video): Video {
  return { ...current, ...refreshed, title: current.title, sizeBytes: current.sizeBytes };
}

/**
 * VideosSeed は復元された一覧の初期状態である。
 *
 * 与えると 1 ページ目を**取りに行かない**。再生画面から戻るたびに読み直すと、
 * 300 件まで読んだ状態を戻すのに 5 ページ分の往復が要る。
 */
export interface VideosSeed {
  items: Video[];
  total: number;
  cursor?: string;
  hasMore: boolean;
}

/** VideosState は一覧の状態である。 */
export interface VideosState {
  items: Video[];
  total: number;
  /** cursor は次のページの続き位置。控えを取るときに使う。 */
  cursor: string | undefined;
  /** hasMore は次のページがあるかどうか。 */
  hasMore: boolean;
  /** loading は最初の1ページを待っている間だけ true になる。 */
  loading: boolean;
  /** loadingMore は続きを読んでいる間 true になる。 */
  loadingMore: boolean;
  error: string | null;
  /** loadMore は次のページを読む。無限スクロールの観測点から呼ぶ。 */
  loadMore: () => void;
  /** retryLoadMore は失敗した続きのページを同じカーソルから再要求する。 */
  retryLoadMore: () => void;
  /** reload は先頭から読み直す。取り込みのあとに使う。 */
  reload: () => void;
}

/**
 * useVideos は一覧を1ページずつ読む。query を与えると題名で絞り込む。
 *
 * 最初の表示は1ページ（60 件）だけを待つ。1万件でも最初の画面が 2 秒以内に
 * 出るのは、全件を読まないことによる。
 *
 * seed を与えると、その並び順・検索語のあいだは1ページ目を取りに行かない
 * （再生画面から戻ったときの復元。一覧の状態はこの受け渡し口からだけ入る）。
 *
 * folder を与えると、ライブラリ全体ではなくそのフォルダ直下の動画を読む
 * （フォルダ画面）。その場合 query は使わない。ページング・中断・復元の
 * 仕組みはライブラリと同じものを使う。
 */
export function useVideos(
  sort: VideoSort,
  query: string,
  seed?: VideosSeed,
  folder?: FolderRef,
): VideosState {
  // フォルダは値で比べる。呼び出し側が描画ごとに新しいオブジェクトを渡しても
  // 読み直さないよう、鍵の文字列だけを依存に使う。
  const folderKey =
    folder === undefined ? "" : `${String(folder.rootId)}\0${folder.path}`;
  const folderRef = useRef(folder);
  folderRef.current = folder;
  const [items, setItems] = useState<Video[]>(seed?.items ?? []);
  const [total, setTotal] = useState(seed?.total ?? 0);
  const [cursor, setCursor] = useState<string | undefined>(seed?.cursor);
  const [hasMore, setHasMore] = useState(seed?.hasMore ?? true);
  const [loading, setLoading] = useState(seed === undefined);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [generation, setGeneration] = useState(0);

  // 再生画面で保存された再生位置を、表示中の項目へ反映する。復元した一覧は
  // 再生前の中身なので、戻ったあとに届く離脱時の保存もここで受ける。
  useEffect(
    () =>
      subscribeProgress((videoId, progress) => {
        setItems((current) =>
          current.some((video) => video.id === videoId)
            ? current.map((video) =>
                video.id === videoId ? { ...video, progress } : video,
              )
            : current,
        );
      }),
    [],
  );

  // 取り込みの準備が進んだ動画を、一覧を読み直さずに1件ずつ取り直す。読み直すと
  // スクロール位置や読み込んだページが失われる。取り直しは1件ずつ順に行い、
  // 知らせが重なっても同じ動画を重ねて取りに行かない。
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const refreshQueue = useRef(new Set<number>());
  const refreshing = useRef<AbortController | null>(null);
  const drainRefreshQueue = useCallback(async () => {
    if (refreshing.current !== null) return;
    const controller = new AbortController();
    refreshing.current = controller;
    try {
      for (const id of refreshQueue.current) {
        refreshQueue.current.delete(id);
        try {
          const refreshed = await getVideo(id, controller.signal);
          setItems((current) =>
            current.map((video) =>
              video.id === id ? mergeRefreshed(video, refreshed) : video,
            ),
          );
        } catch (failure) {
          if (isAborted(failure)) return;
          // 消えた動画や一時的な失敗は、その1件だけ諦める。一覧からの削除は
          // 取り込みの完了時の読み直しが受け持つ。
        }
      }
    } finally {
      if (refreshing.current === controller) refreshing.current = null;
    }
  }, []);
  const refreshItems = useCallback(
    (ids: Iterable<number>) => {
      for (const id of ids) refreshQueue.current.add(id);
      void drainRefreshQueue();
    },
    [drainRefreshQueue],
  );
  const refreshProcessingItems = useCallback(() => {
    refreshItems(
      itemsRef.current.filter((video) => isProcessing(video)).map((video) => video.id),
    );
  }, [refreshItems]);

  // ページの取得中に届いた知らせは、取得した内容より新しいことがある。まだ一覧に
  // 無い動画の知らせを覚えておき、ページを反映したあとで取り直す。
  const pageLoading = useRef(false);
  const changedWhileLoading = useRef(new Set<number>());

  useEffect(() => {
    const unsubscribe = subscribeServerEvents({
      video: (id) => {
        if (itemsRef.current.some((video) => video.id === id)) {
          refreshItems([id]);
        } else if (pageLoading.current) {
          changedWhileLoading.current.add(id);
        }
      },
      // つなぎ直したときは、切れていた間に準備が進んだかもしれない項目を取り直す。
      open: refreshProcessingItems,
    });
    // 復元した一覧は、別の画面にいた間に準備が進んでいることがある。
    refreshProcessingItems();
    return () => {
      unsubscribe();
      refreshing.current?.abort();
      refreshing.current = null;
      refreshQueue.current.clear();
    };
  }, [refreshItems, refreshProcessingItems]);

  // 読み込み中の要求を覚えておく。並び順を変えた直後に古い応答が届いても、
  // 新しい一覧を上書きしないようにする。
  const inFlight = useRef<AbortController | null>(null);

  const fetchPage = useCallback(
    async (from: string | undefined, replace: boolean) => {
      inFlight.current?.abort();
      const controller = new AbortController();
      inFlight.current = controller;
      pageLoading.current = true;
      changedWhileLoading.current.clear();

      if (replace) {
        setLoading(true);
      } else {
        setLoadingMore(true);
      }

      try {
        const target = folderRef.current;
        const page =
          target === undefined
            ? await listVideos({ sort, query, cursor: from, signal: controller.signal })
            : await listFolderVideos({
                folder: target,
                sort,
                cursor: from,
                signal: controller.signal,
              });
        setItems((current) => (replace ? page.items : [...current, ...page.items]));
        const changed = page.items
          .map((video) => video.id)
          .filter((id) => changedWhileLoading.current.has(id));
        changedWhileLoading.current.clear();
        if (changed.length > 0) refreshItems(changed);
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
          pageLoading.current = false;
          setLoading(false);
          setLoadingMore(false);
        }
      }
    },
    // folderKey は folderRef の中身が変わったことを表す。
    [folderKey, query, refreshItems, sort],
  );

  // seeded は「いま持っている中身が復元で埋まったものか」を覚える。
  //
  // 効果を 1 回で消費する印にしないのは、React が開発時に効果を 2 回走らせる
  // ためである（1 回目で消費すると 2 回目が復元を捨てて読み直してしまう）。
  // 鍵（並び順・検索語・読み直しの世代）ごと覚えておけば、何度走っても
  // 同じ判断になる。
  const seeded = useRef(
    seed === undefined ? null : { sort, query, folderKey, generation: 0 },
  );

  // 並び順・検索語・フォルダが変わったら先頭から読み直す。カーソルはそれらに
  // 紐づくので、引き継ぐと境界の意味が変わってしまう。
  //
  // 前の要求は fetchPage が AbortController で打ち切る。入力が連続しても、
  // 古い応答が新しい一覧を上書きすることはない。
  useEffect(() => {
    const restored = seeded.current;
    if (
      restored !== null &&
      restored.sort === sort &&
      restored.query === query &&
      restored.folderKey === folderKey &&
      restored.generation === generation
    ) {
      // 取りに行かなくても打ち切りは要る。復元した一覧で続きを読んでいる
      // 途中に画面を離れると、この経路が後片付けを残さないかぎり要求が
      // 最後まで走ってしまう。
      return () => inFlight.current?.abort();
    }
    seeded.current = null;

    setItems([]);
    setCursor(undefined);
    setHasMore(true);
    void fetchPage(undefined, true);

    return () => inFlight.current?.abort();
  }, [fetchPage, folderKey, generation, query, sort]);

  const loadMore = useCallback(() => {
    if (loading || loadingMore || !hasMore || cursor === undefined) {
      return;
    }
    void fetchPage(cursor, false);
  }, [cursor, fetchPage, hasMore, loading, loadingMore]);

  const retryLoadMore = useCallback(() => {
    if (loading || loadingMore || cursor === undefined) return;
    void fetchPage(cursor, false);
  }, [cursor, fetchPage, loading, loadingMore]);

  const reload = useCallback(() => setGeneration((value) => value + 1), []);

  return {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    loadMore,
    retryLoadMore,
    reload,
  };
}
