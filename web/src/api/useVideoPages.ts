import {
  type Dispatch,
  type RefObject,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
} from "react";

import { errorText, t, type UiText } from "../i18n";
import {
  type FolderRef,
  isAborted,
  type LibraryItem,
  listFolderVideos,
  listLibrary,
  listVideos,
  RequestFailed,
  type TagRef,
} from "./client";
import { groupsWithMembers, videoItem } from "./libraryItems";
import type { useGroupRefresh } from "./useGroupRefresh";
import type { useItemRefresh } from "./useItemRefresh";
import type { VideosCriteria } from "./videosCriteria";
import { shownVideoIds, type VideosDataAction, type VideosSeed } from "./videosData";
import { visibilityMark, withVisibilitySince } from "./visibility";

/** VideoPagesParams は useVideoPages が useVideos から受け取るものである。 */
interface VideoPagesParams {
  key: string;
  seed: VideosSeed | undefined;
  generation: number;
  inconsistent: boolean;
  cursor: string | undefined;
  hasMore: boolean;
  loading: boolean;
  loadingMore: boolean;
  folderKey: string;
  criteriaRef: RefObject<VideosCriteria>;
  folderRef: RefObject<FolderRef | undefined>;
  libraryRef: RefObject<boolean>;
  itemsRef: RefObject<LibraryItem[]>;
  dispatch: Dispatch<VideosDataAction>;
  setLoading: Dispatch<SetStateAction<boolean>>;
  setLoadingMore: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<UiText | null>>;
  setNotFound: Dispatch<SetStateAction<boolean>>;
  setGeneration: Dispatch<SetStateAction<number>>;
  resyncAttempted: RefObject<boolean>;
  pageLoading: RefObject<boolean>;
  changedWhileLoading: RefObject<Set<number>>;
  progressChangedWhileLoading: RefObject<Set<number>>;
  tagsChangedWhileLoading: RefObject<
    Map<string, { videoId: number; tag: TagRef; action: "add" | "remove" }>
  >;
  groups: ReturnType<typeof useGroupRefresh>;
  itemRefresh: ReturnType<typeof useItemRefresh>;
}

/**
 * useVideoPages は useVideos の一覧のページを取りに行く（条件が変わったときの
 * 先頭からの読み直しと、無限スクロールの続きの取得）。
 */
