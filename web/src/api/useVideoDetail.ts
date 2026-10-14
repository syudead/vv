import { useCallback, useEffect, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import { errorText, type UiText } from "../i18n";
import {
  getRelatedVideos,
  getVideo,
  isAborted,
  RequestFailed,
  type RelatedVideos,
  type Video,
} from "./client";
import {
  favoriteMark,
  favoriteReadMark,
  noteFavoriteOf,
  subscribeFavorites,
  subscribeFavoritesStale,
  updateFavorites,
  withFavoriteSince,
} from "./favorites";
import { subscribeServerEvents } from "./serverEvents";
import {
  subscribeVideoVisibility,
  subscribeVideoVisibilityStale,
  visibilityMark,
  withVisibilitySince,
} from "./visibility";

export type VideoDetailState =
  | { kind: "loading"; id: number }
  | { kind: "ready"; id: number; video: Video }
  /** 動画が無い（404、または id の形が正しくない）。取り直しで消えたときもこれになる。 */
  | { kind: "missing"; id: number }
  /** 最初の取得が 404 以外で失敗した。 */
  | { kind: "failed"; id: number; reason: UiText };

/**
 * isProcessing は、取り込みの処理が残っているかを返す（plan の Structural
 * Decisions 7・11）。
 *
 * 読み取りに失敗した動画は、そこで終わりとして扱う。一覧用プレビューのジョブは読み取りの
 * 成功後にしか積まれないので、失敗した動画の `previewState` は `pending` のまま残る。
 * それを「処理中」と読むと、処理中の表示が終わらない。
 */
export function isProcessing(video: Video): boolean {
  if (video.probeState === "pending") return true;
  if (video.probeState !== "done") return false;
  return (
    video.thumbnailState === "pending" ||
    video.seekThumbnailState === "pending" ||
    video.previewState === "pending"
  );
}

/** loadsStarted は、どの画面でも動画 1 件の取得を始めるたびに進める通し番号である。 */
let loadsStarted = 0;

/**
 * DetailMark は、変更の要求を送る直前の時点を表す印である。応答で動画を差し替える
 * `replace` に渡し、要求より前に始まったものと後に起きたものを分ける。
 */
export type DetailMark = { load: number; visibility: number; favorite: number };

/** detailMark は変更の要求（表示名の保存）を送る直前に呼び、その時点の印を返す。 */
export function detailMark(): DetailMark {
  return { load: loadsStarted, visibility: visibilityMark(), favorite: favoriteMark() };
}

/**
 * ReplaceWorkInfo は、`workInfo` を載せない変更の応答で動画を差し替えるときの作品情報の扱いである。
 * - `keep`: 手元の作品情報を残す。作品情報を変えない変更（表示名・代表サムネイル）の応答に使う。
 * - `refetch`: 手元の作品情報を残さず、差し替えた後に動画を取り直して作品情報を読む。作品情報が
 *   変わる変更（束から外すと、外した動画は束ねる前の固有の作品情報に戻る）の応答に使う。
 */
export type ReplaceWorkInfo = "keep" | "refetch";

/**
 * withWorkInfoOf は、`workInfo` を載せない変更の応答 saved に、手元の動画 current の作品情報を
 * 戻す。応答が `workInfo` を持つとき、手元に無いときは saved のままである。
 */
function withWorkInfoOf(saved: Video, current: Video | undefined): Video {
  if (saved.workInfo !== undefined || current?.workInfo === undefined) return saved;
  return { ...saved, workInfo: current.workInfo };
}

/**
 * useVideoDetail は動画 1 件を取得し、その動画が変わったという知らせを受けたら
 * 取り直す。一定間隔では問い合わせない。
 *
 * - 知らせの接続をつなぎ直したときは、切れていた間の変化を取り戻すために取り直す。
 * - 別の動画へ移ったとき、画面を離れたときは、送信中の要求を打ち切って止める。
 * - 取り直しが 404 を返したら `missing` にする（表示中の動画がライブラリから消えた）。
 *   それ以外の一時的な失敗では、手元の控えを残す。
 *
 * `refresh` はすぐに取り直し、その取得が終わったら解決する。
 *
 * `replace` は、変更の要求の応答で受け取った動画を手元の 1 件にする（表示名の保存。
 * specs/029-video-overrides/ui-design.md「Save」）。`mark` は要求を送る直前に取った
 * detailMark である。
 * - 要求より前に始めた取り直しは打ち切る。その応答で、変更した値を巻き戻さないため。
 *   要求の後に始めた取り直しは残す（公開の一部反映で始めた取り直しなど）。
 * - 要求の間に反映した公開の切り替えとお気に入りの付け外しは、応答の `public`・`favorite`
 *   より優先する。どちらもサーバーから知らせが来ないので、ここで巻き戻すと直らない。
 * - replace に渡る応答（表示名・代表サムネイル・束から外す）は契約上 `workInfo` を載せない
 *   （specs/047-video-work-info/contracts/screen-api.md「Video changes」）。`workInfo` が
 *   `keep` なら、応答に無い作品情報は手元の値を残す（そのまま置くと情報欄から作品情報が消える）。
 *   `refetch` なら手元の値を残さず、差し替えた後に取り直す。束から外した動画は束の作品情報から
 *   固有の作品情報に戻るので、手元の値を残すと別の作品情報を出し、詳細の保存で上書きしかねない。
 * - 別の動画の応答は捨てる。
 *
 * `setFavorite` は表示中の動画のお気に入りを `PUT /api/favorites` で付け外しし、成功したら
 * 取り直して、その取得が終わったら解決する（specs/035-favorites/ui-design.md「Video page」）。
 * 塗りは取り直した `favorite` で変える。付け外しの結果の知らせはこの 1 件へ重ねないので、
 * 取り直しの間と取り直しに失敗したときは前の状態のままである。失敗は reject で返す。
 */
export function useVideoDetail(
  id: number,
  initial?: Video,
): {
  state: VideoDetailState;
  refresh: () => Promise<void>;
  replace: (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) => void;
  setFavorite: (favorite: boolean) => Promise<void>;
} {
  const [state, setState] = useState<VideoDetailState>(() =>
    initial !== undefined && initial.id === id
      ? { kind: "ready", id, video: initial }
      : { kind: "loading", id },
  );
  // 手元にある応答（編集画面の保存の応答）から始める。最初の取得までの間もそれを出し、
  // 古い値や読み込み中を挟まない（specs/047-video-work-info/ui-design.md「Save」）。
  // 使うのは最初の 1 回だけで、別の動画へ移ったら使わない。
  const seed = useRef(initial);
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  const replaceRef = useRef<
    (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) => void
  >(() => undefined);
  const setFavoriteRef = useRef<(favorite: boolean) => Promise<void>>(() =>
    Promise.resolve(),
  );
  // 変化の知らせ（/api/events）は所有者だけのものなので、ゲストでは購読しない。
  const owner = useAudience() === "owner";

  useEffect(() => {
    // StrictMode が効果を 2 度走らせても同じ動画には使えるよう、別の動画へ移ったときに捨てる。
    const seeded = seed.current?.id === id ? seed.current : undefined;
    if (seeded === undefined) seed.current = undefined;
    setState(
      seeded === undefined
        ? { kind: "loading", id }
        : { kind: "ready", id, video: seeded },
    );
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "missing", id });
      refreshRef.current = () => Promise.resolve();
      replaceRef.current = () => undefined;
      setFavoriteRef.current = () => Promise.resolve();
      return;
    }

    let alive = true;
    let controller: AbortController | null = null;
    // 送信中の取り直しを始めたときの loadsStarted の値。
    let started = 0;
    let current: Video | undefined = seeded;
    // current を読んだ取得を始めたときの loadsStarted の値と favoriteReadMark の値。
    let currentLoad = 0;
    let currentRead = 0;
    const waiters: (() => void)[] = [];
    // この画面の付け外しを送っている数。その間に届く付け外しの結果の知らせは重ねない。
    let ownFavorites = 0;

    const settle = () => {
      for (const resolve of waiters.splice(0)) resolve();
    };

    const load = async () => {
      controller?.abort();
      const mine = new AbortController();
      controller = mine;
      loadsStarted += 1;
      started = loadsStarted;
      // 取得の間に公開を切り替えたら、切り替える前の `public` を読んだ応答で
      // 表示を巻き戻さない（切り替えはサーバーから知らせが来ない。PR 328）。
      // お気に入りの付け外しも同じ（specs/035-favorites/research.md R-6）。
      const mark = visibilityMark();
      const favorite = favoriteMark();
      const read = favoriteReadMark();
      const loadStarted = started;
      try {
        const video = withFavoriteSince(
          withVisibilitySince(await getVideo(id, mine.signal), mark),
          favorite,
        );
        if (!alive || controller !== mine) return;
        current = video;
        currentLoad = loadStarted;
        currentRead = read;
        setState({ kind: "ready", id, video });
        settle();
      } catch (failure) {
        if (!alive || controller !== mine || isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          current = undefined;
          setState({ kind: "missing", id });
          settle();
          return;
        }
        if (current === undefined) {
          setState({ kind: "failed", id, reason: errorText(failure) });
        }
        settle();
      }
    };

    refreshRef.current = () =>
      new Promise<void>((resolve) => {
        if (!alive) {
          resolve();
          return;
        }
        waiters.push(resolve);
        void load();
      });

    replaceRef.current = (saved, mark, workInfo) => {
      if (!alive || saved.id !== id) return;
      if (controller !== null && started <= mark.load) {
        controller.abort();
        controller = null;
        settle();
      }
      const video = withFavoriteSince(
        withVisibilitySince(
          workInfo === "keep" ? withWorkInfoOf(saved, current) : saved,
          mark.visibility,
        ),
        mark.favorite,
      );
      current = video;
      // 保存の応答はいつ読んだ値か分からないので、代表へは知らせない（setFavorite）。
      currentLoad = 0;
      setState({ kind: "ready", id, video });
      // 先に終わった `video` の知らせの取り直しを、この応答で上書きしたかもしれないので取り直す。
      if (workInfo === "refetch") void load();
    };

    setFavoriteRef.current = async (favorite) => {
      ownFavorites += 1;
      try {
        await updateFavorites([id], [], favorite);
      } finally {
        ownFavorites -= 1;
      }
      // 付け外しの後に始めた取得で読んだ値だけを代表へ知らせる。取り直しが失敗したときの
      // current は付け外しの前の値なので、知らせると代表を古い値で上書きしてしまう。
      const after = loadsStarted;
      await refreshRef.current();
      // 代表以外のバージョンの付け外しは集まりの値を変えるが、一覧に出ているのは代表なので、
      // 取り直した値で代表の項目と一覧の控えを揃える（noteFavoriteOf）。
      const representative = current?.versions?.representativeId;
      if (
        alive &&
        current !== undefined &&
        currentLoad > after &&
        current.favorite !== undefined &&
        representative !== undefined &&
        representative !== id
      ) {
        noteFavoriteOf([representative], current.favorite, currentRead);
      }
    };

    // 公開・非公開の切り替えの結果は、取り直さずに手元の1件へ重ねる
    // （issue 305。再生画面の切り替えは応答を受けてからこれで状態が変わる）。
    const unsubscribeVisibility = subscribeVideoVisibility((videoIds, isPublic) => {
      if (current === undefined || !videoIds.includes(id)) return;
      if (current.public === isPublic) return;
      current = { ...current, public: isPublic };
      setState({ kind: "ready", id, video: current });
    });
    // 一部にしか反映されなかった切り替えは、どれが切り替わったか分からないので、
    // この1件を取り直してサーバーの状態を表示する（Devin の指摘、PR 292）。
    // 再生画面は一覧の控えを取らないので、取り直しの決着は知らせない。
    const unsubscribeStale = subscribeVideoVisibilityStale((videoIds) => {
      if (videoIds.includes(id)) void load();
      return undefined;
    });

    // お気に入りの付け外しの結果も、取り直さずに手元の1件へ重ねる。ゲストの応答には
    // `favorite` が無いので、無いものは足さない（specs/035-favorites/research.md R-6）。
    // この画面の付け外しは setFavorite の取り直しで確かめるので、ここでは重ねない。
    const unsubscribeFavorites = subscribeFavorites((change) => {
      if (ownFavorites > 0) return;
      if (current === undefined || !change.videoIds.includes(id)) return;
      if (current.favorite === undefined || current.favorite === change.favorite) return;
      current = { ...current, favorite: change.favorite };
      setState({ kind: "ready", id, video: current });
    });
    const unsubscribeFavoritesStale = subscribeFavoritesStale((videoIds) => {
      if (videoIds.includes(id)) void load();
      return undefined;
    });

    // 購読してから取得する。取得のあとに起きた変化を取りこぼさない。
    const unsubscribe = owner
      ? subscribeServerEvents({
          video: (changed) => {
            if (changed === id) void load();
          },
          // つなぎ直したときは、切れていた間の知らせを受け取っていないので取り直す。
          // 最初の接続では、開いたときの取得と同じ内容を2回続けて取らないよう、まだ準備中
          // （取得とつながるまでの間に準備が済んだかもしれない）のときだけ取り直す（issue 674）。
          open: (reconnected) => {
            if (reconnected || (current !== undefined && isProcessing(current)))
              void load();
          },
        })
      : () => undefined;
    void load();
    return () => {
      alive = false;
      controller?.abort();
      unsubscribe();
      unsubscribeVisibility();
      unsubscribeStale();
      unsubscribeFavorites();
      unsubscribeFavoritesStale();
      settle();
    };
  }, [id, owner]);

  const refresh = useCallback(() => refreshRef.current(), []);
  const replace = useCallback(
    (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) =>
      replaceRef.current(video, mark, workInfo),
    [],
  );
  const setFavorite = useCallback(
    (favorite: boolean) => setFavoriteRef.current(favorite),
    [],
  );
  return { state, refresh, replace, setFavorite };
}

