import { type Dispatch, type RefObject, useCallback, useRef } from "react";

import { getVideo, isAborted, type LibraryItem, RequestFailed } from "./client";
import { itemVideos } from "./libraryItems";
import { isProcessing } from "./useVideoDetail";
import { shownVideoIds, type VideosDataAction } from "./videosData";
import { favoriteMark, withFavoriteSince } from "./favorites";
import { visibilityMark, withVisibilitySince } from "./visibility";

/** UncertainReason は、一部にしか反映されず確かでなくなった項目の値である。 */
export type UncertainReason = "visibility" | "favorite";

/** useItemRefresh は useVideos の動画の項目を1件ずつ取り直す。 */
export function useItemRefresh(
  pageLoading: RefObject<boolean>,
  itemsRef: RefObject<LibraryItem[]>,
  dispatch: Dispatch<VideosDataAction>,
  // resync は一覧を先頭から読み直す。表示中の項目が畳まれて、代わりに何が出るかを
  // 画面では決められないときに使う。
  resync: () => void,
) {
  // 取り込みの準備が進んだ動画を、一覧を読み直さずに1件ずつ取り直す。読み直すと
  // スクロール位置や読み込んだページが失われる。取り直しは1件ずつ順に行い、
  // 知らせが重なっても同じ動画を重ねて取りに行かない。
  const refreshQueue = useRef(new Set<number>());
  const refreshing = useRef<AbortController | null>(null);
  // idleWaiters は、取り直しとページの取得がすべて終わるのを待つ者である
  // （一部にしか反映されなかった切り替えの取り直しの決着。下の購読を参照）。
  // 一覧を離れたときも解く。
  const idleWaiters = useRef<(() => void)[]>([]);
  // uncertain は、公開の切り替えが一部にしか反映されず、まだサーバーの状態を
  // 取り直せていない表示中の動画である。取り直しが一時的に失敗した動画は古い
  // `public` のまま残るので、ここから外れるまで決着させない（外れると古い一覧が
  // 控えられ、戻ったときに復元される。Devin の指摘、PR 338）。取り直しが成功する
  // か、消えたと分かる（404）か、切り替えの後に取ったページで置き換わるか、
  // 全件に反映された切り替えの結果が届くと外れる。
  //
  // 値は、その動画を確かでないとした直近の知らせの番号（staleNotices）である。
  // 知らせの前に始めた取り直し（更新の知らせ等）は切り替える前の `public` を
  // 読んでいることがあるので、取り直しは始めた時点の番号を覚えておき、それより
  // 新しい知らせで確かでないとされた動画は外さない（Devin の指摘、PR 338）。
  //
  // 確かでない理由（公開状態・お気に入り）は別々に持つ。全件に反映された公開の
  // 切り替えは `public` しか確かにしないので、お気に入りの印が確かでない動画を
  // 外してはならず、その逆も同じである（Codex の指摘、PR 668）。
  const uncertain = useRef(new Map<number, Map<UncertainReason, number>>());
  const staleNotices = useRef(0);
  // markUncertain は `id` を `reason` について、番号 `notice` の知らせで確かでないとする。
  const markUncertain = useCallback(
    (id: number, reason: UncertainReason, notice: number) => {
      const reasons = uncertain.current.get(id);
      if (reasons === undefined) uncertain.current.set(id, new Map([[reason, notice]]));
      else reasons.set(reason, notice);
    },
    [],
  );
  // settleUncertain は、`started`（取り直しを始めた時点の知らせの番号）以後の
  // 知らせで確かでないとされていない理由を、取り直しで確かになったとして外す。
  const settleUncertain = useCallback((id: number, started: number) => {
    const reasons = uncertain.current.get(id);
    if (reasons === undefined) return;
    for (const [reason, notice] of reasons) {
      if (notice <= started) reasons.delete(reason);
    }
    if (reasons.size === 0) uncertain.current.delete(id);
  }, []);
  // settleUncertainReason は、全件に反映された結果が届いた `reason` だけを外す。
  const settleUncertainReason = useCallback(
    (ids: Iterable<number>, reason: UncertainReason) => {
      for (const id of ids) {
        const reasons = uncertain.current.get(id);
        if (reasons === undefined) continue;
        reasons.delete(reason);
        if (reasons.size === 0) uncertain.current.delete(id);
      }
    },
    [],
  );
  const notifyIfIdle = useCallback(() => {
    if (
      pageLoading.current ||
      refreshing.current !== null ||
      refreshQueue.current.size > 0 ||
      uncertain.current.size > 0
    ) {
      return;
    }
    for (const resolve of idleWaiters.current.splice(0)) resolve();
  }, [pageLoading]);
  const drainRefreshQueue = useCallback(async () => {
    if (refreshing.current !== null) return;
    const controller = new AbortController();
    refreshing.current = controller;
    try {
      for (const id of refreshQueue.current) {
        refreshQueue.current.delete(id);
        const started = staleNotices.current;
        try {
          const mark = visibilityMark();
          const favoriteSince = favoriteMark();
          const refreshed = withFavoriteSince(
            withVisibilitySince(await getVideo(id, controller.signal), mark),
            favoriteSince,
          );
          // 条件を変えて読み直した後に届いた古い取り直しは、新しい一覧に重ねない。
          if (controller.signal.aborted) return;
          // 代表でなくなった動画（束ねた・代表を替えた）は、一覧では集まりの代表の
          // 1 件に畳まれる（specs/030-video-versions/research.md R-9）。代表が既に
          // 出ていれば、この 1 件を外せば一覧はサーバーと合う。出ていなければ、代表が
          // この一覧のフォルダ・絞り込みに入るかはサーバーにしか分からないので、
          // 一覧を先頭から読み直す（代表をそのまま差し込むと、範囲の外の動画が出る）。
          const representativeId = refreshed.versions?.representativeId;
          if (representativeId !== undefined && representativeId !== id) {
            settleUncertain(id, started);
            if (shownVideoIds(itemsRef.current).includes(representativeId)) {
              dispatch({ type: "remove", videoId: id });
              continue;
            }
            resync();
            return;
          }
          settleUncertain(id, started);
          dispatch({ type: "refresh", videoId: id, video: refreshed });
        } catch (failure) {
          if (isAborted(failure)) return;
          // 動画が索引から消えていたら、一覧からも外す。一時的な失敗は、その
          // 1件だけ諦める（次の知らせか取り込みの完了時の読み直しで直る）。
          // 公開状態が確かでない動画は、失敗しても uncertain に残す。
          if (failure instanceof RequestFailed && failure.status === 404) {
            settleUncertain(id, started);
            dispatch({ type: "remove", videoId: id });
          }
        }
      }
    } finally {
      if (refreshing.current === controller) refreshing.current = null;
      notifyIfIdle();
    }
  }, [dispatch, itemsRef, notifyIfIdle, resync, settleUncertain]);
  const refreshItems = useCallback(
    (ids: Iterable<number>) => {
      for (const id of ids) refreshQueue.current.add(id);
      void drainRefreshQueue();
    },
    [drainRefreshQueue],
  );
  const refreshProcessingItems = useCallback(() => {
    refreshItems(
      itemVideos(itemsRef.current)
        .filter((video) => isProcessing(video))
        .map((video) => video.id),
    );
  }, [itemsRef, refreshItems]);

  return {
    refreshQueue,
    refreshing,
    idleWaiters,
    uncertain,
    staleNotices,
    markUncertain,
    settleUncertainReason,
    notifyIfIdle,
    refreshItems,
    refreshProcessingItems,
  };
}
