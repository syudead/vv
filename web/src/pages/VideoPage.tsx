import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation, useParams } from "react-router";

import {
  beaconProgress,
  errorMessage,
  getVideo,
  isAborted,
  saveProgress,
  streamUrl,
  type Video,
} from "../api/client";
import Skeleton from "../components/Skeleton";
import StateNotice from "../components/StateNotice";
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
 * backTarget は「一覧へ戻る」の行き先を、遷移元から渡された state で決める
 * （FR-016）。
 *
 * 検索語と並び順は一覧の URL のクエリにしかないので、`/` へ戻すと絞り込みも
 * 並び順も消え、復元の控え（鍵が `q` と `sort` でできている）とも一致しない。
 * 直接 `/videos/:id` を開いた場合は遷移元が無いので `/` に落とす。
 *
 * 受けるのはアプリケーション内の絶対パスだけである。state は履歴に残る値で、
 * 手を加えられうるものを行き先にしたくない。
 */
function backTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !from.startsWith("/") || from.startsWith("//")) {
    return "/";
  }
  return from;
}

/**
 * VideoPage は再生画面である（FR-012〜FR-016）。
 *
 * 中断位置から再開しつつ、先頭から見直す選択肢も出す。見終わった動画を
 * 末尾から再開させても利用者にできることが無いので、その判断はサーバー側の
 * completed に従う。
 *
 * 並びは contracts/screen-states.md 2.「並び」の 5 段に固定する。
 *
 * 1. 一覧へ戻る（**どの状態でも先に出す**。通信中も失敗中も戻れる。FR-016）
 * 2. 題名
 * 3. 知らせの置き場（再生できない形式・再生の失敗・続きから始まった知らせ）
 * 4. 映像
 * 5. 情報欄
 *
 * 3 が映像より**上**にあるのが要点である。映像の上に重ねると映像を隠して
 * FR-014 に反し、情報欄の下に置くと気付かれない。
 */
export default function VideoPage() {
  const params = useParams();
  const id = Number(params.id);
  const backTo = backTarget(useLocation().state);

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
      {/* 1. 一覧へ戻る。状態によらず**先に**出す（FR-016 /
          contracts/screen-states.md 2.・3.）。取得に失敗した画面から
          戻れないと、利用者に残る手が再読み込みしかなくなる。 */}
      <Link
        to={backTo}
        className="inline-flex min-h-[var(--size-tap)] w-fit items-center rounded-control text-sm text-accent outline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-focus"
      >
        ← 一覧へ戻る
      </Link>

      {state.kind === "loading" && (
        // 骨組みは読み上げに渡さない。伝えたいのは「この領域はいま読み込み中
        // である」という 1 つの事実である（contracts/screen-states.md 3.）。
        <div role="status" aria-label="読み込み中">
          <Skeleton />
        </div>
      )}

      {state.kind === "failed" && (
        // 映像は出さない。取得できていないのだから、再生の入口を出しても
        // 押せることは無い（contracts/screen-states.md 2.）。
        <StateNotice tone="danger" title="動画を開けません" description={state.reason} />
      )}

      {state.kind === "ready" && (
        <>
          {/* 2. 題名 */}
          <h1 className="text-xl font-semibold tracking-tight">{state.video.title}</h1>

          {/* 3. 知らせの置き場。映像の**外**で、映像より上に置く。 */}
          <Unplayable video={state.video} />

          {playbackError !== null && (
            <StateNotice
              tone="danger"
              title="再生できません"
              description={playbackError}
            />
          )}

          {resumedFrom !== null && (
            <StateNotice
              tone="info"
              title={`${formatDuration(resumedFrom)} から再開しました`}
            >
              <button
                type="button"
                onClick={restart}
                className="min-h-[var(--size-tap)] rounded-control border border-border px-3 text-sm outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus"
              >
                先頭から見直す
              </button>
            </StateNotice>
          )}

          {/* 4. 映像。地は surface-sunken で、比率を保ったまま画面幅に収まる
              （FR-012）。操作盤はブラウザ標準に任せる。 */}
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
            className="w-full rounded-card bg-surface-sunken"
          />

          {/* 5. 情報欄 */}
          <VideoFacts video={state.video} />
        </>
      )}
    </main>
  );
}

/**
 * Unplayable は再生できない形式であることを、**再生を試みる前に**示す（FR-015）。
 */
function Unplayable({ video }: { video: Video }) {
  const reason = unplayableText(video);
  if (reason === null) {
    return null;
  }

  return (
    <StateNotice
      tone="warning"
      title={`${reason}。ファイルは取得できますが、ブラウザでそのまま再生できない可能性があります。`}
      description={video.probeError}
    />
  );
}

/**
 * missing は取れていない値の見せ方を決める（data-model.md 3.「取れていない値
 * の扱い」/ FR-013）。
 *
 * **空欄にしない。** 空欄は「値が無い」のか「まだ調べていない」のか「調べたが
 * 読めなかった」のかを区別できず、利用者は待てばよいのか諦めるのかを判断
 * できない。
 */
function missing(video: Video): string {
  if (video.probeState === "pending") {
    return "確認中";
  }
  if (video.probeState === "failed") {
    return video.probeError === undefined
      ? "読み取れませんでした"
      : `読み取れませんでした (${video.probeError})`;
  }
  // 解析は済んでいて値が無い。音声の無い動画のように、無いこと自体が情報である。
  return "なし";
}

/** VideoFacts は題名・長さ・解像度などを同じ画面に出す（FR-013）。 */
function VideoFacts({ video }: { video: Video }) {
  const fallback = missing(video);

  /** value は取れていれば値を、取れていなければ言い分けを返す。 */
  const value = (text: string | undefined): string =>
    text === undefined || text === "" ? fallback : text;

  const resolution =
    video.width === undefined || video.height === undefined
      ? undefined
      : `${String(video.width)} × ${String(video.height)}`;

  // sizeBytes は必須の項目なので、この言い分けに入らない。
  const facts: [string, string][] = [
    ["長さ", value(formatDuration(video.durationMs))],
    ["解像度", value(resolution)],
    ["形式", value(video.container)],
    ["映像", value(video.videoCodec)],
    ["音声", value(video.audioCodec)],
    ["大きさ", formatSize(video.sizeBytes)],
  ];

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm text-body">
      {facts.map(([label, text]) => (
        <div key={label} className="contents">
          <dt className="text-muted">{label}</dt>
          <dd className="font-mono">{text}</dd>
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
