import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useLocation, useNavigate, useParams } from "react-router";

import { getAuthSession } from "../api/auth";
import {
  beaconProgress,
  reloadForViewerChange,
  reprobeVideo,
  RequestFailed,
  saveProgress,
  type Video,
} from "../api/client";
import { useRelatedVideos, useVideoDetail } from "../api/useVideoDetail";
import { useAudience } from "../auth/audience";
import Skeleton from "../ui/Skeleton";
import CloseButton from "./CloseButton";
import EndedOverlay from "./EndedOverlay";
import FileLocation from "./FileLocation";
import { useKeyboardShortcuts } from "./keyboard";
import type { PlayerControls } from "./playerControls";
import PropertyStrip from "./PropertyStrip";
import RelatedVideos from "./RelatedVideos";
import {
  CreatingLine,
  LoadFailure,
  LoadingOverlay,
  MissingVideo,
  PlaybackFailure,
  ProcessingStages,
  ReadFailure,
  Unplayable,
} from "./StatusOverlays";
import TouchControls from "./TouchControls";
import VideoPlayer, {
  canStartPlayback,
  initialPlayerStatus,
  type PlayerStatus,
} from "./VideoPlayer";
import VideoTags from "./VideoTags";

/** minResumeMs 未満の位置は「見始めたばかり」として先頭から再生する。 */
const minResumeMs = 5000;

/** backTarget は遷移元の一覧 URL。無ければ `/`。外部 URL は受け付けない。 */
export function backTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !from.startsWith("/") || from.startsWith("//")) {
    return "/";
  }
  return from;
}

/** autoplayRequested は「次を再生」から来たか（移った先で再生を始めるか）を返す。 */
function autoplayRequested(state: unknown): boolean {
  return (state as { autoplay?: unknown } | null)?.autoplay === true;
}

function resumePosition(video: Video): number {
  const progress = video.progress;
  return progress === undefined || progress.completed || progress.positionMs < minResumeMs
    ? 0
    : progress.positionMs;
}

/** 再生の試み。再試行と「次を再生」は、位置と自動再生を決めてプレイヤーを作り直す。 */
interface Attempt {
  key: number;
  startMs: number | null;
  autoplay: boolean;
}

/**
 * VideoPage は動画詳細画面（`/videos/:id`）である。
 *
 * 構成要素はプレイヤー・題名・属性情報（ファイルの場所を含む）・タグ・関連動画の 5 つと
 * する（親 Issue 要件 1・3、issue 268）。状態と失敗は、プレイヤーの上の 1 つの入れ物に重ねて伝える
 * （plan の Structural Decisions 12）。入れ物の中は、上から 状態表示・再生終了・タッチ用の
 * 中央操作 の順で、同時に出すのは 1 つだけである。
 */
