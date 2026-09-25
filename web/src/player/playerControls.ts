/**
 * PlayerControls は、video.js の外（キーボード・タッチ用の中央操作・再生終了の層）から
 * プレイヤーを動かす入口である。
 *
 * 位置と速度はどれも video.js の `currentTime`・`playbackRate` を通す。変換して再生する
 * 経路では `liveOffset.ts` の仲立ちがそこに挟まるので、操作は元動画の時間軸（論理上の
 * 再生位置）に対して効く（plan の Structural Decisions 9）。
 */
export interface PlayerControls {
  togglePlay(): void;
  play(): void;
  /** seekTo は論理上の再生位置を seconds 秒の位置にする。 */
  seekTo(seconds: number): void;
  /** restart は先頭から再生し直す。 */
  restart(): void;
  toggleMute(): void;
  toggleFullscreen(): void;
  isFullscreen(): boolean;
  /** menuOpen は、Esc を自分で扱う吹き出しやメニューが開いているかを返す。 */
  menuOpen(): boolean;
  /** wake は操作バーを見せ続ける（video.js の user-active にする）。 */
  wake(): void;
}

/** ControllablePlayer は、ここで使う video.js の Player の部分である。 */
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

/**
 * rateMenuOpen は、video.js の操作バーのメニュー（再生速度）が開いているかを返す。
 * 押して開いたメニューには `vjs-lock-showing`、ポイントして開いたメニューのボタンには
 * `vjs-hover` が付く。開いている間の Esc はメニューを閉じるだけにする。
 */
export function rateMenuOpen(root: ParentNode): boolean {
  return (
    root.querySelector(".vjs-menu.vjs-lock-showing") !== null ||
    root.querySelector(".vjs-menu-button-popup.vjs-hover") !== null
  );
}

export function createPlayerControls(
  player: ControllablePlayer,
  menuOpen: () => boolean,
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
    menuOpen,
    wake() {
      player.userActive(true);
    },
  };
}
