import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import {
  beaconProgress,
  errorMessage,
  getVideo,
  isAborted,
  saveProgress,
  streamUrl,
  type Video,
} from "../api/client";
import { formatDuration, unplayableText } from "../components/VideoCard";

/** saveInterval は再生中に位置を送る間隔である（R-111）。 */
const saveIntervalMs = 5000;

/**
 * minResumeMs はこれ未満の位置を「見ていない」とみなす下限である。
 * サーバー側の domain.MinResumeMs と同じ値で、続きから始めるかの判断に使う。
 */
const minResumeMs = 5000;

type State =
  | { kind: "loading" }
  | { kind: "ready"; video: Video }
  | { kind: "failed"; reason: string };

/**
 * VideoPage は再生画面である（FR-016〜FR-020）。
 *
 * 中断位置から再開しつつ、先頭から見直す選択肢も出す。見終わった動画を
 * 末尾から再開させても利用者にできることが無いので、その判断はサーバー側の
 * completed に従う。
 */
export default function VideoPage() {
  const params = useParams();
  const id = Number(params.id);

  const [state, setState] = useState<State>({ kind: "loading" });
  const [resumedFrom, setResumedFrom] = useState<number | null>(null);
  const [playbackError, setPlaybackError] = useState<string | null>(null);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 最後に送った位置。同じ値を送り続けないようにする。
  const lastSent = useRef<number>(-1);

  useEffect(() => {
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "failed", reason: "動画の指定が正しくありません" });
      return;
    }

    const controller = new AbortController();
    void (async () => {
      try {
        setState({ kind: "ready", video: await getVideo(id, controller.signal) });
      } catch (failure) {
        if (isAborted(failure)) {
          return;
        }
        setState({ kind: "failed", reason: errorMessage(failure) });
      }
    })();

    return () => controller.abort();
  }, [id]);

  /** send は現在の位置を送る。 */
  const send = useCallback(
    (positionMs: number, leaving: boolean) => {
      if (!Number.isFinite(positionMs) || positionMs < 0) {
        return;
      }
      const rounded = Math.round(positionMs);
      if (!leaving && Math.abs(rounded - lastSent.current) < 1000) {
        return;
      }
      lastSent.current = rounded;

      if (leaving) {
        beaconProgress(id, rounded);
        return;
      }
      void saveProgress(id, rounded).catch(() => {
        // 記録に失敗しても再生は続ける。次の送信で追いつく。
      });
    },
    [id],
  );

  // 再生中は 5 秒ごとに送る。1 秒ごとに送ると書き込みが増えすぎ、
  // 30 秒だと失う視聴時間が大きくなる（R-111）。
  useEffect(() => {
    const timer = setInterval(() => {
      const element = videoRef.current;
      if (element !== null && !element.paused && !element.ended) {
        send(element.currentTime * 1000, false);
      }
    }, saveIntervalMs);

    return () => clearInterval(timer);
  }, [send]);

  // 画面を離れるときに送る。タブを閉じる・別のタブへ移る・戻る操作の
  // いずれでも、最後の位置を取りこぼさないようにする。
  useEffect(() => {
    const onHidden = () => {
      const element = videoRef.current;
      if (document.visibilityState === "hidden" && element !== null) {
        send(element.currentTime * 1000, true);
      }
    };

    document.addEventListener("visibilitychange", onHidden);
    return () => {
      document.removeEventListener("visibilitychange", onHidden);
      const element = videoRef.current;
      if (element !== null) {
        send(element.currentTime * 1000, true);
      }
    };
  }, [send]);

  /** onLoaded は中断位置へ飛ぶ。 */
  const onLoaded = useCallback(() => {
    const element = videoRef.current;
    if (element === null || state.kind !== "ready") {
      return;
    }

    const progress = state.video.progress;
    if (
      progress === undefined ||
      progress.completed ||
      progress.positionMs < minResumeMs
    ) {
      return;
    }

    element.currentTime = progress.positionMs / 1000;
    setResumedFrom(progress.positionMs);
  }, [state]);

  /** restart は先頭から見直す。 */
  const restart = useCallback(() => {
    const element = videoRef.current;
    if (element !== null) {
      element.currentTime = 0;
      void element.play().catch(() => undefined);
    }
    setResumedFrom(null);
  }, []);

  /**
   * onError は再生できなかったことを伝える。元のファイルが失われた場合でも、
   * 画面ごと壊れずにアプリケーションを使い続けられるようにする。
   */
  const onError = useCallback(() => {
    setPlaybackError(
      "この動画を再生できませんでした。ファイルが移動・削除されたか、" +
        "ブラウザが対応していない形式の可能性があります。",
    );
  }, []);

  return (
    <main className="mx-auto flex min-h-dvh max-w-5xl flex-col gap-4 p-6">
      <Link to="/" className="text-sm text-sky-700 hover:underline">
        ← 一覧へ戻る
      </Link>

      {state.kind === "loading" && <p className="text-neutral-600">読み込み中…</p>}

      {state.kind === "failed" && (
        <p className="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-800">
          動画を開けません: {state.reason}
        </p>
      )}

      {state.kind === "ready" && (
        <>
          <h1 className="text-xl font-semibold tracking-tight">{state.video.title}</h1>

          <Unplayable video={state.video} />

          {playbackError !== null && (
            <p className="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-800">
              {playbackError}
            </p>
          )}

          <video
            ref={videoRef}
            src={streamUrl(state.video.id)}
            controls
            preload="metadata"
            onLoadedMetadata={onLoaded}
            onPause={() => {
              const element = videoRef.current;
              if (element !== null) {
                send(element.currentTime * 1000, false);
              }
            }}
            onEnded={() => {
              const element = videoRef.current;
              if (element !== null) {
                send(element.currentTime * 1000, false);
              }
            }}
            onError={onError}
            className="w-full rounded-lg bg-black"
          />

          {resumedFrom !== null && (
            <p className="flex flex-wrap items-center gap-3 text-sm text-neutral-700">
              <span>{formatDuration(resumedFrom)} から再開しました。</span>
              <button
                type="button"
                onClick={restart}
                className="rounded border border-neutral-300 px-2.5 py-1 hover:bg-neutral-100"
              >
                先頭から見直す
              </button>
            </p>
          )}

          <VideoFacts video={state.video} />
        </>
      )}
    </main>
  );
}

