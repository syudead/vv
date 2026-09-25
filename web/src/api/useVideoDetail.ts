import { useCallback, useEffect, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import {
  errorMessage,
  getRelatedVideos,
  getVideo,
  isAborted,
  RequestFailed,
  type RelatedVideos,
  type Video,
} from "./client";
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
  | { kind: "failed"; id: number; reason: string };

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
 */
export function useVideoDetail(id: number): {
  state: VideoDetailState;
  refresh: () => Promise<void>;
} {
  const [state, setState] = useState<VideoDetailState>({ kind: "loading", id });
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  // 変化の知らせ（/api/events）は所有者だけのものなので、ゲストでは購読しない。
  const owner = useAudience() === "owner";

  useEffect(() => {
    setState({ kind: "loading", id });
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "missing", id });
      refreshRef.current = () => Promise.resolve();
      return;
    }

    let alive = true;
    let controller: AbortController | null = null;
    let current: Video | undefined;
    const waiters: (() => void)[] = [];

    const settle = () => {
      for (const resolve of waiters.splice(0)) resolve();
    };

    const load = async () => {
      controller?.abort();
      const mine = new AbortController();
      controller = mine;
      // 取得の間に公開を切り替えたら、切り替える前の `public` を読んだ応答で
      // 表示を巻き戻さない（切り替えはサーバーから知らせが来ない。PR 328）。
      const mark = visibilityMark();
      try {
        const video = withVisibilitySince(await getVideo(id, mine.signal), mark);
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
          setState({ kind: "failed", id, reason: errorMessage(failure) });
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
      settle();
    };
  }, [id, owner]);

  const refresh = useCallback(() => refreshRef.current(), []);
  return { state, refresh };
}

export type RelatedState =
  | { kind: "loading"; id: number }
  | { kind: "ready"; id: number; related: RelatedVideos }
  | { kind: "failed"; id: number };

/** useRelatedVideos は関連動画を 1 回取得する。失敗したら `retry` で取り直せる。 */
export function useRelatedVideos(id: number): {
  state: RelatedState;
  retry: () => void;
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
  return { state, retry };
}
