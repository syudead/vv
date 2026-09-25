import { useCallback, useEffect, useReducer, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import {
  errorMessage,
  type FolderRef,
  type FolderScope,
  getFolderGroup,
  getVideo,
  isAborted,
  type LibraryGroup,
  type LibraryItem,
  listFolderVideos,
  listVideos,
  RequestFailed,
  type TagRef,
  type Video,
  type VideoSort,
  type WatchFilter,
} from "./client";
import {
  folderRefKey,
  groupRef,
  groupsWithMembers,
  itemKey,
  itemVideos,
  videoItem,
} from "./libraryItems";
import { subscribeProgress } from "./progressEvents";
import { subscribeServerEvents } from "./serverEvents";
import { applyTagToTags } from "./tagOrder";
import { isProcessing } from "./useVideoDetail";
import { subscribeVideoTags } from "./videoTagsEvents";
import {
  subscribeVideoVisibility,
  subscribeVideoVisibilityStale,
  visibilityMark,
  withVisibilitySince,
} from "./visibility";

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
  items: LibraryItem[];
  total: number;
  cursor?: string;
  hasMore: boolean;
  /**
   * 控えを取った後にメンバーが変わったグループの項目（ListSnapshot.staleGroups）。
   * 戻ったときにこれを取り直す。
   */
  staleGroups?: readonly FolderRef[];
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
  /**
   * 絞り込むタグの id（listVideos だけが受け取る。listFolderVideos には渡さない。
   * specs/014-video-tags/contracts/tags-api.md §5）。
   */
  tag?: number[];
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
    criteria.tag ?? [],
  ]);
}

/**
 * appendUnique は続きのページから、既に出ている項目を捨てて足す（list-api.md §5）。
 * 動画の項目は動画の id、グループの項目はフォルダで比べる（itemKey）。
 */
function appendUnique(current: LibraryItem[], next: LibraryItem[]): LibraryItem[] {
  const seen = new Set(current.map(itemKey));
  const added: LibraryItem[] = [];
  for (const item of next) {
    const key = itemKey(item);
    if (seen.has(key)) continue;
    seen.add(key);
    added.push(item);
  }
  return added.length === 0 ? current : [...current, ...added];
}

/**
 * mapVideos は動画の項目のうち `match` に当たるものだけを `update` で書き換える。
 * 当たる項目が無ければ受け取った配列をそのまま返す（描画し直さないため）。
 */
function mapVideos(
  items: LibraryItem[],
  match: (video: Video) => boolean,
  update: (video: Video) => Video,
): LibraryItem[] {
  if (!items.some((item) => item.kind === "video" && match(item.video))) return items;
  return items.map((item) =>
    item.kind === "video" && match(item.video) ? videoItem(update(item.video)) : item,
  );
}

/** shownVideoIds は動画の項目の id である（グループのメンバーは含まない）。 */
function shownVideoIds(items: readonly LibraryItem[]): number[] {
  return itemVideos(items).map((video) => video.id);
}

interface VideosData {
  items: LibraryItem[];
  total: number;
  cursor: string | undefined;
  hasMore: boolean;
  /** inconsistent は異なる時点のページを安全に結合できなかったことを表す。 */
  inconsistent: boolean;
  /**
   * missingTagIds は、直近の要求で `tag` に指定したがもう無かった id である
   * （specs/014-video-tags/contracts/tags-api.md §5）。folder を渡す（`tag` を
   * 送らない）ときは常に空。
   */
  missingTagIds: number[];
}

type VideosDataAction =
  | { type: "clear" }
  | { type: "stop" }
  | { type: "progress"; videoId: number; progress: NonNullable<Video["progress"]> }
  | {
      type: "tags";
      videoIds: readonly number[];
      tag: TagRef;
      action: "add" | "remove";
    }
  | { type: "visibility"; videoIds: readonly number[]; isPublic: boolean }
  | { type: "refresh"; videoId: number; video: Video }
  | { type: "remove"; videoId: number }
  | { type: "refreshGroup"; folderKey: string; group: LibraryGroup }
  | { type: "removeGroup"; folderKey: string }
  | {
      type: "page";
      page: {
        items: LibraryItem[];
        total: number;
        nextCursor?: string;
        missingTagIds?: number[];
      };
      replace: boolean;
    };

