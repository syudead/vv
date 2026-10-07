import { useCallback, useEffect, useReducer, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import type { UiText } from "../i18n";
import type { FolderRef, TagRef } from "./client";
import { subscribeFavorites, subscribeFavoritesStale } from "./favorites";
import { folderRefKey, groupRef } from "./libraryItems";
import { subscribeProgress } from "./progressEvents";
import { subscribeServerEvents } from "./serverEvents";
import { useGroupRefresh } from "./useGroupRefresh";
import { type UncertainReason, useItemRefresh } from "./useItemRefresh";
import { useVideoPages } from "./useVideoPages";
import { subscribeVideoTags } from "./videoTagsEvents";
import { criteriaKey, type VideosCriteria, type VideosSource } from "./videosCriteria";
import {
  shownVideoIds,
  type VideosData,
  videosDataReducer,
  type VideosSeed,
  type VideosState,
} from "./videosData";
import { subscribeVideoVisibility, subscribeVideoVisibilityStale } from "./visibility";

export type { VideosCriteria, VideosSource } from "./videosCriteria";
export type { VideosSeed, VideosState } from "./videosData";

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
 * source は読む一覧である（VideosSource）。フォルダを与えると、ライブラリ全体ではなく
 * そのフォルダの動画を読む（フォルダ画面）。ページング・中断・復元の仕組みはどれも
 * 同じものを使う。
 */
export function useVideos(
  criteria: VideosCriteria,
  restored?: VideosSeed,
  source: VideosSource = "videos",
): VideosState {
  const folder = typeof source === "string" ? undefined : source;
  const library = source === "library";
  // 条件は値で比べる。呼び出し側が描画ごとに新しいオブジェクトを渡しても
  // 読み直さないよう、鍵の文字列だけを依存に使う。
  const key = criteriaKey(criteria);
  const criteriaRef = useRef(criteria);
  criteriaRef.current = criteria;
  const seed = restored;
  // フォルダは値で比べる。呼び出し側が描画ごとに新しいオブジェクトを渡しても
  // 読み直さないよう、鍵の文字列だけを依存に使う。
  const folderKey =
    folder === undefined ? (library ? "library" : "") : folderRefKey(folder);
  const folderRef = useRef(folder);
  folderRef.current = folder;
  const libraryRef = useRef(library);
  libraryRef.current = library;
  const [{ items, total, cursor, hasMore, inconsistent, missingTagIds }, dispatch] =
    useReducer(videosDataReducer, seed, (initial): VideosData => ({
      items: initial?.items ?? [],
      total: initial?.total ?? 0,
      cursor: initial?.cursor,
      hasMore: initial?.hasMore ?? true,
      inconsistent: false,
      missingTagIds: [],
    }));
  const [loading, setLoading] = useState(seed === undefined);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [generation, setGeneration] = useState(0);
  const resyncAttempted = useRef(false);

  // 条件や場所が変われば、前の一覧で使った再同期回数は引き継がない。
  useEffect(() => {
    resyncAttempted.current = false;
  }, [folderKey, key]);

  const itemsRef = useRef(items);
  itemsRef.current = items;

  const groups = useGroupRefresh(seed, itemsRef, dispatch);
  const {
    groupQueue,
    groupRefreshing,
    staleGroups,
    refreshGroups,
    refreshGroupsWith,
    unsettledGroupRefs,
  } = groups;

  // ページの取得中に届いた知らせは、取得した内容より新しいことがある
  // （下の「取り込みの準備」の取り直しと、その次のタグの付け外しの両方が使う）。
  const pageLoading = useRef(false);

  // ページの取得中に再生位置が保存された動画である。取得中のページに初めて現れる
  // グループは保存前の値を持つことがあるので、ページを反映したあとでメンバーに
  // 当たるグループを取り直す（fetchPage。Devin の指摘、PR 357）。
  const progressChangedWhileLoading = useRef(new Set<number>());

  // ページの取得中にお気に入りを付け外したグループのフォルダである（下の subscribeFavorites）。
  const favoriteFoldersChangedWhileLoading = useRef(new Map<string, FolderRef>());

  // 再生画面で保存された再生位置を、表示中の項目へ反映する。復元した一覧は
  // 再生前の中身なので、戻ったあとに届く離脱時の保存もここで受ける。
  useEffect(
    () =>
      subscribeProgress((videoId, progress) => {
        if (pageLoading.current) progressChangedWhileLoading.current.add(videoId);
        dispatch({ type: "progress", videoId, progress });
        refreshGroupsWith([videoId]);
      }),
    [refreshGroupsWith],
  );

  // ページの取得中に届いたタグの付け外しは、まだ読み込んでいない動画には
  // 反映しようが無く、そのまま捨てると後から届くページの古い内容で
  // 上書きされたことにもならず消えてしまう（Devin の指摘3）。動画・タグの
  // 組ごとに直近の変更を覚えておき、その後に届いたページにその動画が
  // あれば重ねる（下の changedWhileLoading と同じ仕組み）。対象の動画が
  // どのページで読み込まれるか（続きのどのページか）は分からないので、
  // loadMore（続きの取得）をまたいで持ち越す（下の fetchPage 参照）。
  // 無限に育たないよう、件数の上限を超えたら古い順に間引く。
  const tagsChangedWhileLoading = useRef(
    new Map<string, { videoId: number; tag: TagRef; action: "add" | "remove" }>(),
  );
  const maxTagsChangedWhileLoading = 500;

  // 付け外しの結果を、表示中の項目へ反映する。絞り込みに合わなくなった項目も、その場では一覧から外さない。
  useEffect(
    () =>
      subscribeVideoTags((videoIds, tag, action) => {
        if (pageLoading.current) {
          const map = tagsChangedWhileLoading.current;
          for (const videoId of videoIds) {
            map.set(`${String(videoId)}\0${String(tag.id)}`, { videoId, tag, action });
          }
          // Map は挿入順を保つので、先頭から（一番古い記録から）間引く。
          while (map.size > maxTagsChangedWhileLoading) {
            const oldest = map.keys().next();
            if (oldest.done) break;
            map.delete(oldest.value);
          }
        }
        dispatch({ type: "tags", videoIds, tag, action });
        refreshGroupsWith(videoIds);
      }),
    [refreshGroupsWith],
  );

  // 取り直しで表示中の動画が集まりに畳まれたと分かったときの読み直し（useItemRefresh）。
  // useVideoPages の reload と同じく世代を進め、先頭のページから取り直す。
  const resync = useCallback(() => {
    resyncAttempted.current = false;
    setGeneration((value) => value + 1);
  }, []);
  const itemRefresh = useItemRefresh(pageLoading, itemsRef, dispatch, resync);
  const {
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
  } = itemRefresh;

  // サーバーからの更新の知らせは、取得した内容より新しいことがある。知らせを
  // 受けた動画を覚えておき、ページを反映したあとで取り直す（pageLoading・
  // tagsChangedWhileLoading は上で宣言済み）。
  const changedWhileLoading = useRef(new Set<number>());

  // 切り替えが一部の動画にしか反映されなかったときは、どれが切り替わったか
  // 分からないので、表示中の該当の動画をサーバーから取り直す（更新の知らせと
  // 同じ扱い。Devin の指摘、PR 292）。取り直しとページの取得が終わる（または
  // 一覧を離れる）まで解決しない Promise を返し、その間は一覧の控えを取らせない。
  // 取り直しの途中で動画を開くと、古い項目が控えられて戻ったときに復元される
  // （Devin の指摘、PR 338）。取り直しが一時的に失敗した動画は古い `public` の
  // まま残るので、それが取り直せるまでも決着させない（uncertain）。
  useEffect(() => {
    const waiters = idleWaiters.current;
    const pending = uncertain.current;
    // お気に入りの付け外しが一部の動画にしか反映されなかったときも同じに取り直す
    // （specs/035-favorites/research.md R-6）。
    // 理由ごとに分けて覚え、全件に反映された結果はその理由だけを確かにする。
    const onStale = (reason: UncertainReason) => (videoIds: readonly number[]) => {
      const targets = new Set(videoIds);
      staleNotices.current += 1;
      const notice = staleNotices.current;
      if (pageLoading.current) {
        // 取得中のページは切り替えの前に読まれたかもしれない。届いたページに
        // 現れる対象は取り直し、現れない対象はそこで uncertain から外す（fetchPage）。
        for (const id of targets) {
          changedWhileLoading.current.add(id);
          markUncertain(id, reason, notice);
        }
      }
      const shown = shownVideoIds(itemsRef.current).filter((id) => targets.has(id));
      if (!pageLoading.current && shown.length === 0) return undefined;
      for (const id of shown) markUncertain(id, reason, notice);
      const settled = new Promise<void>((resolve) => {
        waiters.push(resolve);
      });
      refreshItems(shown);
      notifyIfIdle();
      return settled;
    };
    const unsubscribeVisibility = subscribeVideoVisibilityStale(onStale("visibility"));
    const unsubscribeFavorites = subscribeFavoritesStale(onStale("favorite"));
    return () => {
      unsubscribeVisibility();
      unsubscribeFavorites();
      pending.clear();
      for (const resolve of waiters.splice(0)) resolve();
    };
  }, [idleWaiters, markUncertain, notifyIfIdle, refreshItems, staleNotices, uncertain]);

  // 公開・非公開の切り替えの結果も、タグの付け外しと同じく一覧を読み直さずに
  // 表示中の項目へ反映する（issue 305）。取得の間に反映した切り替えは、
  // 届いたページと取り直した1件へ withVisibilitySince で重ねる（fetchPage・
  // drainRefreshQueue）。件数の上限で記録を落とさない（PR 328）。
  //
  // 全件に反映された切り替えの結果は、その動画の確かな公開状態でもある。
  // 一部反映の取り直しに失敗して uncertain に残った動画も、公開状態については
  // これで確かになり、ほかの理由が無ければ控えを取れるようになる（Devin の指摘、PR 338）。同じ動画への切り替えは
  // visibility が要求の通し番号で順序を保ち、後から送った一部反映より古い
  // 全件反映の結果はここに届かない（recordApplied・recordUncertain の applied）。
  useEffect(
    () =>
      subscribeVideoVisibility((videoIds, isPublic) => {
        settleUncertainReason(videoIds, "visibility");
        dispatch({ type: "visibility", videoIds, isPublic });
        notifyIfIdle();
      }),
    [notifyIfIdle, settleUncertainReason],
  );

  // お気に入りの付け外しの結果も一覧を読み直さずに反映する（specs/035-favorites/research.md R-6）。
  // 動画の項目は `favorite` をその場で差し替える。メンバーの付け外しはグループの値を
  // 変えないので、グループは取り直さない（要件 4）。付け外したグループの項目は
  // `GET /api/folders/{rootId}/group` で取り直し、404 なら外す（useGroupRefresh）。
  // お気に入りのみで絞った一覧や「Date favorited」の並びでも、その場では外さず並べ替えない。
  //
  // 確かに反映された動画は、一部反映の取り直しに失敗してお気に入りの印が確かでなかった
  // ものでも、これで確かになる（公開状態の理由は残す。useItemRefresh の uncertain）。
  // ページの取得中に付け外したグループは、取得中のページに初めて現れることがあるので
  // 覚えておき、ページを反映したあとで取り直す（useVideoPages の fetchPage）。
  // 反映の数が 0 の（もうグループでない）フォルダも、取り直して 404 で外すために覚える。
  useEffect(
    () =>
      subscribeFavorites(({ videoIds, folders, favorite }) => {
        if (videoIds.length > 0) {
          settleUncertainReason(videoIds, "favorite");
          dispatch({ type: "favorite", videoIds, favorite });
          notifyIfIdle();
        }
        if (folders.length === 0) return;
        if (pageLoading.current) {
          for (const folder of folders) {
            favoriteFoldersChangedWhileLoading.current.set(folderRefKey(folder), folder);
          }
        }
        const wanted = new Set(folders.map(folderRefKey));
        refreshGroups(
          itemsRef.current.flatMap((item) =>
            item.kind === "group" && wanted.has(folderRefKey(groupRef(item.group)))
              ? [groupRef(item.group)]
              : [],
          ),
        );
      }),
    [notifyIfIdle, refreshGroups, settleUncertainReason],
  );

  // 変化の知らせ（/api/events）は所有者だけのものなので、ゲストでは購読しない
  // （specs/016-single-account-auth/ui-design.md「Top bar」）。
  const owner = useAudience() === "owner";
  useEffect(() => {
    const queue = refreshQueue.current;
    const unsubscribe = owner
      ? subscribeServerEvents({
          video: (id) => {
            // ページの取得中は、表示中の動画でも覚えておく。取り直しの方が先に
            // 終わると、あとから届いたページの古い内容で上書きされる。
            if (pageLoading.current) changedWhileLoading.current.add(id);
            if (shownVideoIds(itemsRef.current).includes(id)) refreshItems([id]);
            refreshGroupsWith([id]);
          },
          // つなぎ直したときは、切れていた間の知らせを受け取っていない。準備が
          // 済んだ動画も消えているかもしれないので、表示中の項目をすべて取り直す
          // （消えていれば一覧から外れる）。最初の接続では、準備中の項目だけでよい。
          open: (reconnected) => {
            if (reconnected) {
              refreshItems(shownVideoIds(itemsRef.current));
              refreshGroups(
                itemsRef.current.flatMap((item) =>
                  item.kind === "group" ? [groupRef(item.group)] : [],
                ),
              );
            } else {
              refreshProcessingItems();
            }
          },
        })
      : () => undefined;
    // 復元した一覧は、別の画面にいた間に準備が進んでいることがある。
    refreshProcessingItems();
    // 控えの後にメンバーが変わったグループも取り直す（ListSnapshot.staleGroups）。
    const stale = new Set(staleGroups.current.map(folderRefKey));
    if (stale.size > 0) {
      refreshGroups(
        itemsRef.current.flatMap((item) =>
          item.kind === "group" && stale.has(folderRefKey(groupRef(item.group)))
            ? [groupRef(item.group)]
            : [],
        ),
      );
    }
    const groups = groupQueue.current;
    return () => {
      unsubscribe();
      refreshing.current?.abort();
      refreshing.current = null;
      queue.clear();
      groupRefreshing.current?.abort();
      groupRefreshing.current = null;
      groups.clear();
    };
  }, [
    groupQueue,
    groupRefreshing,
    owner,
    refreshGroups,
    refreshGroupsWith,
    refreshItems,
    refreshProcessingItems,
    refreshQueue,
    refreshing,
    staleGroups,
  ]);

  const { loadMore, retryLoadMore, reload, refreshInPlace } = useVideoPages({
    key,
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
    favoriteFoldersChangedWhileLoading,
    tagsChangedWhileLoading,
    groups,
    itemRefresh,
    seed,
    generation,
    inconsistent,
    cursor,
    hasMore,
    loading,
    loadingMore,
  });

  return {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    notFound,
    missingTagIds,
    loadMore,
    retryLoadMore,
    reload,
    refreshInPlace,
    staleGroups: unsettledGroupRefs,
  };
}
