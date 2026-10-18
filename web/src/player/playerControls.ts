/** External controls use video.js time/rate APIs so liveOffset applies operations on the original video's timeline. */
export interface PlayerControls {
  togglePlay(): void;
  play(): void;
  pause(): void;
  seekTo(seconds: number): void;
  restart(): void;
  toggleMute(): void;
  toggleFullscreen(): void;
  isFullscreen(): boolean;
  /** Toggle subtitles when available; otherwise do nothing. */
  toggleSubtitles(): void;
  menuOpen(): boolean;
  wake(): void;
  /** Logical milliseconds, matching the saved progress position used by thumbnail capture. */
  positionMs(): number;
}

export interface ControllablePlayer {
  paused(): boolean;
  play(): Promise<void> | undefined;
  pause(): void;
  currentTime(seconds?: number): number | undefined;
  duration(): number | undefined;
  muted(value?: boolean): boolean | undefined;
  isFullscreen(): boolean;
  requestFullscreen(): unknown;
  exitFullscreen(): unknown;
  userActive(value?: boolean): unknown;
}

/** video.js marks clicked menus with vjs-lock-showing and hovered buttons with vjs-hover; these menus own Escape. */
export function controlBarMenuOpen(root: ParentNode): boolean {
  return (
    root.querySelector(".vjs-menu.vjs-lock-showing") !== null ||
    root.querySelector(".vjs-menu-button-popup.vjs-hover") !== null
  );
}

export function createPlayerControls(
  player: ControllablePlayer,
  menuOpen: () => boolean,
  toggleSubtitles: () => void = () => undefined,
  positionMs: () => number = () => (player.currentTime() ?? 0) * 1000,
): PlayerControls {
  const play = () => {
    void player.play()?.catch(() => undefined);
  };
  return {
    togglePlay() {
      if (player.paused()) play();
      else player.pause();
    },
    play,
    pause() {
      player.pause();
    },
    seekTo(seconds) {
      player.currentTime(Math.max(0, seconds));
    },
    restart() {
      player.currentTime(0);
      play();
    },
    toggleMute() {
      player.muted(!(player.muted() ?? false));
    },
    toggleFullscreen() {
      if (player.isFullscreen()) void player.exitFullscreen();
      else void player.requestFullscreen();
    },
    isFullscreen: () => player.isFullscreen(),
    toggleSubtitles,
    menuOpen,
    wake() {
      player.userActive(true);
    },
    positionMs,
  };
}