function videosDataReducer(state: VideosData, action: VideosDataAction): VideosData {
  switch (action.type) {
    case "clear":
      return {
        items: [],
        total: 0,
        cursor: undefined,
        hasMore: true,
        inconsistent: false,
        missingTagIds: [],
      };
    case "stop":
      return state.hasMore ? { ...state, hasMore: false } : state;
    // 再生位置・タグ・公開・取り直しは動画の項目にだけ直接重ねる。グループの値は
    // メンバーから数えるので、メンバーの変化ではグループを1件取り直す（useVideos の
    // refreshGroups。specs/017-folder-groups/plan.md の Structural Decisions 10）。
    case "progress": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => ({ ...video, progress: action.progress }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "tags": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id),
        (video) => ({
          ...video,
          tags: applyTagToTags(video.tags, action.tag, action.action),
        }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "visibility": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id) && video.public !== action.isPublic,
        (video) => ({ ...video, public: action.isPublic }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "refresh": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => mergeRefreshed(video, action.video),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "remove": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "video" && item.video.id === action.videoId;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.filter((item) => !isTarget(item)),
        total: Math.max(0, state.total - 1),
      };
    }
    case "refreshGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && folderRefKey(groupRef(item.group)) === action.folderKey;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.map((item) =>
          isTarget(item) ? { kind: "group", group: action.group } : item,
        ),
      };
    }
    case "removeGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && folderRefKey(groupRef(item.group)) === action.folderKey;
      if (!state.items.some(isTarget)) return state;
      // total は次に一覧を読むまでそのままにする（ui-design.md「Refresh and removal」）。
      return { ...state, items: state.items.filter((item) => !isTarget(item)) };
    }
    case "page": {
      const missingTagIds = action.page.missingTagIds ?? [];
      if (action.replace) {
        return {
          items: action.page.items,
          total: action.page.total,
          cursor: action.page.nextCursor,
          hasMore: action.page.nextCursor !== undefined,
          inconsistent: false,
          missingTagIds,
        };
      }
      const items = appendUnique(state.items, action.page.items);
      // ページ間で索引が縮むと、前のページにだけ残る項目と最新の total を
      // 安全に結合できない。古い整合した状態を保ち、先頭からの再読込を求める。
      if (items.length > action.page.total) {
        return { ...state, hasMore: false, inconsistent: true };
      }
      return {
        items,
        total: action.page.total,
        cursor: action.page.nextCursor,
        hasMore: action.page.nextCursor !== undefined,
        inconsistent: false,
        missingTagIds,
      };
    }
  }
}

const inconsistentPageMessage =
  "一覧の更新が続いているため取得できません。しばらくしてから再試行してください。";

/** VideosState は一覧の状態である。 */
export interface VideosState {
  /**
   * 読み込み済みの項目。動画の項目とグループの項目がある（グループはライブラリの
   * 一覧にだけ現れる。specs/017-folder-groups/plan.md の Structural Decisions 13）。
   */
  items: LibraryItem[];
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
  /**
   * missingTagIds は、直近の要求の `tag` のうちもう無かった id である
   * （specs/014-video-tags/contracts/tags-api.md §5）。画面はこれを受けて、
   * もう無いことを伝え、タグの一覧を取り直し、URL から取り除く。
   */
  missingTagIds: number[];
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
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [generation, setGeneration] = useState(0);
  const resyncAttempted = useRef(false);

  // 条件や場所が変われば、前の一覧で使った再同期回数は引き継がない。
  useEffect(() => {
    resyncAttempted.current = false;
  }, [folderKey, key]);

