import { AlertCircle, ArrowLeft } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation, useParams } from "react-router";

import {
  beaconProgress,
  errorMessage,
  getVideo,
  isAborted,
  saveProgress,
  type Video,
} from "../api/client";
import { formatDuration } from "../lib/format";
import { buttonClassName } from "../ui/Button";
import Skeleton from "../ui/Skeleton";
import FileDetails from "./FileDetails";
import VideoHeader from "./VideoHeader";
import VideoPlayer, { canStartPlayback } from "./VideoPlayer";

/** minResumeMs 未満の位置は「見始めたばかり」として先頭から再生する。 */
const minResumeMs = 5000;

type State =
  | { kind: "loading" }
  | { kind: "ready"; video: Video }
  | { kind: "failed"; reason: string };

/** backTarget は遷移元の一覧 URL。無ければ `/`。外部 URL は受け付けない。 */
function backTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !from.startsWith("/") || from.startsWith("//")) {
    return "/";
  }
  return from;
}

export default function VideoPage() {
  const params = useParams();
  const id = Number(params.id);
  const backTo = backTarget(useLocation().state);

  const [state, setState] = useState<State>({ kind: "loading" });
  const [playbackError, setPlaybackError] = useState<string | null>(null);
  const [resumedFrom, setResumedFrom] = useState<number | null>(null);

  const lastSent = useRef<{ videoId: number; positionMs: number } | null>(null);
  const latestPosition = useRef<{ videoId: number; positionMs: number } | null>(null);

  useEffect(() => {
    setState({ kind: "loading" });
    setPlaybackError(null);
    setResumedFrom(null);
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "failed", reason: "動画の指定が正しくありません" });
      return;
    }
    const controller = new AbortController();
    void (async () => {
      try {
        setState({ kind: "ready", video: await getVideo(id, controller.signal) });
      } catch (failure) {
        if (isAborted(failure)) return;
        setState({ kind: "failed", reason: errorMessage(failure) });
      }
    })();
    return () => controller.abort();
  }, [id]);

  const video = state.kind === "ready" && state.video.id === id ? state.video : undefined;

  useEffect(() => {
    const previous = document.title;
    if (video !== undefined) document.title = `${video.title} - vv`;
    return () => {
      document.title = previous;
    };
  }, [video]);

  const send = useCallback(
    (positionMs: number, leaving: boolean, force = false) => {
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
    [id],
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

  const onError = useCallback(() => {
    setPlaybackError(
      "この動画を再生できませんでした。ファイルが移動・削除されたか、ブラウザが対応していない形式の可能性があります。",
    );
  }, []);

  const initialPositionMs =
    video?.progress === undefined ||
    video.progress.completed ||
    video.progress.positionMs < minResumeMs
      ? 0
      : video.progress.positionMs;

  return (
    <div className="flex min-h-dvh flex-col bg-bg">
      <header className="flex h-navbar shrink-0 items-center border-b border-border bg-bg px-2 sm:px-3">
        <Link
          to={backTo}
          className="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm text-fg transition-colors hover:bg-hover-wash"
        >
          <ArrowLeft className="size-4" />
          ライブラリ
        </Link>
      </header>

      {/* プレイヤー領域。幅いっぱい、高さは画面に収まる範囲で 16:9。 */}
      <div className="flex w-full justify-center bg-navbar">
        <div className="relative aspect-video w-full max-w-[calc((100dvh-12rem)*16/9)] overflow-hidden bg-navbar">
          {state.kind === "loading" && (
            <Skeleton className="absolute inset-0 rounded-none" />
          )}

          {state.kind === "failed" && (
            <Blocked
              title="動画を開けません"
              description={state.reason}
              backTo={backTo}
            />
          )}

          {video !== undefined && !canStartPlayback(video) && (
            <Blocked
              title="この動画は再生できません"
              description="再生に必要な動画情報を取得できませんでした。"
              backTo={backTo}
            />
          )}

          {video !== undefined && canStartPlayback(video) && (
            <VideoPlayer
              video={video}
              initialPositionMs={initialPositionMs}
              onPosition={rememberProgress}
              onProgress={savePlayerProgress}
              onResumed={setResumedFrom}
              onError={onError}
            />
          )}
        </div>
      </div>

      <div className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-5 sm:px-6">
        {playbackError !== null && (
          <div
            role="alert"
            className="flex items-start gap-3 rounded-md border border-danger-strong px-4 py-3 text-sm text-fg animate-fade-in"
          >
            <AlertCircle className="mt-0.5 size-4 shrink-0" />
            {playbackError}
          </div>
        )}

        {state.kind === "loading" && (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-7 w-2/3" />
            <Skeleton className="h-5 w-1/2" />
          </div>
        )}

        {video !== undefined && (
          <>
            <VideoHeader
              video={video}
              resumedFrom={resumedFrom === null ? null : formatDuration(resumedFrom)}
            />
            <FileDetails video={video} />
          </>
        )}
      </div>
    </div>
  );
}

function Blocked({
  title,
  description,
  backTo,
}: {
  title: string;
  description: string;
  backTo: string;
}) {
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-6 text-center animate-fade-in">
      <AlertCircle className="size-8 text-warning" />
      <h2 className="text-lg font-semibold tracking-tight text-fg">{title}</h2>
      <p className="max-w-md text-sm text-fg-muted text-balance">{description}</p>
      <Link to={backTo} className={buttonClassName("secondary", "md", "mt-2")}>
        ライブラリへ戻る
      </Link>
    </div>
  );
}
