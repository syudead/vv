import { Info, RotateCcw, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import videojs from "video.js";
import "video.js/dist/video-js.css";

import { streamUrl, type SubtitleTrack, type Video } from "../api/client";
import { t, type UiText } from "../i18n";
import {
  readPlaybackQuality,
  writePlaybackQuality,
  type PlaybackQuality,
} from "../preferences/playbackQuality";
import { readPlaybackVolume, writePlaybackVolume } from "../preferences/playbackVolume";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import {
  attachChapterMarks,
  attachChaptersTrack,
  chaptersButtonName,
  chapterTitleAt,
  playerChapters,
  registerChaptersButton,
  type ChaptersPlayer,
  type PlayerChapter,
} from "./chapters";
import { liveSource } from "./liveOffset";
import {
  createPlaybackAttempt,
  fallbackToTranscode,
  switchQuality,
  updatePosition,
  type PlaybackAttempt,
  type PlaybackRoute,
} from "./playbackAttempt";
import {
  createPlayerControls,
  type ControllablePlayer,
  type PlayerControls,
  controlBarMenuOpen,
} from "./playerControls";
import {
  classifyMediaError,
  probeMediaSource,
  reconnectDelay,
  recoveredAfterMs,
  reloadTimeoutMs,
  type PlaybackFailureKind,
} from "./playbackRecovery";
import { effectiveQuality, qualityOptions, sourceShortSide } from "./quality";
import { qualitySelectEvent, setQualityMenu, type QualitySelection } from "./qualityMenu";
import { attachSeekPreview } from "./seekPreview";
import {
  initialStallState,
  stallStep,
  waitDeadlineMs,
  type StallEvent,
  type StallState,
} from "./stallMonitor";
import { registerSubtitlesButton, subtitlesButtonName } from "./subtitleMenu";
import {
  createSubtitleTracks,
  type SubtitlePlayer,
  type SubtitleTracks,
} from "./subtitleTracks";

const saveIntervalMs = 5000;
/** Treat larger timeupdate jumps as seeks; normal playback updates about every 250ms, even at 2x speed. */
const maxPlaybackStepMs = 2000;

export const playbackRates = [0.5, 0.75, 1, 1.25, 1.5, 2];

/** Custom controls extend video.js; see docs/design-docs/media-information.md for chapter behavior. */
const controlBarChildren = [
  "playToggle",
  "volumePanel",
  "currentTimeDisplay",
  "timeDivider",
  "durationDisplay",
  "progressControl",
  "customControlSpacer",
  chaptersButtonName,
  "qualityMenuButton",
  subtitlesButtonName,
  "playbackRateMenuButton",
  "pictureInPictureToggle",
  "fullscreenToggle",
];

/** Use an English private language subtag for catalog labels and shortcut hints; missing labels fall back to en. */
export const playerLanguage = "en-x-vv";

export function playerDictionary(): Record<string, string> {
  const c = t.player.controls;
  const s = t.player.subtitles;
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
    ", selected": c.menuItemSelected,
    // Keep subtitle and caption labels consistent for the private language subtag.
    Subtitles: c.withKey(s.button, "C"),
    Captions: c.withKey(s.button, "C"),
    "subtitles off": s.off,
    "captions off": s.off,
    "captions and subtitles off": s.off,
    Chapters: t.player.chapters.button,
  };
}

const keyShortcuts: [string, string][] = [
  [".vjs-play-control", "Space"],
  [".vjs-mute-control", "M"],
  [".vjs-fullscreen-control", "F"],
  ["button.vjs-subs-caps-button", "C"],
];

export interface PlayerStatus {
  loading: boolean;
  reconnecting: boolean;
  playing: boolean;
  /** video.js user-active state, indicating visible controls. */
  userActive: boolean;
  ended: boolean;
  /** A stall warning does not pause playback; clear it on end or failure. */
  stalled: boolean;
  /** Latched once resume seeking or live-offset reporting confirms the displayed position. Thumbnail capture must wait for this. */
  positioned: boolean;
}

export const initialPlayerStatus: PlayerStatus = {
  loading: false,
  reconnecting: false,
  playing: false,
  userActive: true,
  ended: false,
  stalled: false,
  positioned: false,
};