export type RelatedState =
  | { kind: "loading"; id: number }
  | {
      kind: "ready";
      id: number;
      /** 読んだページを合わせた関連動画。`nextCursor` は最後に読んだページのもの。 */
      related: RelatedVideos;
      /** 続きのページの読み込み。省くと `idle`。 */
      more?: "idle" | "loading" | "failed";
    }
  | { kind: "failed"; id: number };

/**
 * useRelatedVideos は関連動画の最初のページを取得する。失敗したら `retry` で取り直せる。
 *
 * `loadMore` は続きのページ（`nextCursor` の後）を読み、並びの後ろに足す（issue 931）。
 * 読み込み中と続きが無いときは何もしない。すでにある動画は足さないので、読む間に並びが
 * 変わっても同じ動画が二重に出ない。失敗したら `more` が `failed` になり、もう一度
 * `loadMore` で読み直せる。
 *
 * `rename` は、基準の動画の表示名を保存した応答を、グループのメンバーの並びにある
 * 同じ動画へ写す（並びは基準の動画も含む）。取り直さずに、並びの題名を画面の題名とそろえる。
 * `rethumb` は、基準の動画の代表サムネイルを指定・解除した応答を同じように写し、
 * 並びのサムネイル（URL の版が変わる）を画面のサムネイルとそろえる。
 */
