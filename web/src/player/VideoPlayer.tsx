import { Info, RotateCcw, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import videojs from "video.js";
import "video.js/dist/video-js.css";

import { streamUrl, type Video } from "../api/client";
import { t, type UiText } from "../i18n";
import { readPlaybackVolume, writePlaybackVolume } from "../preferences/playbackVolume";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { liveSource } from "./liveOffset";
import {
  createPlaybackAttempt,
  fallbackToTranscode,
  updatePosition,
  type PlaybackAttempt,
  type PlaybackRoute,
} from "./playbackAttempt";
import {
  createPlayerControls,
  type ControllablePlayer,
  type PlayerControls,
  rateMenuOpen,
} from "./playerControls";
import {
  classifyMediaError,
  reconnectDelay,
  recoveredAfterMs,
  serverReachable,
  type PlaybackFailureKind,
} from "./playbackRecovery";
import { attachSeekPreview } from "./seekPreview";

const saveIntervalMs = 5000;
/**
 * maxPlaybackStepMs は続けて届いた timeupdate の間に進んだとみなす上限である。timeupdate は
 * 再生中おおよそ 0.25 秒ごとに届くので、2 倍速でもこれを超えない。超えた差はシークとみなす。
 */
const maxPlaybackStepMs = 2000;

/** playbackRates は操作バーの再生速度の選択肢である（ui-design「Control bar」）。 */
export const playbackRates = [0.5, 0.75, 1, 1.25, 1.5, 2];

/**
 * 操作バーの並び（要件 6）。残り時間は出さず、現在時刻/長さを出す。再生バーは
 * index.css で操作バーの上へ出す。「最初に戻る」は再生の前へ、「変換して再生中」は再生速度の
 * 前へ差し込む。秒数送りは置かない。前後の動画はプレイヤーの左右の端に置く（NeighborArrows）。
 */
const controlBarChildren = [
  "playToggle",
  "volumePanel",
  "currentTimeDisplay",
  "timeDivider",
  "durationDisplay",
  "progressControl",
  "customControlSpacer",
  "playbackRateMenuButton",
  "pictureInPictureToggle",
  "fullscreenToggle",
];

/**
 * 操作バーの読み上げ名とポイントしたときの説明。video.js の文言をカタログの英語で置き換える
 * 独自言語で、キーボード操作を持つボタンにはキーを添える（ui-design「Control bar」、
 * specs/023-english-i18n/research.md R-9）。video.js は同じ文字を title にも使う。
 *
 * 名前は英語の私用の副タグにする。video.js はこれをプレイヤーの `lang` 属性にも使い、
 * ここに無い文言は主の言語（`en`）の既定に戻る。
 */
export const playerLanguage = "en-x-vv";

/** playerDictionary は video.js の文言からカタログの英語への表である。作るたびにカタログを引く。 */
export function playerDictionary(): Record<string, string> {
  const c = t.player.controls;
  return {
    Play: c.withKey(c.play, "Space"),
    Pause: c.withKey(c.pause, "Space"),
    Replay: c.withKey(c.replay, "Space"),
    "Play Video": c.play,
    Mute: c.withKey(c.mute, "M"),
    Unmute: c.withKey(c.unmute, "M"),
    Fullscreen: c.withKey(c.fullscreen, "F"),
    "Exit Fullscreen": c.withKey(c.exitFullscreen, "F"),
    "Picture-in-Picture": c.pictureInPicture,
    "Exit Picture-in-Picture": c.exitPictureInPicture,
    "Playback Rate": c.playbackRate,
    "Current Time": c.currentTime,
    Duration: c.duration,
    "Progress Bar": c.progressBar,
    "Volume Level": c.volumeLevel,
    "Video Player": c.videoPlayer,
  };
}

/** キーボード操作を持つボタンと、そのキー（aria-keyshortcuts）。 */
const keyShortcuts: [string, string][] = [
  [".vjs-play-control", "Space"],
  [".vjs-mute-control", "M"],
  [".vjs-fullscreen-control", "F"],
];

/** PlayerStatus は、プレイヤーの上に重ねる層を決めるための状態である。 */
export interface PlayerStatus {
  /** 読み込み中（再生を求めたのに映像が来ていない）。 */
  loading: boolean;
  /** 通信が切れて、同じ位置から読み込み直そうとしている。 */
  reconnecting: boolean;
  playing: boolean;
  /** video.js の user-active（操作バーが見えている）。 */
  userActive: boolean;
  ended: boolean;
}

export const initialPlayerStatus: PlayerStatus = {
  loading: false,
  reconnecting: false,
  playing: false,
  userActive: true,
  ended: false,
};

interface Props {
  video: Video;
  /** 作ったときの 1 回だけ読む。取り直しで変わってもプレイヤーは作り直さない。 */
  initialPositionMs: number;
  /** 作ったときの 1 回だけ読む。真なら作ったらすぐに再生を始める。 */
  autoplay: boolean;
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  /**
   * 再生できなかった。positionMs は失敗した論理上の位置、kind は見る人に伝える失敗の種類。
   * 通信が切れたときは読み込み直しを使い切ってから呼ぶ。
   */
  onError: (positionMs: number, kind: PlaybackFailureKind) => void;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
  /** 映像の寸法が分かったら、その横÷縦の比率を知らせる（縦長なら 1 未満）。 */
  onAspectRatio?: (ratio: number) => void;
  /**
   * 全画面にする要素（プレイヤーと、その上に重ねる層を含む入れ物）。無ければ video.js の
   * 既定どおりプレイヤーだけを全画面にする。
   */
  fullscreenTarget?: () => HTMLElement | null;
}

/** FullscreenPlayer は、全画面の先を差し替えるために触る video.js の Player の部分である。 */
interface FullscreenPlayer {
  isFullscreen(value?: boolean): boolean | undefined;
  requestFullscreen(): Promise<void>;
  exitFullscreen(): Promise<void>;
  trigger(event: string): void;
  /** video.js が document の fullscreenchange で呼ぶ（内部の名前）。 */
  documentFullscreenChange_?: () => void;
}

/**
 * VideoPlayer は video.js のプレイヤーを 1 つ持つ。
 *
 * 作り直すのは、再生に関わる値（id・`probeState`・`durationMs`・`playable`）が変わった
 * ときだけにする（plan の Structural Decisions 7）。処理中の取り直しで `thumbnailState` などが
 * 変わるたびに作り直すと、再生が途切れる。
 */
export default function VideoPlayer(props: Props) {
  const { video } = props;
  const hostRef = useRef<HTMLDivElement | null>(null);
  const latest = useRef(props);
  const playerRef = useRef<ReturnType<typeof videojs> | null>(null);
  const popoverOpen = useRef(false);
  const [indicatorSlot, setIndicatorSlot] = useState<HTMLElement | null>(null);
  const [restartSlot, setRestartSlot] = useState<HTMLElement | null>(null);
  const [route, setRoute] = useState<PlaybackRoute | null>(null);
  /** 最初の読み込みが終わるまで操作バーを隠す（自動で再生を始めるとき）。 */
  const [holdControlBar, setHoldControlBar] = useState(true);
  const [playerReady, setPlayerReady] = useState(false);
  /** 全画面にしている入れ物。吹き出しはその中に描かないと全画面の間に見えない。 */
  const [fullscreenFrame, setFullscreenFrame] = useState<HTMLElement | null>(null);

  useEffect(() => {
    latest.current = props;
  });

  useEffect(() => {
    const host = hostRef.current;
    const current = latest.current;
    const initialPositionMs = current.initialPositionMs;
    const initialAttempt = createPlaybackAttempt(current.video, initialPositionMs);
    if (host === null || initialAttempt === null) return;
    const source = current.video;

    // 言語はプレイヤーを作るときにカタログから作り直す（テストの疑似ロケールも届く）。
    videojs.addLanguage(playerLanguage, playerDictionary());
    const element = document.createElement("video-js");
    element.classList.add("video-js", "vjs-big-play-centered");
    host.appendChild(element);

    const player = videojs(element, {
      controls: true,
      fill: true,
      playsinline: true,
      poster: source.thumbnailUrl,
      preload: "metadata",
      language: playerLanguage,
      // 失敗はプレイヤーの上の層で伝える。video.js 自身の誤りの面は出さない。
      errorDisplay: false,
      playbackRates,
      controlBar: {
        children: controlBarChildren,
        remainingTimeDisplay: false,
      },
    });
    const savedVolume = readPlaybackVolume();
    player.volume(savedVolume.volume);
    player.muted(savedVolume.muted);
    player.on("volumechange", () => {
      writePlaybackVolume({
        volume: player.volume() ?? savedVolume.volume,
        muted: player.muted() ?? savedVolume.muted,
      });
    });
    playerRef.current = player;
    let attempt: PlaybackAttempt = initialAttempt;
    let switchingSource = false;
    let resumeApplied = false;
    // 通信が切れたときの読み込み直し（playbackRecovery.ts）。reconnectCount は続けて
    // 読み込み直した回数で、読み込み直したあと実際に recoveredAfterMs 再生したら数え直す。
    // playedSinceRecoveryMs はその再生した長さで、続けて届いた timeupdate の差
    // （maxPlaybackStepMs 以下）だけを足す。シークは lastTickMs を捨てて数えない。
    // resumeAtMs は直接再生を読み込み直したときに
    // メタデータの後で戻す位置である。recoveryGeneration は誤り・読み込み直しのたびに
    // 進め、古い確かめや読み込み直しの結果を捨てる。
    let reconnectCount = 0;
    let reconnectTimer: number | undefined;
    let playedSinceRecoveryMs: number | null = null;
    let lastTickMs: number | null = null;
    let resumeAtMs: number | undefined;
    let recoveryGeneration = 0;
    let reachability: AbortController | undefined;
    let slot: HTMLElement | null = null;
    let restart: HTMLElement | null = null;
    let status: PlayerStatus = { ...initialPlayerStatus };
    setRoute(attempt.route);
    setHoldControlBar(true);

    // 全画面は、プレイヤーだけでなく上に重ねる層ごと（入れ物ごと）にする。video.js の
    // 全画面の操作（ボタン・ダブルクリック）も F キーも、この差し替えを通る。
    const frame = current.fullscreenTarget?.() ?? null;
    let syncFullscreen: (() => void) | undefined;
    if (frame !== null) {
      const fs = player as unknown as FullscreenPlayer;
      syncFullscreen = () => {
        const on = document.fullscreenElement === frame;
        if (fs.isFullscreen() !== on) {
          fs.isFullscreen(on);
          // 接頭辞の無い API では video.js は自分で知らせないので、全画面ボタンの表示を
          // 切り替えるために知らせる。受けた側がまた呼んでも、状態が同じなので止まる。
          fs.trigger("fullscreenchange");
        }
        setFullscreenFrame(on ? frame : null);
      };
      fs.requestFullscreen = () =>
        frame.requestFullscreen?.().catch(() => undefined) ?? Promise.resolve();
      fs.exitFullscreen = () =>
        document.fullscreenElement != null
          ? document.exitFullscreen().catch(() => undefined)
          : Promise.resolve();
      fs.documentFullscreenChange_ = syncFullscreen;
      document.addEventListener("fullscreenchange", syncFullscreen);
    }

    const setStatus = (next: Partial<PlayerStatus>) => {
      const merged = { ...status, ...next };
      if (
        merged.loading === status.loading &&
        merged.reconnecting === status.reconnecting &&
        merged.playing === status.playing &&
        merged.userActive === status.userActive &&
        merged.ended === status.ended
      ) {
        return;
      }
      status = merged;
      latest.current.onStatus(status);
    };
    latest.current.onStatus(status);

    const menuOpen = () => popoverOpen.current || rateMenuOpen(host);
    latest.current.onControls(
      createPlayerControls(player as unknown as ControllablePlayer, menuOpen),
    );

    player.ready(() => {
      if (player.isDisposed()) return;
      setPlayerReady(true);
      for (const [selector, keys] of keyShortcuts) {
        host.querySelector(selector)?.setAttribute("aria-keyshortcuts", keys);
      }
      const bar = host.querySelector<HTMLElement>(".vjs-control-bar");
      const playControl = bar?.querySelector<HTMLElement>(":scope > .vjs-play-control");
      if (bar != null && playControl != null) {
        restart = document.createElement("div");
        restart.className = "vv-player-restart flex flex-none";
        bar.insertBefore(restart, playControl);
        setRestartSlot(restart);
      }
      if (bar !== null) {
        slot = document.createElement("div");
        slot.className = "vv-transcode-indicator flex flex-none items-center px-2";
        bar.insertBefore(slot, bar.querySelector(":scope > .vjs-playback-rate"));
        setIndicatorSlot(slot);
      }
    });

    const logicalPositionMs = () => {
      if (attempt.state === "loading") return attempt.logicalPositionMs;
      const seconds = player.currentTime();
      const position = typeof seconds === "number" ? seconds * 1000 : 0;
      attempt = updatePosition(attempt, position);
      return attempt.logicalPositionMs;
    };
    const reportPosition = () => {
      const position = logicalPositionMs();
      latest.current.onPosition(position);
      return position;
    };
    const setLiveSource = (positionMs: number) => {
      player.src(
        liveSource(source.id, attempt.durationMs, positionMs, (seconds) => {
          attempt = {
            ...updatePosition(attempt, seconds * 1000),
            sourceOffsetMs: seconds * 1000,
          };
          latest.current.onPosition(attempt.logicalPositionMs);
        }),
      );
    };

    player.on("loadedmetadata", () => {
      attempt = { ...attempt, state: "ready" };
      const videoWidth = player.videoWidth();
      const videoHeight = player.videoHeight();
      if (videoWidth > 0 && videoHeight > 0) {
        latest.current.onAspectRatio?.(videoWidth / videoHeight);
      }
      setHoldControlBar(false);
      if (resumeAtMs !== undefined) {
        const at = resumeAtMs;
        resumeAtMs = undefined;
        player.currentTime(at / 1000);
        latest.current.onPosition(at);
        return;
      }
      if (resumeApplied || initialPositionMs <= 0) return;
      resumeApplied = true;
      if (attempt.route === "direct") {
        player.currentTime(initialPositionMs / 1000);
        latest.current.onPosition(initialPositionMs);
        return;
      }
      // ライブ変換はコピーで始めると直前のキーフレームから映る。仲立ちが実際の開始位置に
      // 合わせた論理時刻を伝える（contracts/transcode-start-api.md §3）。
      reportPosition();
    });
    player.on("timeupdate", () => {
      const position = reportPosition();
      if (playedSinceRecoveryMs === null) return;
      if (lastTickMs !== null) {
        const step = position - lastTickMs;
        if (step > 0 && step <= maxPlaybackStepMs) playedSinceRecoveryMs += step;
      }
      lastTickMs = position;
      if (playedSinceRecoveryMs >= recoveredAfterMs) {
        reconnectCount = 0;
        playedSinceRecoveryMs = null;
        lastTickMs = null;
      }
    });
    player.on("play", () => {
      attempt = { ...attempt, state: "playing", playIntended: true };
      setStatus({ playing: true, ended: false });
    });
    player.on("waiting", () => setStatus({ loading: true }));
    for (const event of ["playing", "canplay", "seeked"]) {
      player.on(event, () => setStatus({ loading: false }));
    }
    player.on("seeking", () => {
      lastTickMs = null;
      setStatus({ ended: false });
    });
    player.on("useractive", () => setStatus({ userActive: true }));
    player.on("userinactive", () => setStatus({ userActive: false }));
    player.on("pause", () => {
      if (switchingSource || player.error() !== null) return;
      attempt = { ...attempt, playIntended: false };
      setStatus({ playing: false, loading: false });
      latest.current.onProgress(reportPosition(), true);
    });
    player.on("ended", () => {
      setStatus({ playing: false, loading: false, ended: true });
      latest.current.onProgress(reportPosition(), true);
    });
    const setDirectSource = () => {
      player.src({ src: streamUrl(source.id), type: directContentType(source) });
    };

    const fail = (kind: PlaybackFailureKind, position: number) => {
      switchingSource = false;
      attempt = { ...attempt, state: "failed" };
      setStatus({ playing: false, loading: false, reconnecting: false });
      latest.current.onError(position, kind);
      // 誤りの印（vjs-error）は操作バーを隠す。失敗はプレイヤーの上の層で伝え、
      // 操作バーは見えるままにする（ui-design「Overlay layer」）。読み込みの前に
      // 失敗したときも、操作バーを隠したままにしない。
      player.error(null);
      setHoldControlBar(false);
      // 変換へ切り替えると video.js は「再生を始めた」印（vjs-has-started）を外し、
      // 操作バーを出さなくなる。失敗の層の下でも操作バーを押せるように付け直す。
      player.hasStarted(true);
    };

    // reload は通信が切れた位置から今の経路を読み込み直す。直接再生はメタデータが来たら
    // その位置へ戻し、変換は位置から始め直す。どちらもメタデータが来たら読み込み直しの
    // 表示を下ろし、再生していたなら再生を続ける。
    const reload = () => {
      reconnectTimer = undefined;
      if (player.isDisposed()) return;
      const generation = ++recoveryGeneration;
      const position = attempt.logicalPositionMs;
      resumeApplied = true;
      player.one("loadedmetadata", () => {
        if (generation !== recoveryGeneration || player.isDisposed()) return;
        switchingSource = false;
        playedSinceRecoveryMs = 0;
        lastTickMs = position;
        setStatus({ reconnecting: false });
        if (attempt.playIntended) {
          setStatus({ loading: true });
          void player.play()?.catch(() => setStatus({ loading: false }));
        }
      });
      if (attempt.route === "direct") {
        resumeAtMs = position;
        setDirectSource();
      } else {
        setLiveSource(position);
      }
    };

    const scheduleReconnect = (position: number) => {
      const delay = reconnectDelay(reconnectCount);
      if (delay === null) {
        fail("network", position);
        return;
      }
      reconnectCount += 1;
      setStatus({ playing: false, loading: false, reconnecting: true });
      setHoldControlBar(false);
      player.hasStarted(true);
      reconnectTimer = window.setTimeout(reload, delay);
    };

    // 端末が回線に戻ったら、待ちを切り上げてすぐ読み込み直す。
    const reconnectNow = () => {
      if (reconnectTimer === undefined) return;
      window.clearTimeout(reconnectTimer);
      reload();
    };
    window.addEventListener("online", reconnectNow);

    const handleFailure = (
      kind: PlaybackFailureKind,
      position: number,
      playIntended: boolean,
    ) => {
      if (kind === "network") {
        scheduleReconnect(position);
        return;
      }
      // 直接再生で形式が読めないときだけ、1 回だけ変換へ切り替える。
      const fallback = fallbackToTranscode(attempt, position, playIntended);
      if (fallback === null) {
        fail(kind, position);
        return;
      }
      attempt = fallback;
      setRoute("transcode");
      setStatus({ reconnecting: false });
      setLiveSource(attempt.sourceOffsetMs);
      player.one("canplay", () => {
        switchingSource = false;
        if (attempt.playIntended) void player.play()?.catch(() => undefined);
        else player.pause();
      });
    };

    // 誤りは種類で分ける（playbackRecovery.ts）。通信の失敗は同じ経路で読み込み直し、
    // 形式・データの失敗は直接再生なら変換へ切り替える。番号だけで決まらない誤りは
    // サーバーに届くかを確かめてから分ける。
    player.on("error", () => {
      const code = (player.error() as { code?: number } | null)?.code;
      const position = logicalPositionMs();
      const playIntended = attempt.playIntended || !player.paused();
      const generation = ++recoveryGeneration;
      reachability?.abort();
      reachability = undefined;
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
      attempt = {
        ...updatePosition(attempt, position),
        state: "loading",
        playIntended,
      };
      // 確かめと読み込み直しの間は、誤りの印（操作バーを隠す）を外し、止まったことを
      // 再生を止めた操作として扱わない。
      switchingSource = true;
      player.error(null);

      const errorClass = classifyMediaError(code);
      if (errorClass !== "ambiguous") {
        handleFailure(errorClass, position, playIntended);
        return;
      }
      setStatus({ loading: true });
      const controller = new AbortController();
      reachability = controller;
      void serverReachable(controller.signal).then((reachable) => {
        if (controller.signal.aborted || player.isDisposed()) return;
        if (generation !== recoveryGeneration) return;
        reachability = undefined;
        handleFailure(reachable ? "source" : "network", position, playIntended);
      });
    });

    if (attempt.route === "direct") {
      setDirectSource();
    } else {
      setLiveSource(attempt.sourceOffsetMs);
    }
    if (current.autoplay) {
      setStatus({ loading: true });
      void player.play()?.catch(() => setStatus({ loading: false }));
    }

    const timer = window.setInterval(() => {
      if (!player.paused() && !player.ended()) {
        latest.current.onProgress(reportPosition(), false);
      }
    }, saveIntervalMs);

    return () => {
      window.clearInterval(timer);
      window.removeEventListener("online", reconnectNow);
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      recoveryGeneration += 1;
      reachability?.abort();
      if (syncFullscreen !== undefined) {
        document.removeEventListener("fullscreenchange", syncFullscreen);
      }
      setFullscreenFrame(null);
      setPlayerReady(false);
      slot?.remove();
      setIndicatorSlot(null);
      restart?.remove();
      setRestartSlot(null);
      popoverOpen.current = false;
      latest.current.onControls(null);
      attempt = { ...attempt, state: "disposed" };
      playerRef.current = null;
      if (!player.isDisposed()) player.dispose();
    };
  }, [video.id, video.probeState, video.durationMs, video.playable]);

  // シーク位置サムネイルは、処理中の取り直しで後から URL が来ることもある。プレイヤーは
  // 作り直さず、再生バーへの取り付けだけをやり直す。URL は同じでも、生成が終わって
  // seekThumbnailState が変わったら取り付け直し、取得に失敗した配置情報を捨てる。
  const seekThumbnailUrl = video.seekThumbnailUrl;
  const seekThumbnailState = video.seekThumbnailState;
  const durationMs = video.durationMs;
  useEffect(() => {
    const host = hostRef.current;
    if (!playerReady || host === null || seekThumbnailUrl === undefined) return;
    if (durationMs === undefined || durationMs <= 0) return;
    const progress = host.querySelector<HTMLElement>(".vjs-progress-holder");
    const progressControl = host.querySelector<HTMLElement>(".vjs-progress-control");
    if (progress === null || progressControl === null) return;
    return attachSeekPreview(
      progress,
      { durationMs, thumbnailUrl: seekThumbnailUrl },
      progressControl,
    );
  }, [durationMs, playerReady, seekThumbnailUrl, seekThumbnailState]);

  // 代表サムネイルが後からできたときは、作り直さずに背景の画像だけを差し替える。
  useEffect(() => {
    const player = playerRef.current;
    if (player !== null && !player.isDisposed() && video.thumbnailUrl !== undefined) {
      player.poster(video.thumbnailUrl);
    }
  }, [video.thumbnailUrl]);

  return (
    <div
      ref={hostRef}
      data-loading={holdControlBar ? "true" : undefined}
      className="vv-video-player absolute inset-0"
    >
      {restartSlot !== null &&
        createPortal(
          <BarButton
            label={t.player.controls.restart}
            keys="0"
            icon={RotateCcw}
            onClick={() => {
              const player = playerRef.current;
              if (player !== null && !player.isDisposed()) player.currentTime(0);
            }}
          />,
          restartSlot,
        )}
      {indicatorSlot !== null &&
        route === "transcode" &&
        createPortal(
          <TranscodeIndicator
            container={fullscreenFrame}
            onOpenChange={(open) => {
              popoverOpen.current = open;
            }}
          />,
          indicatorSlot,
        )}
    </div>
  );
}

/** BarButton は操作バーへ差し込むボタンである。video.js のボタンと同じ見た目と大きさにそろえる。 */
function BarButton({
  label,
  keys,
  icon: Icon,
  onClick,
}: {
  label: UiText;
  keys?: string;
  icon: LucideIcon;
  onClick: () => void;
}) {
  const title = keys === undefined ? label : t.player.controls.withKey(label, keys);
  return (
    <button
      type="button"
      className="vjs-control vjs-button vv-bar-button"
      aria-label={label}
      aria-keyshortcuts={keys}
      title={title}
      onClick={onClick}
    >
      <Icon className="size-[1.6em]" aria-hidden="true" />
    </button>
  );
}

/**
 * TranscodeIndicator は操作バーの中の「変換して再生中」である（要件 10）。
 *
 * ポイントしたときだけ開くツールチップにはしない。タッチやキーボードの人が理由に
 * 届かなくなるので、押して開く吹き出しにする。
 */
function TranscodeIndicator({
  container,
  onOpenChange,
}: {
  container: HTMLElement | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <PopoverRoot onOpenChange={onOpenChange}>
      {/* video.js の `.video-js button` が表示・文字の大きさ・色を上書きするので、! で戻す。 */}
      <PopoverTrigger className="inline-flex! items-center gap-1 rounded-sm px-1 text-xs! leading-4! whitespace-nowrap text-fg-muted! transition-colors! hover:text-fg!">
        <Info className="size-3.5 shrink-0" aria-hidden="true" />
        {/* 縦長の動画などで枠が狭いときは、印だけを残して操作バーの幅に収める。 */}
        <span className="@max-[22.5rem]:sr-only">{t.player.controls.transcoding}</span>
      </PopoverTrigger>
      <PopoverContent container={container} className="w-64 text-sm text-fg">
        {t.player.controls.transcodingDetail}
      </PopoverContent>
    </PopoverRoot>
  );
}

function directContentType(video: Video): string {
  return video.container === "webm" ? "video/webm" : "video/mp4";
}

export function canStartPlayback(video: Video): boolean {
  return createPlaybackAttempt(video, 0) !== null;
}
