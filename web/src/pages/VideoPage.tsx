import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useParams } from "react-router";

import {
  beaconProgress,
  errorMessage,
  getVideo,
  isAborted,
  saveProgress,
  streamUrl,
  type Video,
} from "../api/client";
import InfoPanel from "../components/InfoPanel";
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
 * （FR-017）。
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
 * VideoPage は再生画面である（FR-015〜FR-017）。
 *
 * 中断位置から再開しつつ、先頭から見直す選択肢も出す。見終わった動画を
 * 末尾から再開させても利用者にできることが無いので、その判断はサーバー側の
 * completed に従う。
 *
 * **構図は 2 分割である**（contracts/layout.md 4.）。左に映像、右に情報パネル
 * （C14）を置き、一覧と同じ 640px の境界（`sm:`）でパネルを映像の下へ畳む。
 * 境界を一覧と別にすると、狭くしていく途中で構図の変わる点が 2 つになる
 * （R-505）。
 *
 * 004 の縦 5 段（戻る道 → 題名 → 知らせ → 映像 → 情報欄）は、**題名・知らせ・
 * 情報をパネルへ移し、戻る道を × に替える**ことで置き換わった。知らせを映像の
 * 上に積まないので、知らせが増えても**映像の大きさは変わらない**（spec US5-6 /
 * US4-6）。× を映像に重ねないのは FR-017 である。
 *
 * サイドバーもヘッダーも被せない。分岐は `App.tsx` の 1 か所にあり、この画面は
 * 骨格の存在を知らない（FR-015 / R-505）。
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

  const ready = state.kind === "ready" ? state.video : undefined;

  return (
    // 2 分割の器（contracts/layout.md 4.）。
    //
    // 幅 640px 以上で左に映像・右にパネル、未満では 1 列に畳む。パネルの幅を
    // `minmax(18rem, 24rem)` にするのは、固定幅だと幅 640〜800px で映像が
    // 極端に細くなるからである（R-505）。映像の側は `minmax(0, 1fr)` で、
    // 0 を下限にしないと中身（映像）の既定幅が下限になって器からはみ出す。
    //
    // 余白は狭い画面で詰める。幅 360px では p-6（左右で 48px）が中身の 13% を
    // 占め、映像もパネルもその分だけ狭くなる。
    //
    // `items-start` で、パネルを映像の高さに引き伸ばさない。
    <main className="mx-auto grid min-h-dvh w-full max-w-7xl items-start gap-4 p-4 sm:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)] sm:gap-6 sm:p-6">
      {/* 左（狭い画面では上）: 映像だけを置く。題名も知らせもここには無い。 */}
      <div className="min-w-0">
        {state.kind === "loading" && (
          // 骨組みは読み上げに渡さない。伝えたいのは「この領域はいま読み込み中
          // である」という 1 つの事実である（contracts/screen-states.md 3.）。
          <div role="status" aria-label="読み込み中">
            <Skeleton />
          </div>
        )}

        {/* 取得に失敗したら映像は出さない。取得できていないのだから、再生の
            入口を出しても押せることは無い。理由はパネルの中に出る。 */}

        {ready !== undefined && (
          // 地は surface-sunken で、パネルを除いた領域いっぱいに広がる
          // （spec US5-2）。操作列はブラウザ標準に任せる。
          //
          // 比率は aspect-video で決め打たない。16:9 以外（縦長の動画など）で
          // 必ず見切れるか余白が出る。<video> は自分の比率を知っているので、
          // 高さを指定しなければ幅から比率どおりの高さが決まる。
          //
          // max-h-[70dvh] は縦長の動画が画面の高さを超え、操作列ごと画面外へ
          // 出るのを防ぐ。高さで頭打ちになった分は地（surface-sunken）が
          // レターボックスとして見える（contracts/design-tokens.md 2.）。
          <video
            ref={videoRef}
            src={streamUrl(ready.id)}
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
            className="h-auto max-h-[70dvh] w-full rounded-card bg-surface-sunken"
          />
        )}
      </div>

      {/* 右（狭い画面では下）: 情報パネル。× → 題名 → 知らせ → 情報の順は
          InfoPanel が持つ。**どの状態でも描く** — 取得に失敗した画面から
          戻れないと、利用者に残る手が再読み込みしかなくなる（FR-017）。 */}
      <InfoPanel backTo={backTo} title={ready?.title} video={ready}>
        {state.kind === "failed" && (
          <StateNotice
            tone="danger"
            title="動画を開けません"
            description={state.reason}
          />
        )}

        {ready !== undefined && <Unplayable video={ready} />}

        {playbackError !== null && (
          <StateNotice tone="danger" title="再生できません" description={playbackError} />
        )}

        {resumedFrom !== null && (
          <StateNotice
            tone="info"
            title={`${formatDuration(resumedFrom)} から再開しました`}
          >
            <button
              type="button"
              onClick={restart}
              className="min-h-[var(--size-tap)] min-w-[var(--size-tap)] rounded-control border border-border px-3 text-sm outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus"
            >
              先頭から見直す
            </button>
          </StateNotice>
        )}
      </InfoPanel>
    </main>
  );
}

/**
 * Unplayable は再生できない形式であることを、**再生を試みる前に**示す
 * （spec US5-6 の知らせのうちの 1 つ）。
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