export function useVideoPages({
  key,
  seed,
  generation,
  inconsistent,
  cursor,
  hasMore,
  loading,
  loadingMore,
  folderKey,
  criteriaRef,
  folderRef,
  libraryRef,
  itemsRef,
  dispatch,
  setLoading,
  setLoadingMore,
  setError,
  setNotFound,
  setGeneration,
  resyncAttempted,
  pageLoading,
  changedWhileLoading,
  progressChangedWhileLoading,
  tagsChangedWhileLoading,
  groups: { groupQueue, groupRefreshing, staleGroups, unsettledGroups, refreshGroups },
  itemRefresh: { refreshQueue, refreshing, uncertain, notifyIfIdle, refreshItems },
}: VideoPagesParams) {
  // 読み込み中の要求を覚えておく。条件を変えた直後に古い応答が届いても、
  // 新しい一覧を上書きしないようにする。
  const inFlight = useRef<AbortController | null>(null);

  const fetchPage = useCallback(
    async (from: string | undefined, replace: boolean) => {
      inFlight.current?.abort();
      const controller = new AbortController();
      inFlight.current = controller;
      pageLoading.current = true;
      changedWhileLoading.current.clear();
      progressChangedWhileLoading.current.clear();
      if (replace) {
        // 前の一覧のために始めた取り直しは捨てる。新しいページの内容の方が新しい。
        refreshing.current?.abort();
        refreshing.current = null;
        refreshQueue.current.clear();
        groupRefreshing.current?.abort();
        groupRefreshing.current = null;
        groupQueue.current.clear();
        staleGroups.current = [];
        unsettledGroups.current.clear();
        // 条件やフォルダを変えた一から読み直し（または不整合からの再同期）
        // でだけ、それまでに記録した付け外しを捨てる。古い条件のときの変更は
        // 新しい一覧に持ち越さない。続きの取得（loadMore、replace===false）
        // ではここを通らないので、まだどのページにも現れていない動画への
        // 変更は消さずに残す（Devin の指摘3。次に読み込まれたページで重ねる）。
        tagsChangedWhileLoading.current.clear();
      }

      if (replace) {
        setLoading(true);
        setError(null);
        setNotFound(false);
      } else {
        setLoadingMore(true);
      }

      const mark = visibilityMark();
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
        const fetched =
          target !== undefined
            ? await listFolderVideos({ folder: target, scope: current.scope, ...params })
            : libraryRef.current
              ? await listLibrary({ ...params, tag: current.tag })
              : await listVideos({ ...params, tag: current.tag });
        const page = {
          ...fetched,
          items: fetched.items.map((item): LibraryItem => {
            if ("kind" in item) {
              return item.kind === "video"
                ? videoItem(withVisibilitySince(item.video, mark))
                : item;
            }
            return videoItem(withVisibilitySince(item, mark));
          }),
        };
        // 打ち切った要求の応答は捨てる。fetch は打ち切りで reject するが、
        // 応答の本文を読み終えた後に打ち切られた場合はここに来る。
        if (controller.signal.aborted || inFlight.current !== controller) return;
        if (replace && page.items.length > page.total) {
          if (!resyncAttempted.current) {
            resyncAttempted.current = true;
            setGeneration((value) => value + 1);
          } else {
            dispatch({ type: "clear" });
            dispatch({ type: "stop" });
            setError(t.list.inconsistentPage);
          }
          return;
        }
        if (replace) resyncAttempted.current = false;
        const shownBefore = new Set(shownVideoIds(itemsRef.current));
        dispatch({ type: "page", page, replace });
        const changed = shownVideoIds(page.items).filter((id) =>
          changedWhileLoading.current.has(id),
        );
        // ページの取得中にメンバーが変わったグループは、ページを反映したあとで取り直す。
        // 再生位置の保存も同じく、ページの内容より新しいことがある。
        refreshGroups(
          groupsWithMembers(page.items, [
            ...changedWhileLoading.current,
            ...progressChangedWhileLoading.current,
          ]),
        );
        changedWhileLoading.current.clear();
        progressChangedWhileLoading.current.clear();
        // 公開状態が確かでない動画のうち、このページで取り直す（changed）ものと、
        // 続きの取得で残る表示中のものだけを uncertain に残す。一から読み直した
        // ページは切り替えの後に取ったものなので、それ以外はもう確かである。
        // ページの取得中に届いた知らせの対象は changedWhileLoading に入っている
        // ので、切り替えの前に読まれたかもしれないページで確かになることはない。
        const stillShown = new Set(changed);
        if (!replace) for (const id of shownBefore) stillShown.add(id);
        for (const id of uncertain.current.keys()) {
          if (!stillShown.has(id)) uncertain.current.delete(id);
        }
        if (changed.length > 0) refreshItems(changed);
        // このページの取得中に届いたタグの付け外しのうち、このページで
        // ちょうど読み込んだ動画のものは、取り直さずここで直接重ねる
        // （サーバーがすでに教えてくれている内容なので、getVideo で1件ずつ
        // 取り直す必要が無い。Devin の指摘3）。
        // グループのメンバーへの付け外しは、そのグループを取り直す。
        if (tagsChangedWhileLoading.current.size > 0) {
          const pageIds = new Set(shownVideoIds(page.items));
          const memberIds = new Set(
            page.items.flatMap((item) =>
              item.kind === "group" ? item.group.videoIds : [],
            ),
          );
          const changedMembers: number[] = [];
          for (const [tagKey, change] of tagsChangedWhileLoading.current) {
            if (memberIds.has(change.videoId)) {
              changedMembers.push(change.videoId);
              tagsChangedWhileLoading.current.delete(tagKey);
              continue;
            }
            if (!pageIds.has(change.videoId)) continue;
            dispatch({
              type: "tags",
              videoIds: [change.videoId],
              tag: change.tag,
              action: change.action,
            });
            tagsChangedWhileLoading.current.delete(tagKey);
          }
          refreshGroups(groupsWithMembers(page.items, changedMembers));
        }
        setError(null);
        setNotFound(false);
      } catch (failure) {
        // 打ち切った要求や、条件を変えた後に届いた古い要求の失敗は、新しい一覧に
        // 404 や失敗を持ち込まないよう捨てる。
        if (
          isAborted(failure) ||
          controller.signal.aborted ||
          inFlight.current !== controller
        ) {
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
          dispatch({ type: "stop" });
          return;
        }
        setError(errorText(failure));
        // 前の要求の 404 を残すと、取得の失敗が「見つかりません」に隠れて再試行できない。
        setNotFound(false);
        // 続きが読めない状態で観測点を残すと、同じ要求を繰り返してしまう。
        dispatch({ type: "stop" });
      } finally {
        if (!controller.signal.aborted) {
          pageLoading.current = false;
          setLoading(false);
          setLoadingMore(false);
          notifyIfIdle();
        }
      }
    },
    // folderKey と key は folderRef・criteriaRef の中身が変わったことを表す。
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [folderKey, key, notifyIfIdle, refreshGroups, refreshItems],
  );

  // seeded は「いま持っている中身が復元で埋まったものか」を覚える。
  //
  // 効果を 1 回で消費する印にしないのは、React が開発時に効果を 2 回走らせる
  // ためである（1 回目で消費すると 2 回目が復元を捨てて読み直してしまう）。
  // 鍵（条件・フォルダ・読み直しの世代）ごと覚えておけば、何度走っても
  // 同じ判断になる。
  const seeded = useRef(seed === undefined ? null : { key, folderKey, generation: 0 });

  // 異なる時点のページを結合できなかったときは、古い整合した一覧を保ったまま
  // 世代を進め、通常の先頭ページ取得へ戻す。
  useEffect(() => {
    if (!inconsistent) return;
    if (!resyncAttempted.current) {
      resyncAttempted.current = true;
      setGeneration((value) => value + 1);
      return;
    }
    dispatch({ type: "clear" });
    dispatch({ type: "stop" });
    setError(t.list.inconsistentPage);
  }, [dispatch, inconsistent, resyncAttempted, setError, setGeneration]);

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

    dispatch({ type: "clear" });
    void fetchPage(undefined, true);

    return () => inFlight.current?.abort();
  }, [dispatch, fetchPage, folderKey, generation, key]);

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

  const reload = useCallback(() => {
    resyncAttempted.current = false;
    setGeneration((value) => value + 1);
  }, [resyncAttempted, setGeneration]);

  return { loadMore, retryLoadMore, reload };
}