export function useRelatedVideos(id: number): {
  state: RelatedState;
  retry: () => void;
  loadMore: () => void;
  rename: (video: Video) => void;
  rethumb: (video: Video) => void;
} {
  const [state, setState] = useState<RelatedState>({ kind: "loading", id });
  const [attempt, setAttempt] = useState(0);
  const stateRef = useRef(state);
  useEffect(() => {
    stateRef.current = state;
  }, [state]);
  const moreAbort = useRef<AbortController | null>(null);

  useEffect(() => {
    setState({ kind: "loading", id });
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "ready", id, related: { items: [] }, more: "idle" });
      return;
    }
    const controller = new AbortController();
    void (async () => {
      try {
        const related = await getRelatedVideos(id, controller.signal);
        setState({ kind: "ready", id, related, more: "idle" });
      } catch (failure) {
        if (isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          setState({ kind: "ready", id, related: { items: [] }, more: "idle" });
          return;
        }
        setState({ kind: "failed", id });
      }
    })();
    return () => {
      controller.abort();
      moreAbort.current?.abort();
      moreAbort.current = null;
    };
  }, [attempt, id]);

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (current.kind !== "ready" || current.more === "loading") return;
    const cursor = current.related.nextCursor;
    if (cursor === undefined) return;
    const controller = new AbortController();
    moreAbort.current = controller;
    const loading = { ...current, more: "loading" as const };
    stateRef.current = loading;
    setState(loading);
    getRelatedVideos(current.id, controller.signal, cursor).then(
      (page) => {
        if (controller.signal.aborted) return;
        setState((previous) => {
          if (previous.kind !== "ready" || previous.id !== current.id) return previous;
          const seen = new Set(previous.related.items.map((item) => item.id));
          const added = page.items.filter((item) => !seen.has(item.id));
          const related: RelatedVideos = {
            ...previous.related,
            items: [...previous.related.items, ...added],
          };
          delete related.nextCursor;
          if (page.nextCursor !== undefined) related.nextCursor = page.nextCursor;
          return { ...previous, related, more: "idle" };
        });
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setState((previous) =>
          previous.kind === "ready" && previous.id === current.id
            ? { ...previous, more: "failed" }
            : previous,
        );
      },
    );
  }, []);

  const retry = useCallback(() => setAttempt((value) => value + 1), []);
  // patchMember は、グループのメンバーの並びにある基準の動画 video.id の項目を patch で
  // 書き換える。並びに無ければ何も変えない。
  const patchMember = useCallback((video: Video, patch: (item: Video) => Video) => {
    setState((previous) => {
      if (previous.kind !== "ready" || previous.id !== video.id) return previous;
      const group = previous.related.group;
      if (group === undefined || !group.items.some((item) => item.id === video.id)) {
        return previous;
      }
      const items = group.items.map((item) =>
        item.id === video.id ? patch(item) : item,
      );
      return {
        ...previous,
        related: { ...previous.related, group: { ...group, items } },
      };
    });
  }, []);
  const rename = useCallback(
    (video: Video) =>
      patchMember(video, (item) => {
        const renamed: Video = { ...item, title: video.title };
        delete renamed.displayName;
        if (video.fileTitle !== undefined) renamed.fileTitle = video.fileTitle;
        if (video.displayName !== undefined) renamed.displayName = video.displayName;
        return renamed;
      }),
    [patchMember],
  );
  const rethumb = useCallback(
    (video: Video) =>
      patchMember(video, (item) => {
        const updated: Video = { ...item };
        delete updated.thumbnailUrl;
        delete updated.thumbnailPositionMs;
        if (video.thumbnailUrl !== undefined) updated.thumbnailUrl = video.thumbnailUrl;
        if (video.thumbnailPositionMs !== undefined) {
          updated.thumbnailPositionMs = video.thumbnailPositionMs;
        }
        return updated;
      }),
    [patchMember],
  );
  return { state, retry, loadMore, rename, rethumb };
}
