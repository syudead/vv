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
  subscribeFavorites,
  subscribeFavoritesStale,
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
 * - 別の動画の応答は捨てる。
 */
export function useVideoDetail(id: number): {
  state: VideoDetailState;
  refresh: () => Promise<void>;
  replace: (video: Video, mark: DetailMark) => void;
} {
  const [state, setState] = useState<VideoDetailState>({ kind: "loading", id });
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  const replaceRef = useRef<(video: Video, mark: DetailMark) => void>(() => undefined);
  // 変化の知らせ（/api/events）は所有者だけのものなので、ゲストでは購読しない。
  const owner = useAudience() === "owner";

  useEffect(() => {
    setState({ kind: "loading", id });
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "missing", id });
      refreshRef.current = () => Promise.resolve();
      replaceRef.current = () => undefined;
      return;
    }

    let alive = true;
    let controller: AbortController | null = null;
    // 送信中の取り直しを始めたときの loadsStarted の値。
    let started = 0;
    let current: Video | undefined;
    const waiters: (() => void)[] = [];

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
      try {
        const video = withFavoriteSince(
          withVisibilitySince(await getVideo(id, mine.signal), mark),
          favorite,
        );
        if (!alive || controller !== mine) return;
        current = video;
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

    replaceRef.current = (saved, mark) => {
      if (!alive || saved.id !== id) return;
      if (controller !== null && started <= mark.load) {
        controller.abort();
        controller = null;
        settle();
      }
      const video = withFavoriteSince(
        withVisibilitySince(saved, mark.visibility),
        mark.favorite,
      );
      current = video;
      setState({ kind: "ready", id, video });
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
    const unsubscribeFavorites = subscribeFavorites((change) => {
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
          open: () => void load(),
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
    (video: Video, mark: DetailMark) => replaceRef.current(video, mark),
    [],
  );
  return { state, refresh, replace };
}

export type RelatedState =
  | { kind: "loading"; id: number }
  | { kind: "ready"; id: number; related: RelatedVideos }
  | { kind: "failed"; id: number };

/**
 * useRelatedVideos は関連動画を 1 回取得する。失敗したら `retry` で取り直せる。
 *
 * `rename` は、基準の動画の表示名を保存した応答を、グループのメンバーの並びにある
 * 同じ動画へ写す（並びは基準の動画も含む）。取り直さずに、並びの題名を画面の題名とそろえる。
 * `rethumb` は、基準の動画の代表サムネイルを指定・解除した応答を同じように写し、
 * 並びのサムネイル（URL の版が変わる）を画面のサムネイルとそろえる。
 */
export function useRelatedVideos(id: number): {
  state: RelatedState;
  retry: () => void;
  rename: (video: Video) => void;
  rethumb: (video: Video) => void;
} {
  const [state, setState] = useState<RelatedState>({ kind: "loading", id });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    setState({ kind: "loading", id });
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "ready", id, related: { items: [] } });
      return;
    }
    const controller = new AbortController();
    void (async () => {
      try {
        const related = await getRelatedVideos(id, controller.signal);
        setState({ kind: "ready", id, related });
      } catch (failure) {
        if (isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          setState({ kind: "ready", id, related: { items: [] } });
          return;
        }
        setState({ kind: "failed", id });
      }
    })();
    return () => controller.abort();
  }, [attempt, id]);

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
  return { state, retry, rename, rethumb };
}
