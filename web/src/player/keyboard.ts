import { useEffect, useLayoutEffect, useRef } from "react";

import type { PlayerControls } from "./playerControls";

/** Listen in window capture because video.js stops bubbling keys; leave each control's own shortcuts untouched. */

export type ShortcutAction =
  "togglePlay" | "fullscreen" | "theater" | "mute" | "subtitles" | "start" | "close";

function isEditable(element: Element): boolean {
  if (element instanceof HTMLElement && element.isContentEditable) return true;
  return element.closest("input, textarea, select, [contenteditable='true']") !== null;
}

const spaceOwners =
  "button, a[href], summary, [role='button'], [role='link'], [role='slider'], [role='checkbox'], [role='switch'], [role='menuitem'], [role='menuitemradio'], [role='menuitemcheckbox'], [role='option'], [role='tab']";

/** Popover content owns its keys; allow Escape through only to check whether the page may close. */
const popperContent = "[data-radix-popper-content-wrapper]";

export function shortcutFor(event: KeyboardEvent): ShortcutAction | null {
  if (event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return null;
  const target = event.target instanceof Element ? event.target : null;
  if (target !== null && isEditable(target)) return null;
  // The side panel beside the page owns its keys, Escape included.
  if (target?.closest("[data-slot='side-panel']") != null) return null;
  const inPopover = target?.closest(popperContent) != null;
  if (inPopover && event.key !== "Escape" && event.key !== "Esc") return null;

  switch (event.key) {
    case " ":
    case "Spacebar":
      if (event.shiftKey) return null;
      return target?.closest(spaceOwners) != null ? null : "togglePlay";
    case "f":
    case "F":
      return "fullscreen";
    case "t":
    case "T":
      return "theater";
    case "m":
    case "M":
      return "mute";
    case "c":
    case "C":
      return "subtitles";
    case "0":
      return event.shiftKey ? null : "start";
    case "Escape":
    case "Esc":
      return "close";
    default:
      return null;
  }
}

function isDocumentFullscreen(): boolean {
  return typeof document !== "undefined" && document.fullscreenElement != null;
}

/** Any open popover or menu owns Escape, including controls outside the player; tooltips do not. */
function isPopoverOpen(): boolean {
  return (
    typeof document !== "undefined" &&
    document.querySelector(
      '[data-radix-popper-content-wrapper] [role="dialog"], [data-radix-popper-content-wrapper] [role="menu"]',
    ) !== null
  );
}

/** An open modal dialog owns every key, so neither Escape nor the player keys reach the page under it. */
function isModalOpen(): boolean {
  return (
    typeof document !== "undefined" &&
    document.querySelector(
      '[data-slot="dialog-content"], [data-slot="alert-dialog-content"]',
    ) !== null
  );
}

/** Fullscreen and open menus own Escape; without a player only Escape and T work. Suspension leaves all keys to the editor. */
export function useKeyboardShortcuts(
  controls: PlayerControls | null,
  onClose: () => void,
  suspended = false,
  onTheater?: () => void,
): void {
  const latest = useRef({ controls, onClose, suspended, onTheater });
  // Update at layout commit; a passive effect could briefly treat Escape as having no player.
  useLayoutEffect(() => {
    latest.current = { controls, onClose, suspended, onTheater };
  }, [controls, onClose, suspended, onTheater]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || latest.current.suspended || isModalOpen()) return;
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
      // Theater mode changes the page layout, so it works for a page without a player too.
      if (action === "theater") {
        if (latest.current.onTheater === undefined) return;
        event.preventDefault();
        latest.current.onTheater();
        player?.wake();
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
        case "subtitles":
          player.toggleSubtitles();
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