/**
 * Unplayable は再生できない形式であることを、再生を試みる前に示す（FR-020）。
 */
function Unplayable({ video }: { video: Video }) {
  const reason = unplayableText(video);
  if (reason === null) {
    return null;
  }

  return (
    <p className="rounded border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
      {reason}。ファイルは取得できますが、ブラウザでそのまま再生できない可能性があります。
      {video.probeError !== undefined && ` (${video.probeError})`}
    </p>
  );
}

/** VideoFacts は題名・長さ・解像度などを同じ画面に出す（FR-021）。 */
function VideoFacts({ video }: { video: Video }) {
  const facts: [string, string][] = [];

  const duration = formatDuration(video.durationMs);
  if (duration !== "") {
    facts.push(["長さ", duration]);
  }
  if (video.width !== undefined && video.height !== undefined) {
    facts.push(["解像度", `${String(video.width)} × ${String(video.height)}`]);
  }
  if (video.container !== undefined) {
    facts.push(["形式", video.container]);
  }
  if (video.videoCodec !== undefined) {
    facts.push(["映像", video.videoCodec]);
  }
  facts.push(["音声", video.audioCodec ?? "なし"]);
  facts.push(["大きさ", formatSize(video.sizeBytes)]);

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm text-neutral-700">
      {facts.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-neutral-500">{label}</dt>
          <dd className="font-mono">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** formatSize はバイト数を読める大きさにする。 */
function formatSize(bytes: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit] ?? "B"}`;
}