interface Props {
  video: Video;
  /** Read only at creation; refreshes must not recreate the player. */
  initialPositionMs: number;
  /** Read only at creation to decide whether to autoplay. */
  autoplay: boolean;
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  /** Begin watch history on play; recovery reloads do not start a new watch. */
  onPlay?: () => void;
  /** Report logical position and failure kind only after reconnect retries are exhausted. */
  onError: (positionMs: number, kind: PlaybackFailureKind) => void;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
  onAspectRatio?: (ratio: number) => void;
  /** Include overlays in fullscreen; defaults to video.js player-only fullscreen. */
  fullscreenTarget?: () => HTMLElement | null;
  /** Replace tracks when subtitles arrive without recreating the player. */
  subtitles?: readonly SubtitleTrack[];
}

const noSubtitles: readonly SubtitleTrack[] = [];

interface FullscreenPlayer {
  isFullscreen(value?: boolean): boolean | undefined;
  requestFullscreen(): Promise<void>;
  exitFullscreen(): Promise<void>;
  trigger(event: string): void;
  /** Internal video.js callback for document fullscreenchange. */
  documentFullscreenChange_?: () => void;
}

/** Recreate only when playback-relevant fields change; thumbnail and processing refreshes must not interrupt playback. */
export default function VideoPlayer(props: Props) {
  const { video } = props;
  const hostRef = useRef<HTMLDivElement | null>(null);
  const latest = useRef(props);
  const playerRef = useRef<ReturnType<typeof videojs> | null>(null);
  const popoverOpen = useRef(false);
  const subtitlesRef = useRef<SubtitleTracks | null>(null);
  const [indicatorSlot, setIndicatorSlot] = useState<HTMLElement | null>(null);
  const [restartSlot, setRestartSlot] = useState<HTMLElement | null>(null);
  const [route, setRoute] = useState<PlaybackRoute | null>(null);
  const [quality, setQuality] = useState<PlaybackQuality>("original");
  const [holdControlBar, setHoldControlBar] = useState(true);
  const [playerReady, setPlayerReady] = useState(false);
  /** Tooltips must render inside the fullscreen container to remain visible. */
  const [fullscreenFrame, setFullscreenFrame] = useState<HTMLElement | null>(null);

  useEffect(() => {
    latest.current = props;
  });

  useEffect(() => {
    const host = hostRef.current;
    const current = latest.current;
    const initialPositionMs = current.initialPositionMs;
    // Read saved quality once; unavailable choices fall back without overwriting the preference.
    const initialAttempt = createPlaybackAttempt(
      current.video,
      initialPositionMs,
      effectiveQuality(readPlaybackQuality(), current.video),
    );
    if (host === null || initialAttempt === null) return;
    const source = current.video;

    // Rebuild catalog labels on creation so test pseudolocales also apply.
    videojs.addLanguage(playerLanguage, playerDictionary());
    registerSubtitlesButton();
    registerChaptersButton();
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
      // The page overlay reports failures; suppress video.js's own error panel.
      errorDisplay: false,
      playbackRates,
      textTrackSettings: false,
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
    const subtitles = createSubtitleTracks(player as unknown as SubtitlePlayer);
    subtitles.replace(source.id, current.subtitles ?? noSubtitles);
    subtitlesRef.current = subtitles;
    let attempt: PlaybackAttempt = initialAttempt;
    let switchingSource = false;
    let resumeApplied = false;
    // Positioning waits for initial metadata, the direct resume seek, and any live-offset report.
    let metadataAccepted = false;
    // Report positions only after play; otherwise leaving would create a watch record for an unplayed video.
    let played = false;
    let resumeSeekPending = false;
    let liveOffsetPending = false;
    // During recovery, retain user intent and position instead of forwarding controls to the broken source.
    // Reset reconnect attempts only after sustained playback; seeks and pauses do not count.
    // Reload deadlines handle stalled requests; generations reject stale checks and timeout results.
    let recovering = false;
    let reconnectCount = 0;
    let reconnectTimer: number | undefined;
    let reloadDeadline: number | undefined;
    let pendingRecovery: { generation: number } | null = null;
    // Track user seeks separately from offset reports, which also change the attempt position.
    let seekDuringReloadMs: number | null = null;
    let playedSinceRecoveryMs: number | null = null;
    let lastTickMs: number | null = null;
    let recoveryGeneration = 0;
    // Accept switch metadata only for the current source; restore rate because replacing a source resets it.
    // Once a source reaches the element, stale events are discarded; later unbuffered seeks may change its URL.
    let pendingSwitch: { src: string; loaded: boolean; rate: number } | null = null;
    let probe: AbortController | undefined;
    let slot: HTMLElement | null = null;
    let restart: HTMLElement | null = null;
    let status: PlayerStatus = { ...initialPlayerStatus };
    let stall: StallState = initialStallState;
    let stallTimer: number | undefined;

    // During recovery, controls reflect user intent rather than the stalled element's paused state.
    // Play retries immediately; pause and seek update the saved intent and position.
    const media = player as unknown as {
      play(): Promise<void> | undefined;
      pause(): void;
      paused(): boolean;
      currentTime(seconds?: number): number | undefined;
    };
    const mediaPlay = media.play.bind(player);
    const mediaPause = media.pause.bind(player);
    const mediaPaused = media.paused.bind(player);
    const mediaCurrentTime = media.currentTime.bind(player);
    media.play = () => {
      // During quality switches, remember user intent; element pause events cannot identify user actions.
      if (pendingSwitch !== null) attempt = { ...attempt, playIntended: true };
      if (!recovering) return mediaPlay();
      attempt = { ...attempt, playIntended: true };
      player.trigger("play");
      reconnectNow();
      return Promise.resolve();
    };
    media.pause = () => {
      if (pendingSwitch !== null) {
        attempt = { ...attempt, playIntended: false };
        setStatus({ playing: false });
      }
      if (!recovering) {
        mediaPause();
        return;
      }
      attempt = { ...attempt, playIntended: false };
      mediaPause();
      player.trigger("pause");
    };
    media.paused = () => (recovering ? !attempt.playIntended : mediaPaused());
    media.currentTime = (seconds?: number) => {
      // Defer direct-play seeks until metadata arrives; seeking an element loading from zero may fail.
      if (
        !recovering &&
        pendingSwitch !== null &&
        attempt.route === "direct" &&
        seconds !== undefined
      ) {
        attempt = updatePosition(attempt, seconds * 1000);
      }
      if (!recovering) return mediaCurrentTime(seconds);
      if (seconds !== undefined) {
        attempt = updatePosition(attempt, seconds * 1000);
        if (pendingRecovery !== null) seekDuringReloadMs = attempt.logicalPositionMs;
        latest.current.onPosition(attempt.logicalPositionMs);
        player.trigger("timeupdate");
      }
      return attempt.logicalPositionMs / 1000;
    };
    setRoute(attempt.route);
    setQuality(attempt.quality);
    setHoldControlBar(true);

    // Override fullscreen once for buttons, double-click, and keyboard controls, including overlays.
    const frame = current.fullscreenTarget?.() ?? null;
    let syncFullscreen: (() => void) | undefined;
    if (frame !== null) {
      const fs = player as unknown as FullscreenPlayer;
      syncFullscreen = () => {
        const on = document.fullscreenElement === frame;
        if (fs.isFullscreen() !== on) {
          fs.isFullscreen(on);
          // Notify video.js for the unprefixed API so its fullscreen button updates; unchanged state prevents recursion.
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
        merged.ended === status.ended &&
        merged.stalled === status.stalled &&
        merged.positioned === status.positioned
      ) {
        return;
      }
      status = merged;
      latest.current.onStatus(status);
    };
    latest.current.onStatus(status);
    // Latch positioned once ready, then report only after playback starts to avoid false watch records.
    const markPositioned = () => {
      if (metadataAccepted && !resumeSeekPending && !liveOffsetPending) {
        if (!status.positioned && (played || attempt.playIntended)) reportPosition();
        setStatus({ positioned: true });
      }
    };

    const feedStall = (event: StallEvent) => {
      stall = stallStep(stall, event, Date.now());
      if (stallTimer !== undefined) window.clearTimeout(stallTimer);
      stallTimer = undefined;
      const deadline = waitDeadlineMs(stall);
      if (deadline !== null) {
        stallTimer = window.setTimeout(
          () => {
            stallTimer = undefined;
            if (!player.isDisposed()) feedStall("tick");
          },
          Math.max(0, deadline - Date.now()),
        );
      }
      setStatus({ stalled: stall.stalled });
    };
    for (const event of [
      "waiting",
      "playing",
      "seeking",
      "loadstart",
      "play",
      "pause",
    ] as const) {
      player.on(event, () => feedStall(event));
    }

    const menuOpen = () => popoverOpen.current || controlBarMenuOpen(host);
    const controls = createPlayerControls(
      player as unknown as ControllablePlayer,
      menuOpen,
      () => subtitles.toggle(),
      () => reportPosition(),
    );
    latest.current.onControls(controls);
    // On touch devices, tapping video toggles playback; video.js handles poster taps itself.
    player.tech(true).on("tap", () => {
      if (window.matchMedia?.("(pointer: coarse)").matches) controls.togglePlay();
    });

    const renderQualityMenu = () => {
      setQualityMenu(player, {
        options: qualityOptions(source),
        current: attempt.quality,
        sourceSize: sourceShortSide(source),
      });
    };

    player.ready(() => {
      if (player.isDisposed()) return;
      setPlayerReady(true);
      subtitles.start();
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
        // Keep subtitles beside playback speed by inserting the transcode indicator before the quality controls.
        bar.insertBefore(
          slot,
          bar.querySelector(
            ":scope > .vv-quality, :scope > .vjs-subs-caps-button, :scope > .vjs-playback-rate",
          ),
        );
        setIndicatorSlot(slot);
      }
      renderQualityMenu();
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
    // All transcode starts retain attempt quality; liveOffset reloadAt preserves it for unbuffered seeks.
    // Subtitle cues use the output timeline; replace tracks for each confirmed offset and remove them while pending.
    const liveSourceAt = (positionMs: number) =>
      liveSource(
        source.id,
        attempt.durationMs,
        positionMs,
        (seconds) => {
          attempt = {
            ...updatePosition(attempt, seconds * 1000),
            sourceOffsetMs: seconds * 1000,
          };
          latest.current.onPosition(attempt.logicalPositionMs);
        },
        attempt.quality === "original" ? undefined : attempt.quality,
        (seconds) => {
          subtitles.setOffset(seconds * 1000);
          resumeSeekPending = false;
          liveOffsetPending = false;
          markPositioned();
        },
        () => {
          subtitles.setOffset(null);
          resumeSeekPending = false;
          liveOffsetPending = true;
        },
      );
    const setLiveSource = (positionMs: number) => {
      player.src(liveSourceAt(positionMs));
    };

    player.on("loadedmetadata", () => {
      // Ignore delayed metadata from requests already abandoned during recovery.
      if (recovering && pendingRecovery === null) return;
      // Ignore metadata from the source replaced by a quality switch.
      if (
        pendingSwitch !== null &&
        !pendingSwitch.loaded &&
        !mediaSourceIs(pendingSwitch.src)
      ) {
        return;
      }
      attempt = { ...attempt, state: "ready" };
      const videoWidth = player.videoWidth();
      const videoHeight = player.videoHeight();
      if (videoWidth > 0 && videoHeight > 0) {
        latest.current.onAspectRatio?.(videoWidth / videoHeight);
      }
      setHoldControlBar(false);
      if (pendingRecovery !== null) {
        finishRecovery();
      } else if (pendingSwitch !== null) {
        finishSwitch();
      } else if (!resumeApplied && initialPositionMs > 0) {
        resumeApplied = true;
        if (attempt.route === "direct") {
          resumeSeekPending = true;
          player.currentTime(initialPositionMs / 1000);
          latest.current.onPosition(initialPositionMs);
        } else {
          // Copy-mode transcoding starts at the preceding keyframe; the offset bridge supplies logical time.
          reportPosition();
        }
      }
      // Direct positioning waits for seeked; transcode positioning waits for the offset report.
      metadataAccepted = true;
      markPositioned();
    });
    player.on("seeked", () => {
      if (!resumeSeekPending) return;
      resumeSeekPending = false;
      markPositioned();
    });
    player.on("loadstart", () => {
      if (pendingSwitch !== null && mediaSourceIs(pendingSwitch.src)) {
        pendingSwitch.loaded = true;
      }
    });
    player.on("timeupdate", () => {
      const position = reportPosition();
      if (playedSinceRecoveryMs === null) return;
      if (recovering || player.paused()) {
        lastTickMs = null;
        return;
      }
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
      if (recovering) return;
      attempt = { ...attempt, state: "playing", playIntended: true };
      played = true;
      setStatus({ playing: true, ended: false });
      // Replay may emit play before timeupdate; publish the confirmed position before onPlay.
      if (status.positioned) reportPosition();
      latest.current.onPlay?.();
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
      lastTickMs = null;
      if (recovering || switchingSource || player.error() !== null) return;
      attempt = { ...attempt, playIntended: false };
      setStatus({ playing: false, loading: false });
      latest.current.onProgress(reportPosition(), true);
    });
    player.on("ended", () => {
      feedStall("clear");
      setStatus({ playing: false, loading: false, ended: true });
      latest.current.onProgress(reportPosition(), true);
    });
    const setDirectSource = () => {
      subtitles.setOffset(0);
      liveOffsetPending = false;
      player.src({ src: streamUrl(source.id), type: directContentType(source) });
    };

    const clearRecoveryTimers = () => {
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
      if (reloadDeadline !== undefined) window.clearTimeout(reloadDeadline);
      reloadDeadline = undefined;
      pendingRecovery = null;
      seekDuringReloadMs = null;
    };

    const fail = (kind: PlaybackFailureKind, position: number) => {
      clearRecoveryTimers();
      recovering = false;
      switchingSource = false;
      attempt = { ...attempt, state: "failed" };
      feedStall("clear");
      setStatus({ playing: false, loading: false, reconnecting: false });
      latest.current.onError(position, kind);
      // Clear video.js error state so controls remain available beneath the page's failure overlay.
      player.error(null);
      setHoldControlBar(false);
      // Transcode switches clear vjs-has-started; restore it so controls remain usable on failure.
      player.hasStarted(true);
    };

    // Restore direct-play position after reload; transcodes need another seek only if the user moved during reload.
    const finishRecovery = () => {
      const seekMs = seekDuringReloadMs;
      clearRecoveryTimers();
      recovering = false;
      resumeApplied = true;
      const targetMs = attempt.route === "direct" ? attempt.logicalPositionMs : seekMs;
      if (targetMs !== null) {
        player.currentTime(targetMs / 1000);
        latest.current.onPosition(targetMs);
      }
      playedSinceRecoveryMs = 0;
      lastTickMs = null;
      setStatus({ reconnecting: false });
      if (attempt.playIntended) {
        setStatus({ loading: true });
        void player.play()?.catch(() => setStatus({ loading: false }));
      }
    };

    // Advance retries at the deadline even when a stalled request emits no error.
    const reload = () => {
      reconnectTimer = undefined;
      if (player.isDisposed()) return;
      const generation = ++recoveryGeneration;
      const positionMs = attempt.logicalPositionMs;
      pendingRecovery = { generation };
      seekDuringReloadMs = null;
      reloadDeadline = window.setTimeout(() => {
        reloadDeadline = undefined;
        if (player.isDisposed() || pendingRecovery?.generation !== generation) return;
        pendingRecovery = null;
        scheduleReconnect(attempt.logicalPositionMs);
      }, reloadTimeoutMs);
      if (attempt.route === "direct") setDirectSource();
      else setLiveSource(positionMs);
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

    // Going online or pressing play bypasses the retry delay.
    function reconnectNow() {
      if (reconnectTimer === undefined) return;
      window.clearTimeout(reconnectTimer);
      reload();
    }
    window.addEventListener("online", reconnectNow);

    const handleFailure = (
      kind: PlaybackFailureKind,
      position: number,
      allowFallback = true,
    ) => {
      if (kind === "network") {
        scheduleReconnect(position);
        return;
      }
      // Fall back to transcoding once for unsupported direct formats, never for unavailable files.
      const fallback = allowFallback
        ? fallbackToTranscode(attempt, position, attempt.playIntended)
        : null;
      if (fallback === null) {
        fail(kind, position);
        return;
      }
      recovering = false;
      switchingSource = true;
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

    // video.js delays forwarding sources to the tech layer; compare currentSrc to reject old metadata when available.
    const mediaSourceIs = (src: string) => {
      const tech = (
        player as unknown as { tech?(safety: boolean): { el?(): unknown } | undefined }
      ).tech?.(true);
      const element = tech?.el?.();
      if (!(element instanceof HTMLMediaElement)) return true;
      return (
        new URL(element.currentSrc || src, window.location.href).href ===
        new URL(src, window.location.href).href
      );
    };

    // Replace the source at the logical position while preserving playback intent, including during recovery.
    const changeQuality = (quality: PlaybackQuality) => {
      if (player.isDisposed()) return;
      // Save explicit choices even when unchanged so selecting original replaces an unavailable preference.
      writePlaybackQuality(quality);
      if (quality === attempt.quality) return;
      if (attempt.state === "failed") {
        attempt = { ...attempt, quality };
        setQuality(quality);
        renderQualityMenu();
        return;
      }
      const busy = recovering || switchingSource || pendingSwitch !== null;
      const playIntended = busy ? attempt.playIntended : !player.paused();
      const positionMs = recovering ? attempt.logicalPositionMs : logicalPositionMs();
      probe?.abort();
      probe = undefined;
      clearRecoveryTimers();
      recoveryGeneration += 1;
      recovering = false;
      playedSinceRecoveryMs = null;
      lastTickMs = null;
      resumeApplied = true;
      const rate = pendingSwitch?.rate ?? player.playbackRate() ?? 1;
      attempt = switchQuality(
        attempt,
        quality,
        source.playable,
        positionMs,
        playIntended,
      );
      switchingSource = true;
      setRoute(attempt.route);
      setQuality(attempt.quality);
      renderQualityMenu();
      setStatus({ reconnecting: false, ended: false, loading: true });
      latest.current.onPosition(attempt.logicalPositionMs);
      if (attempt.route === "direct") {
        pendingSwitch = { src: streamUrl(source.id), loaded: false, rate };
        setDirectSource();
      } else {
        const next = liveSourceAt(attempt.sourceOffsetMs);
        pendingSwitch = { src: next.src, loaded: false, rate };
        player.src(next);
      }
    };
    player.on(qualitySelectEvent, (_event: unknown, selection?: QualitySelection) => {
      if (selection !== undefined) changeQuality(selection.quality);
    });

    // Direct sources start at zero and need a seek; transcode URLs already encode the position.
    const finishSwitch = () => {
      const pending = pendingSwitch;
      if (pending === null) return;
      pendingSwitch = null;
      switchingSource = false;
      player.playbackRate(pending.rate);
      if (attempt.route === "direct") {
        player.currentTime(attempt.logicalPositionMs / 1000);
        latest.current.onPosition(attempt.logicalPositionMs);
      } else {
        reportPosition();
      }
      if (attempt.playIntended) {
        void player.play()?.catch(() => setStatus({ loading: false }));
      } else {
        setStatus({ loading: false });
      }
    };

    // Retry network failures, transcode unsupported formats, and probe reachability for ambiguous errors.
    player.on("error", () => {
      const code = (player.error() as { code?: number } | null)?.code;
      const position = logicalPositionMs();
      const playIntended = attempt.playIntended || !player.paused();
      const generation = ++recoveryGeneration;
      probe?.abort();
      probe = undefined;
      clearRecoveryTimers();
      // Handle switch failures from the selected source's position.
      pendingSwitch = null;
      switchingSource = false;
      attempt = {
        ...updatePosition(attempt, position),
        state: "loading",
        playIntended,
      };
      // Keep controls visible during checks and reloads so recovery can capture user intent.
      recovering = true;
      feedStall("recovering");
      playedSinceRecoveryMs = null;
      lastTickMs = null;
      player.error(null);

      const errorClass = classifyMediaError(code);
      if (errorClass !== "ambiguous") {
        handleFailure(errorClass, position);
        return;
      }
      setStatus({ loading: true });
      const controller = new AbortController();
      probe = controller;
      const direct = attempt.route === "direct";
      void probeMediaSource(direct ? streamUrl(source.id) : null, controller.signal).then(
        (result) => {
          if (controller.signal.aborted || player.isDisposed()) return;
          if (generation !== recoveryGeneration) return;
          probe = undefined;
          if (result === "unreachable")
            handleFailure("network", attempt.logicalPositionMs);
          // Unavailable direct files cannot be repaired by switching to transcoding.
          else
            handleFailure(
              "source",
              attempt.logicalPositionMs,
              !direct || result === "ok",
            );
        },
      );
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
      if (stallTimer !== undefined) window.clearTimeout(stallTimer);
      window.removeEventListener("online", reconnectNow);
      clearRecoveryTimers();
      recovering = false;
      recoveryGeneration += 1;
      probe?.abort();
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
      subtitles.dispose();
      subtitlesRef.current = null;
      attempt = { ...attempt, state: "disposed" };
      playerRef.current = null;
      if (!player.isDisposed()) player.dispose();
    };
  }, [video.id, video.probeState, video.durationMs, video.playable]);

  // Late subtitle responses replace tracks without recreating the player.
  const subtitleList = props.subtitles ?? noSubtitles;
  useEffect(() => {
    subtitlesRef.current?.replace(video.id, subtitleList);
  }, [video.id, subtitleList]);

  // Reattach previews when generation completes, even with the same URL, to discard failed layout requests.
  // Chapters provide title/time previews even without thumbnail URLs.
  //
  const seekThumbnailUrl = video.seekThumbnailUrl;
  const seekThumbnailState = video.seekThumbnailState;
  const durationMs = video.durationMs;
  const chapters = usePlayerChapters(video);
  useEffect(() => {
    const host = hostRef.current;
    if (!playerReady || host === null) return;
    if (seekThumbnailUrl === undefined && chapters.length === 0) return;
    if (durationMs === undefined || durationMs <= 0) return;
    const progress = host.querySelector<HTMLElement>(".vjs-progress-holder");
    const progressControl = host.querySelector<HTMLElement>(".vjs-progress-control");
    if (progress === null || progressControl === null) return;
    return attachSeekPreview(
      progress,
      {
        durationMs,
        thumbnailUrl: seekThumbnailUrl,
        chapterTitleAt:
          chapters.length === 0
            ? undefined
            : (positionMs) => chapterTitleAt(chapters, positionMs),
      },
      progressControl,
    );
  }, [chapters, durationMs, playerReady, seekThumbnailUrl, seekThumbnailState]);

  // Late chapter metadata replaces tracks and marks without recreating the player.
  useEffect(() => {
    const host = hostRef.current;
    const player = playerRef.current;
    if (!playerReady || host === null || player === null || chapters.length === 0) return;
    if (durationMs === undefined || durationMs <= 0) return;
    const detachTrack = attachChaptersTrack(
      player as unknown as ChaptersPlayer,
      chapters,
    );
    const progress = host.querySelector<HTMLElement>(".vjs-progress-holder");
    const detachMarks =
      progress === null ? undefined : attachChapterMarks(progress, chapters, durationMs);
    return () => {
      detachMarks?.();
      if (!player.isDisposed()) detachTrack();
    };
  }, [chapters, durationMs, playerReady]);

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
            quality={quality}
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

/** Preserve the chapter array reference when refreshed contents are equal to avoid reinstalling tracks and marks. */
function usePlayerChapters(video: Video): readonly PlayerChapter[] {
  const chapters = video.mediaInfo?.chapters;
  const { durationMs } = video;
  const key = useMemo(
    () => JSON.stringify(playerChapters(chapters, durationMs)),
    [chapters, durationMs],
  );
  return useMemo(() => JSON.parse(key) as PlayerChapter[], [key]);
}

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

/** Use a clickable popover so touch and keyboard users can read why transcoding is active. */
function TranscodeIndicator({
  quality,
  container,
  onOpenChange,
}: {
  quality: PlaybackQuality;
  container: HTMLElement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const c = t.player.controls;
  const label = quality === "original" ? c.transcoding : c.transcodingTo(quality);
  const detail =
    quality === "original" ? c.transcodingDetail : c.transcodingToDetail(quality);
  return (
    <Popover onOpenChange={onOpenChange}>
      {/* Override video.js button styles with !; this is a registered control-bar design exception. */}
      <PopoverTrigger className="inline-flex! items-center gap-1 rounded-sm px-1 text-xs! leading-4! whitespace-nowrap text-muted-foreground! transition-colors! hover:text-foreground!">
        <Info className="size-4 shrink-0" aria-hidden="true" />
        {/* Below 32rem, keep only the icon so chapter and subtitle controls still fit. */}
        <span className="@max-[32rem]:sr-only">{label}</span>
      </PopoverTrigger>
      <PopoverContent container={container} align="end">
        {detail}
      </PopoverContent>
    </Popover>
  );
}

function directContentType(video: Video): string {
  return video.container === "webm" ? "video/webm" : "video/mp4";
}

export function canStartPlayback(video: Video): boolean {
  return createPlaybackAttempt(video, 0) !== null;
}
