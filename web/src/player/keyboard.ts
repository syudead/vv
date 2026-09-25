import { useEffect, useLayoutEffect, useRef } from "react";

import type { PlayerControls } from "./playerControls";

/**
 * 画面全体のキーボード操作（要件 7、plan の Structural Decisions 9）。
 *
 * video.js の操作バーの部品は、自分が受けたキーの伝播を止める（Tab 以外）。画面のどこに
 * フォーカスがあっても効くように、`window` の捕捉段階で受ける。そのため、部品自身の
 * 操作になるキーはここで見送る。
 */

export type ShortcutAction = "togglePlay" | "fullscreen" | "mute" | "start" | "close";

/** 入力欄など、文字の入力を受ける要素。 */
function isEditable(element: Element): boolean {
  if (element instanceof HTMLElement && element.isContentEditable) return true;
  return element.closest("input, textarea, select, [contenteditable='true']") !== null;
}

/** Space で自分の操作をする要素（ボタン・リンク・スライダー・メニュー項目）。 */
const spaceOwners =
  "button, a[href], summary, [role='button'], [role='link'], [role='slider'], [role='checkbox'], [role='switch'], [role='menuitem'], [role='menuitemradio'], [role='menuitemcheckbox'], [role='option'], [role='tab']";

/** shortcutFor は押されたキーが起こす操作を返す。この画面が扱わないキーは null。 */
export function shortcutFor(event: KeyboardEvent): ShortcutAction | null {
  if (event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return null;
  const target = event.target instanceof Element ? event.target : null;
  if (target !== null && isEditable(target)) return null;

  switch (event.key) {
    case " ":
    case "Spacebar":
      if (event.shiftKey) return null;
      return target?.closest(spaceOwners) != null ? null : "togglePlay";
    case "f":
    case "F":
      return "fullscreen";
    case "m":
    case "M":
      return "mute";
    case "0":
      return event.shiftKey ? null : "start";
    case "Escape":
    case "Esc":
      return "close";
    default:
      return null;
  }
}

/** isDocumentFullscreen は、ブラウザが全画面を表示しているかを返す。 */
function isDocumentFullscreen(): boolean {
  return typeof document !== "undefined" && document.fullscreenElement != null;
}

/**
 * isPopoverOpen は、画面のどこかで吹き出し（Radix の Popover）が開いているかを返す。
 * 取り込み状況の概要のように、再生画面の外の部品が開く吹き出しも含む。そのときの Esc は
 * 吹き出しを閉じるためのもので、画面を閉じてはいけない。ツールチップ（role="tooltip"）は含めない。
 */
function isPopoverOpen(): boolean {
  return (
    typeof document !== "undefined" &&
    document.querySelector('[data-radix-popper-content-wrapper] [role="dialog"]') !== null
  );
}

/**
 * useKeyboardShortcuts は Space・F・M・0・Esc を画面全体で受ける。
 *
 * - プレイヤーが無い（取り込み中・読み取り失敗など）ときは、Esc だけが効く。
 * - 全画面中の Esc は何もしない。全画面の解除はブラウザが行う。
 * - 再生速度のメニューや吹き出しが開いているときの Esc は、それを閉じるだけにする。
 * - Esc で呼ぶのは `onClose` なので、呼び出し側が別の操作を渡せる。再生画面は自動再生の
 *   予告が出ている間、画面を閉じる代わりに予告の取り消しを渡す。
 */
export function useKeyboardShortcuts(
  controls: PlayerControls | null,
  onClose: () => void,
): void {
  const latest = useRef({ controls, onClose });
  // 描画を確定した時点で最新の値にする。useEffect だと、プレイヤーの操作を
  // 描いてから反映するまでの間の Esc が「プレイヤー無し」として画面を閉じる。
  useLayoutEffect(() => {
    latest.current = { controls, onClose };
  }, [controls, onClose]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      const action = shortcutFor(event);
      if (action === null) return;
      const { controls: player, onClose: close } = latest.current;

      if (action === "close") {
        if (isDocumentFullscreen() || player?.isFullscreen() === true) return;
        if (player?.menuOpen() === true || isPopoverOpen()) return;
        event.preventDefault();
        close();
        return;
      }
      if (player === null) return;
      event.preventDefault();
      switch (action) {
        case "togglePlay":
          player.togglePlay();
          break;
        case "fullscreen":
          player.toggleFullscreen();
          break;
        case "mute":
          player.toggleMute();
          break;
        case "start":
          player.seekTo(0);
          break;
      }
      player.wake();
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, []);
}
