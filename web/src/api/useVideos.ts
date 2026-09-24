import { useCallback, useEffect, useRef, useState } from "react";

import {
  errorMessage,
  type FolderRef,
  type FolderScope,
  isAborted,
  listFolderVideos,
  listVideos,
  RequestFailed,
  type Video,
  type VideoSort,
  type WatchFilter,
} from "./client";
import { subscribeProgress } from "./progressEvents";

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

/**
 * VideosCriteria は一覧を取りに行く条件である
 * （specs/013-library-search/contracts/list-url.md の q・watch・playable・sort・seed）。
 * 省略した項目はサーバーの既定になる。
 */
export interface VideosCriteria {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  sort: VideoSort;
  /** sort=random のときだけ送る。 */
  seed?: number;
  /**
   * フォルダ画面での検索範囲（direct = 直下だけ、subtree = 配下すべて）。
   * folder を渡さない（ライブラリ）ときは無視する。省略時は direct と同じ。
   */
  scope?: FolderScope;
}

/** criteriaKey は条件を値で比べるための文字列にする。 */
function criteriaKey(criteria: VideosCriteria): string {
  return JSON.stringify([
    criteria.query ?? "",
    criteria.watch ?? "all",
    criteria.playable === true,
    criteria.sort,
    criteria.sort === "random" ? (criteria.seed ?? null) : null,
    criteria.scope ?? "direct",
  ]);
}

/** appendUnique は続きのページから、既に出ている id を捨てて足す（list-api.md §5）。 */
function appendUnique(current: Video[], next: Video[]): Video[] {
  const seen = new Set(current.map((video) => video.id));
  const added: Video[] = [];
  for (const video of next) {
    if (seen.has(video.id)) continue;
    seen.add(video.id);
    added.push(video);
  }
  return added.length === 0 ? current : [...current, ...added];
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
  /**
   * notFound は folder を渡したときに、そのフォルダの動画の要求が 404 で
   * 返ったことを表す（検索中にフォルダが無くなった場合、
   * list-api.md §5「listFolderVideos でフォルダが無いとき」）。
   */
  notFound: boolean;
  /** loadMore は次のページを読む。無限スクロールの観測点から呼ぶ。 */
  loadMore: () => void;
  /** retryLoadMore は失敗した続きのページを同じカーソルから再要求する。 */
  retryLoadMore: () => void;
  /** reload は先頭から読み直す。取り込みのあとに使う。 */
  reload: () => void;
}

/**
 * useVideos は一覧を1ページずつ読む。条件（検索語・視聴状態・再生可否・並び順・
 * seed）はサーバーが適用し、画面は絞り込みの後処理をしない。
 *
 * 最初の表示は1ページ（60 件）だけを待つ。1万件でも最初の画面が 2 秒以内に
 * 出るのは、全件を読まないことによる。
 *
 * restored を与えると、その条件のあいだは1ページ目を取りに行かない
 * （再生画面から戻ったときの復元。一覧の状態はこの受け渡し口からだけ入る）。
 *
 * folder を与えると、ライブラリ全体ではなくそのフォルダ直下の動画を読む
 * （フォルダ画面）。ページング・中断・復元の仕組みはライブラリと同じものを使う。
 */
export function useVideos(
  criteria: VideosCriteria,
  restored?: VideosSeed,
  folder?: FolderRef,
): VideosState {
  // 条件は値で比べる。呼び出し側が描画ごとに新しいオブジェクトを渡しても
  // 読み直さないよう、鍵の文字列だけを依存に使う。
  const key = criteriaKey(criteria);
  const criteriaRef = useRef(criteria);
  criteriaRef.current = criteria;
  const seed = restored;
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
  const [notFound, setNotFound] = useState(false);
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

  // 読み込み中の要求を覚えておく。条件を変えた直後に古い応答が届いても、
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
        const target = folderRef.current;
        const current = criteriaRef.current;
        const params = {
          query: current.query,
          watch: current.watch === "all" ? undefined : current.watch,
          playable: current.playable,
          sort: current.sort,
          seed: current.sort === "random" ? current.seed : undefined,
          cursor: from,
          signal: controller.signal,
        };
        const page =
          target === undefined
            ? await listVideos(params)
            : await listFolderVideos({
                folder: target,
                scope: current.scope,
                ...params,
              });
        // 打ち切った要求の応答は捨てる。fetch は打ち切りで reject するが、
        // 応答の本文を読み終えた後に打ち切られた場合はここに来る。
        if (controller.signal.aborted || inFlight.current !== controller) return;
        setItems((items) => (replace ? page.items : appendUnique(items, page.items)));
        setTotal(page.total);
        setCursor(page.nextCursor);
        setHasMore(page.nextCursor !== undefined);
        setError(null);
        setNotFound(false);
      } catch (failure) {
        if (isAborted(failure)) {
          return;
        }
        if (
          folderRef.current !== undefined &&
          failure instanceof RequestFailed &&
          failure.status === 404
        ) {
          // フォルダが無くなった（検索中に配下が削除された等）。一致なしではなく
          // 「このフォルダは見つかりません」を出す（list-api.md §5）。
          setNotFound(true);
          setError(null);
          setHasMore(false);
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
    // folderKey と key は folderRef・criteriaRef の中身が変わったことを表す。
    [folderKey, key],
  );

  // seeded は「いま持っている中身が復元で埋まったものか」を覚える。
  //
  // 効果を 1 回で消費する印にしないのは、React が開発時に効果を 2 回走らせる
  // ためである（1 回目で消費すると 2 回目が復元を捨てて読み直してしまう）。
  // 鍵（条件・フォルダ・読み直しの世代）ごと覚えておけば、何度走っても
  // 同じ判断になる。
  const seeded = useRef(seed === undefined ? null : { key, folderKey, generation: 0 });

  // 条件・フォルダが変わったら先頭から読み直す。カーソルはそれらに紐づくので、
  // 引き継ぐと境界の意味が変わってしまう。
  //
  // 前の要求は fetchPage が AbortController で打ち切る。入力が連続しても、
  // 古い応答が新しい一覧を上書きすることはない。
  useEffect(() => {
    const held = seeded.current;
    if (
      held !== null &&
      held.key === key &&
      held.folderKey === folderKey &&
      held.generation === generation
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
  }, [fetchPage, folderKey, generation, key]);

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
    notFound,
    loadMore,
    retryLoadMore,
    reload,
  };
}
