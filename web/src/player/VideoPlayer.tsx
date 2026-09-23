import { Info } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import videojs from "video.js";
import "video.js/dist/video-js.css";

import { streamUrl, type Video } from "../api/client";
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
} from "./playerControls";
import { attachSeekPreview } from "./seekPreview";

const saveIntervalMs = 5000;

/** playbackRates は操作バーの再生速度の選択肢である（ui-design「Control bar」）。 */
export const playbackRates = [0.5, 0.75, 1, 1.25, 1.5, 2];

/**
 * 操作バーの並び（要件 6）。残り時間は出さず、現在時刻/長さを出す。再生バーは
 * index.css で操作バーの上へ出す。「変換して再生中」は再生速度の前へ差し込む。
 */
const controlBarChildren = [
  "playToggle",
  "skipBackward",
  "skipForward",
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
 * 操作バーの読み上げ名とポイントしたときの説明。キーボード操作を持つボタンには
 * キーを添える（ui-design「Control bar」）。video.js は同じ文字を title にも使う。
 */
const language = "vv-ja";
videojs.addLanguage(language, {
  Play: "再生（Space）",
  Pause: "一時停止（Space）",
  Replay: "もう一度再生（Space）",
  "Play Video": "再生",
  "Skip backward {1} seconds": "{1} 秒戻る（←）",
  "Skip forward {1} seconds": "{1} 秒進む（→）",
  Mute: "ミュート（M）",
  Unmute: "ミュートを解除（M）",
  Fullscreen: "全画面（F）",
  "Exit Fullscreen": "全画面を終了（F）",
  "Picture-in-Picture": "ピクチャーインピクチャー",
  "Exit Picture-in-Picture": "ピクチャーインピクチャーを終了",
  "Playback Rate": "再生速度",
  "Current Time": "現在の時刻",
  Duration: "長さ",
  "Progress Bar": "再生位置",
  "Volume Level": "音量",
  "Video Player": "動画プレイヤー",
});

/** キーボード操作を持つボタンと、そのキー（aria-keyshortcuts）。 */
const keyShortcuts: [string, string][] = [
  [".vjs-play-control", "Space"],
  [".vjs-skip-backward-10", "ArrowLeft"],
  [".vjs-skip-forward-10", "ArrowRight"],
  [".vjs-mute-control", "M"],
  [".vjs-fullscreen-control", "F"],
];

/** PlayerStatus は、プレイヤーの上に重ねる層を決めるための状態である。 */
export interface PlayerStatus {
  /** 読み込み中（再生を求めたのに映像が来ていない）。 */
  loading: boolean;
  playing: boolean;
  /** video.js の user-active（操作バーが見えている）。 */
  userActive: boolean;
  ended: boolean;
}

export const initialPlayerStatus: PlayerStatus = {
  loading: false,
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
  /** 再生できなかった。positionMs は失敗した論理上の位置。 */
  onError: (positionMs: number) => void;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
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
  const [route, setRoute] = useState<PlaybackRoute | null>(null);
  const [metadataLoaded, setMetadataLoaded] = useState(false);

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

    const element = document.createElement("video-js");
    element.classList.add("video-js", "vjs-big-play-centered");
    host.appendChild(element);

    const player = videojs(element, {
      controls: true,
      fill: true,
      playsinline: true,
      poster: source.thumbnailUrl,
      preload: "metadata",
      language,
      // 失敗はプレイヤーの上の層で伝える。video.js 自身の誤りの面は出さない。
      errorDisplay: false,
      playbackRates,
      controlBar: {
        children: controlBarChildren,
        skipButtons: { forward: 10, backward: 10 },
        remainingTimeDisplay: false,
      },
    });
    playerRef.current = player;
    let attempt: PlaybackAttempt = initialAttempt;
    let switchingSource = false;
    let resumeApplied = false;
    let detachSeekPreview: (() => void) | undefined;
    let slot: HTMLElement | null = null;
    let status: PlayerStatus = { ...initialPlayerStatus };
    setRoute(attempt.route);
    setMetadataLoaded(false);

    const setStatus = (next: Partial<PlayerStatus>) => {
      const merged = { ...status, ...next };
      if (
        merged.loading === status.loading &&
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

    const menuOpen = () =>
      popoverOpen.current ||
      host.querySelector(".vjs-menu.vjs-lock-showing") !== null ||
      host.querySelector(".vjs-menu-button-popup.vjs-hover") !== null;
    latest.current.onControls(
      createPlayerControls(player as unknown as ControllablePlayer, menuOpen),
    );

    player.ready(() => {
      if (player.isDisposed()) return;
      for (const [selector, keys] of keyShortcuts) {
        host.querySelector(selector)?.setAttribute("aria-keyshortcuts", keys);
      }
      const bar = host.querySelector<HTMLElement>(".vjs-control-bar");
      if (bar !== null) {
        slot = document.createElement("div");
        slot.className = "vv-transcode-indicator flex flex-none items-center px-2";
        bar.insertBefore(slot, bar.querySelector(":scope > .vjs-playback-rate"));
        setIndicatorSlot(slot);
      }

      const seekThumbnailUrl = latest.current.video.seekThumbnailUrl;
      if (seekThumbnailUrl === undefined) return;
      const progress = host.querySelector<HTMLElement>(".vjs-progress-holder");
      const progressControl = host.querySelector<HTMLElement>(".vjs-progress-control");
      if (progress !== null && progressControl !== null) {
        detachSeekPreview = attachSeekPreview(
          progress,
          { durationMs: attempt.durationMs, thumbnailUrl: seekThumbnailUrl },
          progressControl,
        );
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
      setMetadataLoaded(true);
      if (resumeApplied || initialPositionMs <= 0) return;
      resumeApplied = true;
      if (attempt.route === "direct") player.currentTime(initialPositionMs / 1000);
      latest.current.onPosition(initialPositionMs);
    });
    player.on("timeupdate", reportPosition);
    player.on("play", () => {
      attempt = { ...attempt, state: "playing", playIntended: true };
      setStatus({ playing: true, ended: false });
    });
    player.on("waiting", () => setStatus({ loading: true }));
    for (const event of ["playing", "canplay", "seeked"]) {
      player.on(event, () => setStatus({ loading: false }));
    }
    player.on("seeking", () => setStatus({ ended: false }));
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
    player.on("error", () => {
      const position = logicalPositionMs();
      const fallback = fallbackToTranscode(
        attempt,
        position,
        attempt.playIntended || !player.paused(),
      );
      if (fallback === null) {
        attempt = { ...attempt, state: "failed" };
        setStatus({ playing: false, loading: false });
        latest.current.onError(position);
        // 誤りの印（vjs-error）は操作バーを隠す。失敗はプレイヤーの上の層で伝え、
        // 操作バーは見えるままにする（ui-design「Overlay layer」）。
        player.error(null);
        return;
      }

      attempt = fallback;
      setRoute("transcode");
      switchingSource = true;
      player.error(null);
      setLiveSource(attempt.sourceOffsetMs);
      player.one("canplay", () => {
        switchingSource = false;
        if (attempt.playIntended) void player.play()?.catch(() => undefined);
        else player.pause();
      });
    });

    if (attempt.route === "direct") {
      player.src({ src: streamUrl(source.id), type: directContentType(source) });
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
      detachSeekPreview?.();
      slot?.remove();
      setIndicatorSlot(null);
      popoverOpen.current = false;
      latest.current.onControls(null);
      attempt = { ...attempt, state: "disposed" };
      playerRef.current = null;
      if (!player.isDisposed()) player.dispose();
    };
  }, [video.id, video.probeState, video.durationMs, video.playable]);

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
      data-loading={metadataLoaded ? undefined : "true"}
      className="vv-video-player absolute inset-0"
    >
      {indicatorSlot !== null &&
        route === "transcode" &&
        createPortal(
          <TranscodeIndicator
            onOpenChange={(open) => {
              popoverOpen.current = open;
            }}
          />,
          indicatorSlot,
        )}
    </div>
  );
}

/**
 * TranscodeIndicator は操作バーの中の「変換して再生中」である（要件 10）。
 *
 * ポイントしたときだけ開くツールチップにはしない。タッチやキーボードの人が理由に
 * 届かなくなるので、押して開く吹き出しにする。
 */
function TranscodeIndicator({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  return (
    <PopoverRoot onOpenChange={onOpenChange}>
      {/* video.js の `.video-js button` が表示・文字の大きさ・色を上書きするので、! で戻す。 */}
      <PopoverTrigger className="inline-flex! items-center gap-1 rounded-sm px-1 text-xs! leading-4! whitespace-nowrap text-fg-muted! transition-colors! hover:text-fg!">
        <Info className="size-3.5 shrink-0" aria-hidden="true" />
        変換して再生中
      </PopoverTrigger>
      <PopoverContent className="w-64 text-sm text-fg">
        ブラウザがそのまま再生できない形式のため、変換しながら再生しています。シークに数秒かかります。
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