  const itemsRef = useRef(items);
  itemsRef.current = items;

  // グループの項目は、メンバーの再生位置・タグ・`video` イベントの変化で
  // `GET /api/folders/{rootId}/group` から1件ずつ取り直して差し替える
  // （specs/017-folder-groups/plan.md の Structural Decisions 10、ui-design.md
  // 「Refresh and removal」）。取り直しの間は項目を変えず、404 ならその項目を外す。
  // 取り直しは1件ずつ順に行い、同じグループを重ねて取りに行かない（動画の
  // 取り直しの refreshQueue と同じ形）。取り直しの途中に同じグループが変われば、
  // 終わった後にもう一度取る。
  const groupQueue = useRef(new Map<string, FolderRef>());
  const groupRefreshing = useRef<AbortController | null>(null);
  // staleGroups は、控えから戻った一覧のうち、控えの後にメンバーが変わったグループである。
  const staleGroups = useRef<readonly FolderRef[]>(seed?.staleGroups ?? []);
  const drainGroupQueue = useCallback(async () => {
    if (groupRefreshing.current !== null) return;
    const controller = new AbortController();
    groupRefreshing.current = controller;
    try {
      for (const [groupKey, folder] of groupQueue.current) {
        groupQueue.current.delete(groupKey);
        try {
          const group = await getFolderGroup(folder, controller.signal);
          // 条件を変えて読み直した後に届いた古い取り直しは、新しい一覧に重ねない。
          if (controller.signal.aborted) return;
          dispatch({ type: "refreshGroup", folderKey: groupKey, group });
        } catch (failure) {
          if (isAborted(failure) || controller.signal.aborted) return;
          // 今はグループでない（例外で単体に戻った等）か、フォルダが無い。何も伝えずに
          // 外す。一時的な失敗は、その1件だけ諦める（次の変化か読み直しで直る）。
          if (failure instanceof RequestFailed && failure.status === 404) {
            dispatch({ type: "removeGroup", folderKey: groupKey });
          }
        }
      }
    } finally {
      if (groupRefreshing.current === controller) groupRefreshing.current = null;
    }
  }, []);
  const refreshGroups = useCallback(
    (folders: Iterable<FolderRef>) => {
      let queued = false;
      for (const folder of folders) {
        groupQueue.current.set(folderRefKey(folder), folder);
        queued = true;
      }
      if (queued) void drainGroupQueue();
    },
    [drainGroupQueue],
  );
  // refreshGroupsWith は `videoIds` のどれかをメンバーに持つ表示中のグループを取り直す。
  const refreshGroupsWith = useCallback(
    (videoIds: Iterable<number>) => {
      refreshGroups(groupsWithMembers(itemsRef.current, videoIds));
    },
    [refreshGroups],
  );

  // 再生画面で保存された再生位置を、表示中の項目へ反映する。復元した一覧は
  // 再生前の中身なので、戻ったあとに届く離脱時の保存もここで受ける。
  useEffect(
    () =>
      subscribeProgress((videoId, progress) => {
        dispatch({ type: "progress", videoId, progress });
        refreshGroupsWith([videoId]);
      }),
    [refreshGroupsWith],
  );

  // ページの取得中に届いた知らせは、取得した内容より新しいことがある
  // （下の「取り込みの準備」の取り直しと、その次のタグの付け外しの両方が使う）。
  const pageLoading = useRef(false);

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