export default function VideoPage() {
  const params = useParams();
  // 形の正しくない id は 0 に寄せる（NaN は自分自身と等しくならず、比べられない）。
  const parsedId = Number(params.id);
  const id = Number.isSafeInteger(parsedId) && parsedId > 0 ? parsedId : 0;
  const location = useLocation();
  const navigate = useNavigate();
  const backTo = backTarget(location.state);
  // ゲストには所有者のデータ（タグ・ファイルの場所・再生位置）と所有者だけの操作
  // （読み取りのやり直し・既定アプリで開く）を出さない
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const audience = useAudience();
  const owner = audience === "owner";

  const { state: detailState, refresh } = useVideoDetail(id);
  const { state: relatedState, retry: retryRelated } = useRelatedVideos(id);
  const detail = detailState.id === id ? detailState : { kind: "loading" as const, id };
  const related =
    relatedState.id === id ? relatedState : { kind: "loading" as const, id };
  const video = detail.kind === "ready" ? detail.video : undefined;

  // 一覧をスクロールした位置から来ても、プレイヤーを画面の上に出す。別の動画へ移ったときも
  // 同じ。一覧へ戻ったときの位置の復元は一覧の側（LibraryPage）が行う。広い画面では左右の列が
  // それぞれスクロールするので、左の列も先頭へ戻す。
  const mainColumnRef = useRef<HTMLDivElement | null>(null);
  useLayoutEffect(() => {
    window.scrollTo(0, 0);
    mainColumnRef.current?.scrollTo?.(0, 0);
  }, [id]);

  const [pageId, setPageId] = useState(id);
  const [controls, setControls] = useState<PlayerControls | null>(null);
  const [status, setStatus] = useState<PlayerStatus>(initialPlayerStatus);
  const [failure, setFailure] = useState<{ positionMs: number } | null>(null);
  const [attempt, setAttempt] = useState<Attempt>(() => ({
    key: 0,
    startMs: null,
    autoplay: autoplayRequested(location.state),
  }));
  const [endedTakesFocus, setEndedTakesFocus] = useState(false);
  const frameRef = useRef<HTMLDivElement | null>(null);
  // 全画面はプレイヤーの上の層ごとにする（状態表示・再生終了・中央操作を全画面でも出す）。
  const fullscreenTarget = useCallback(() => frameRef.current, []);

  // 別の動画へ移ったら、前の動画の再生の状態を持ち越さない。
  if (pageId !== id) {
    setPageId(id);
    setFailure(null);
    setStatus(initialPlayerStatus);
    setEndedTakesFocus(false);
    setAttempt({ key: 0, startMs: null, autoplay: autoplayRequested(location.state) });
  }

  // 「次を再生」の自動再生は 1 回だけ使う。再読み込みや履歴の戻りで再生を始めない。
  useEffect(() => {
    if (!autoplayRequested(location.state)) return;
    void navigate(
      { pathname: location.pathname, search: location.search },
      { replace: true, state: { from: backTo } },
    );
  }, [backTo, location.pathname, location.search, location.state, navigate]);

  const close = useCallback(() => void navigate(backTo), [backTo, navigate]);

  const title = video?.title;
  useEffect(() => {
    const previous = document.title;
    if (title !== undefined) document.title = `${title} - vv`;
    return () => {
      document.title = previous;
    };
  }, [title]);

  // --- 再生位置の保存（既存どおり） ---
  const lastSent = useRef<{ videoId: number; positionMs: number } | null>(null);
  const latestPosition = useRef<{ videoId: number; positionMs: number } | null>(null);

  const send = useCallback(
    (positionMs: number, leaving: boolean, force = false) => {
      // 再生位置は所有者のものなので、ゲストでは保存を送らない。
      if (!owner) return;
      if (!Number.isFinite(positionMs) || positionMs < 0) return;
      const rounded = Math.round(positionMs);
      if (
        !leaving &&
        !force &&
        lastSent.current?.videoId === id &&
        Math.abs(rounded - lastSent.current.positionMs) < 1000
      ) {
        return;
      }
      lastSent.current = { videoId: id, positionMs: rounded };
      if (leaving) {
        beaconProgress(id, rounded);
        return;
      }
      void saveProgress(id, rounded).catch(() => undefined);
    },
    [id, owner],
  );

  const rememberProgress = useCallback(
    (positionMs: number) => {
      latestPosition.current = { videoId: id, positionMs };
    },
    [id],
  );

  const savePlayerProgress = useCallback(
    (positionMs: number, immediate: boolean) => {
      latestPosition.current = { videoId: id, positionMs };
      send(positionMs, false, immediate);
    },
    [id, send],
  );

  useEffect(() => {
    const sendLatest = () => {
      const latest = latestPosition.current;
      if (latest?.videoId === id) send(latest.positionMs, true);
    };
    const onHidden = () => {
      if (document.visibilityState === "hidden") sendLatest();
    };
    document.addEventListener("visibilitychange", onHidden);
    window.addEventListener("pagehide", sendLatest);
    return () => {
      document.removeEventListener("visibilitychange", onHidden);
      window.removeEventListener("pagehide", sendLatest);
      sendLatest();
    };
  }, [id, send]);

  // --- プレイヤーの状態 ---
  const endedRef = useRef(false);
  const onStatus = useCallback((next: PlayerStatus) => {
    // 再生終了の層へフォーカスを移すのは、フォーカスがプレイヤーの中にあったときだけにする。
    if (next.ended && !endedRef.current) {
      const frame = frameRef.current;
      setEndedTakesFocus(frame !== null && frame.contains(document.activeElement));
    }
    endedRef.current = next.ended;
    setStatus(next);
  }, []);

  // 再生の失敗は、見る人が変わった（セッションが失効した）せいかもしれない。`video`
  // 要素の読み込みの失敗には 401 も X-VV-Audience も付かないので、状態を確かめ、
  // 変わっていれば失敗の層を出さずにページを1度だけ読み直す（ui-design.md
  // 「Guest degradation」）。変わっていなければ、今の再生失敗の層を出す。
  // 同じ route で別の動画へ移るとこの部品は使い回されるので、確かめは動画が変わったときにも
  // 止め、返ってきたときに始めた動画のままのときだけ結果を使う。前の動画の失敗を次の動画に
  // 出さないためである。
  const sessionCheck = useRef<AbortController | null>(null);
  const currentId = useRef(id);
  currentId.current = id;
  useEffect(() => () => sessionCheck.current?.abort(), [id]);
  const onError = useCallback(
    (positionMs: number) => {
      sessionCheck.current?.abort();
      const controller = new AbortController();
      sessionCheck.current = controller;
      const checkedId = id;
      const stale = () => controller.signal.aborted || currentId.current !== checkedId;
      const showFailure = () => {
        setFailure({ positionMs });
        // 動画がライブラリから消えた（ゲストでは公開でなくなった）せいかもしれない。
        // 取り直して確かめる。
        void refresh();
      };
      getAuthSession(undefined, controller.signal).then(
        (session) => {
          if (stale()) return;
          if (session.state !== audience) {
            reloadForViewerChange();
            return;
          }
          showFailure();
        },
        () => {
          if (stale()) return;
          showFailure();
        },
      );
    },
    [audience, id, refresh],
  );

  const retryPlayback = () => {
    const startMs = failure?.positionMs ?? 0;
    setFailure(null);
    setStatus(initialPlayerStatus);
    setAttempt((previous) => ({ key: previous.key + 1, startMs, autoplay: true }));
  };

  const reprobe = useCallback(async () => {
    try {
      await reprobeVideo(id);
    } catch (error) {
      // 409 は、別のタブや連打ですでにやり直しが始まっている。
      if (!(error instanceof RequestFailed && error.code === "probe_not_failed"))
        throw error;
    }
    // 202 の本文は所在を持たないので、置き換えずに取り直す。
    await refresh();
  }, [id, refresh]);

  const nextVideo =
    related.kind === "ready" && related.related.nextId !== undefined
      ? related.related.items.find((item) => item.id === related.related.nextId)
      : undefined;

  const playNext = () => {
    if (nextVideo === undefined) return;
    void navigate(`/videos/${String(nextVideo.id)}`, {
      state: { from: backTo, autoplay: true },
    });
  };

  // --- プレイヤーの上に重ねる層（同時に 1 つだけ） ---
  const playable =
    video !== undefined && video.probeState === "done" && canStartPlayback(video);
  let statusLayer: ReactNode = null;
  if (detail.kind === "missing") statusLayer = <MissingVideo />;
  else if (detail.kind === "failed")
    statusLayer = <LoadFailure reason={detail.reason} onRetry={refresh} />;
  else if (video === undefined) statusLayer = <LoadingOverlay backdrop />;
  else if (video.probeState === "pending")
    statusLayer = <ProcessingStages video={video} />;
  else if (video.probeState === "failed")
    statusLayer = <ReadFailure video={video} onReprobe={owner ? reprobe : undefined} />;
  else if (!playable) statusLayer = <Unplayable />;
  else if (failure !== null)
    statusLayer = (
      <PlaybackFailure positionMs={failure.positionMs} onRetry={retryPlayback} />
    );

  const showPlayer = playable && detail.kind === "ready";
  const chromeVisible = !status.playing || status.userActive;
  let layer: ReactNode = statusLayer;
  if (layer === null && status.ended) {
    layer = (
      <EndedOverlay
        next={nextVideo}
        backTo={backTo}
        takeFocus={endedTakesFocus}
        onReplay={() => controls?.restart()}
        onPlayNext={playNext}
      />
    );
  } else if (layer === null && status.loading) {
    layer = <LoadingOverlay backdrop={false} />;
  } else if (layer === null && controls !== null) {
    layer = (
      <TouchControls
        playing={status.playing}
        visible={chromeVisible}
        onBack={() => {
          controls.seekBy(-10);
          controls.wake();
        }}
        onToggle={() => {
          controls.togglePlay();
          controls.wake();
        }}
        onForward={() => {
          controls.seekBy(10);
          controls.wake();
        }}
      />
    );
  }
  const overlayShown = statusLayer !== null || (showPlayer && status.ended);

  useKeyboardShortcuts(showPlayer ? controls : null, close);

  return (
    // 狭い画面はページ全体を 1 つとしてスクロールする。広い画面はページを画面の高さに留め、
    // 左の列（プレイヤー・題名・属性）と右の列（関連動画）がそれぞれ中でスクロールする。
    // ページと列の両方がスクロールして二重に動くことがないようにするためである。
    <div className="min-h-dvh bg-bg pb-16 lg:h-dvh lg:min-h-0 lg:overflow-hidden lg:pb-0">
      <div className="flex w-full flex-col gap-5 lg:grid lg:h-full lg:grid-cols-[minmax(0,1fr)_20rem] lg:grid-rows-[minmax(0,1fr)] lg:gap-6 lg:px-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <div
          ref={mainColumnRef}
          // 列の端にあるフォーカスの輪郭が切れないよう、はみ出す分だけ内側に余白を取る。
          className="flex min-w-0 flex-col gap-5 lg:-mx-1 lg:min-h-0 lg:overflow-y-auto lg:overscroll-contain lg:px-1 lg:pt-6 lg:pb-16"
        >
          <div
            ref={frameRef}
            data-player-frame=""
            className="relative isolate mx-auto grid w-full grid-cols-[minmax(0,1fr)] max-w-[calc((100dvh-9rem)*16/9)] overflow-hidden bg-navbar lg:rounded-lg [&:fullscreen]:rounded-none"
          >
            {/* 16:9 は下限。状態表示が収まらない幅では、内容に合わせて伸びる。
                全画面では入れ物が画面いっぱいになるので、下限は要らない。 */}
            <div
              aria-hidden="true"
              className="col-start-1 row-start-1 aspect-video [:fullscreen>&]:hidden"
            />
            <CloseButton
              variant="overlay"
              onClose={close}
              visible={chromeVisible || overlayShown}
            />
            <div
              data-overlay-layer=""
              className="pointer-events-none relative z-10 col-start-1 row-start-1 flex min-w-0"
            >
              {layer}
            </div>
            {showPlayer && (
              <VideoPlayer
                key={`${String(id)}:${String(attempt.key)}`}
                video={video}
                initialPositionMs={attempt.startMs ?? resumePosition(video)}
                autoplay={attempt.autoplay}
                onPosition={rememberProgress}
                onProgress={savePlayerProgress}
                onError={onError}
                onControls={setControls}
                onStatus={onStatus}
                fullscreenTarget={fullscreenTarget}
              />
            )}
          </div>

          <div className="flex flex-col gap-5 px-4 sm:px-6 lg:px-0">
            {detail.kind === "loading" && (
              <div className="flex flex-col gap-3">
                <Skeleton className="h-7 w-2/3" />
                <Skeleton className="h-5 w-1/2" />
              </div>
            )}
            {video !== undefined && (
              <>
                <CreatingLine video={video} />
                {/* 題名とタグは1つのまとまり（ui-design.md「Video page tags」Placement）。 */}
                <div className="flex flex-col gap-2">
                  <h1 className="text-xl leading-snug font-semibold text-fg [overflow-wrap:anywhere] sm:text-2xl">
                    {video.title}
                  </h1>
                  {owner && (
                    <VideoTags
                      // VideoPage 自身が動画ごとに作り直されず（同じ /videos/:id
                      // ルートのまま次の動画へ移ることがある）使い回されるため、
                      // VideoTags を videoId で作り直し、前の動画の重ねた
                      // 付け外し（appliedRef）を持ち越さない（Devin の指摘1）。
                      key={video.id}
                      videoId={video.id}
                      tags={video.tags}
                      onStaleVideo={() => void refresh()}
                    />
                  )}
                </div>
                <PropertyStrip video={video} lastPlayed={owner} />
                {owner && video.location !== undefined && (
                  <FileLocation videoId={video.id} location={video.location} />
                )}
              </>
            )}
          </div>
        </div>

        <aside
          // 広い画面では見出しの行を上に留め、関連動画の並びだけを中でスクロールさせる。
          className="min-w-0 px-4 sm:px-6 lg:flex lg:min-h-0 lg:flex-col lg:px-0 lg:pt-6"
        >
          <RelatedVideos
            state={related}
            backTo={backTo}
            onRetry={retryRelated}
            closeButton={<CloseButton variant="wide" onClose={close} />}
          />
        </aside>
      </div>
    </div>
  );
}