  // 付け外しの結果を、表示中の項目へ反映する（issue 267、Plan の Structural
  // Decisions 7）。絞り込みに合わなくなった項目も、その場では一覧から外さない。
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
  const uncertain = useRef(new Map<number, number>());
  const staleNotices = useRef(0);
  // settleUncertain は、`started`（取り直しを始めた時点の知らせの番号）以後の
  // 知らせで確かでないとされていない動画を、確かになったとして外す。
  const settleUncertain = useCallback((id: number, started: number) => {
    if ((uncertain.current.get(id) ?? 0) <= started) uncertain.current.delete(id);
  }, []);
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
  }, []);
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
          const refreshed = withVisibilitySince(
            await getVideo(id, controller.signal),
            mark,
          );
          // 条件を変えて読み直した後に届いた古い取り直しは、新しい一覧に重ねない。
          if (controller.signal.aborted) return;
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
  }, [notifyIfIdle, settleUncertain]);
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
  }, [refreshItems]);

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
    const unsubscribe = subscribeVideoVisibilityStale((videoIds) => {
      const targets = new Set(videoIds);
      staleNotices.current += 1;
      const notice = staleNotices.current;
      if (pageLoading.current) {
        // 取得中のページは切り替えの前に読まれたかもしれない。届いたページに
        // 現れる対象は取り直し、現れない対象はそこで uncertain から外す（fetchPage）。
        for (const id of targets) {
          changedWhileLoading.current.add(id);
          pending.set(id, notice);
        }
      }
      const shown = shownVideoIds(itemsRef.current).filter((id) => targets.has(id));
      if (!pageLoading.current && shown.length === 0) return undefined;
      for (const id of shown) pending.set(id, notice);
      const settled = new Promise<void>((resolve) => {
        waiters.push(resolve);
      });
      refreshItems(shown);
      notifyIfIdle();
      return settled;
    });
    return () => {
      unsubscribe();
      pending.clear();
      for (const resolve of waiters.splice(0)) resolve();
    };
  }, [notifyIfIdle, refreshItems]);

  // 公開・非公開の切り替えの結果も、タグの付け外しと同じく一覧を読み直さずに
  // 表示中の項目へ反映する（issue 305）。取得の間に反映した切り替えは、
  // 届いたページと取り直した1件へ withVisibilitySince で重ねる（fetchPage・
  // drainRefreshQueue）。件数の上限で記録を落とさない（PR 328）。
  //
  // 全件に反映された切り替えの結果は、その動画の確かな公開状態でもある。
  // 一部反映の取り直しに失敗して uncertain に残った動画も、これで確かになり、
  // 控えを取れるようになる（Devin の指摘、PR 338）。同じ動画への切り替えは
  // visibility が要求の通し番号で順序を保ち、後から送った一部反映より古い
  // 全件反映の結果はここに届かない（recordApplied・recordUncertain の applied）。
  useEffect(
    () =>
      subscribeVideoVisibility((videoIds, isPublic) => {
        for (const id of videoIds) uncertain.current.delete(id);
        dispatch({ type: "visibility", videoIds, isPublic });
        notifyIfIdle();
      }),
    [notifyIfIdle],
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
  }, [owner, refreshGroups, refreshGroupsWith, refreshItems, refreshProcessingItems]);

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
      if (replace) {
        // 前の一覧のために始めた取り直しは捨てる。新しいページの内容の方が新しい。
        refreshing.current?.abort();
        refreshing.current = null;
        refreshQueue.current.clear();
        groupRefreshing.current?.abort();
        groupRefreshing.current = null;
        groupQueue.current.clear();
        staleGroups.current = [];
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
          target === undefined
            ? await listVideos({ ...params, tag: current.tag })
            : await listFolderVideos({
                folder: target,
                scope: current.scope,
                ...params,
              });
        const page = {
          ...fetched,
          items: fetched.items.map((video) =>
            videoItem(withVisibilitySince(video, mark)),
          ),
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
            setError(inconsistentPageMessage);
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
        refreshGroups(groupsWithMembers(page.items, changedWhileLoading.current));
        changedWhileLoading.current.clear();
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
        setError(errorMessage(failure));
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
    setError(inconsistentPageMessage);
  }, [inconsistent]);

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

  const reload = useCallback(() => {
    resyncAttempted.current = false;
    setGeneration((value) => value + 1);
  }, []);

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
  };
}
