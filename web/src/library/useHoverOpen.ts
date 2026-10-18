import { type PointerEvent, useCallback, useEffect, useRef, useState } from "react";

export const HOVER_OPEN_DELAY_MS = 400;
// Allow the pointer to cross the gap between trigger and popover.
export const HOVER_CLOSE_DELAY_MS = 200;

export type HoverOpenState = "closed" | "hover" | "press";

export interface HoverHandlers {
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: (event: PointerEvent<HTMLElement>) => void;
}

export interface HoverOpen {
  open: boolean;
  // Hover opening neither moves nor restores focus.
  openedByHover: boolean;
  // Retain the last opening mode after closure for onCloseAutoFocus.
  lastOpenedByHover: () => boolean;
  setOpen: (open: boolean) => void;
  triggerHandlers: HoverHandlers;
  contentHandlers: HoverHandlers;
}

/** Mouse hover opens after 400 ms and closes after a 200 ms gap. Press-open popovers survive pointer leave; disabled popovers stay closed. */
export function useHoverOpen(enabled = true): HoverOpen {
  const [state, setState] = useState<HoverOpenState>("closed");
  const stateRef = useRef<HoverOpenState>("closed");
  const lastOpenRef = useRef<HoverOpenState>("closed");
  const openTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const update = useCallback((next: HoverOpenState) => {
    stateRef.current = next;
    if (next !== "closed") lastOpenRef.current = next;
    setState(next);
  }, []);

  const clearOpenTimer = useCallback(() => {
    if (openTimer.current !== null) {
      clearTimeout(openTimer.current);
      openTimer.current = null;
    }
  }, []);

  const clearCloseTimer = useCallback(() => {
    if (closeTimer.current !== null) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  }, []);

  const clearTimers = useCallback(() => {
    clearOpenTimer();
    clearCloseTimer();
  }, [clearOpenTimer, clearCloseTimer]);

  useEffect(() => clearTimers, [clearTimers]);

  useEffect(() => {
    if (!enabled) {
      clearTimers();
      update("closed");
    }
  }, [enabled, clearTimers, update]);

  const setOpen = useCallback(
    (open: boolean) => {
      clearTimers();
      update(open ? "press" : "closed");
    },
    [clearTimers, update],
  );

  const enter = useCallback(
    (event: PointerEvent<HTMLElement>, onTrigger: boolean) => {
      if (event.pointerType !== "mouse" || !enabled) return;
      clearCloseTimer();
      if (!onTrigger || stateRef.current !== "closed" || openTimer.current !== null) {
        return;
      }
      openTimer.current = setTimeout(() => {
        openTimer.current = null;
        if (stateRef.current === "closed") update("hover");
      }, HOVER_OPEN_DELAY_MS);
    },
    [clearCloseTimer, enabled, update],
  );

  const leave = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (event.pointerType !== "mouse") return;
      clearOpenTimer();
      if (stateRef.current !== "hover") return;
      clearCloseTimer();
      closeTimer.current = setTimeout(() => {
        closeTimer.current = null;
        if (stateRef.current === "hover") update("closed");
      }, HOVER_CLOSE_DELAY_MS);
    },
    [clearCloseTimer, clearOpenTimer, update],
  );

  const open = enabled && state !== "closed";
  return {
    open,
    openedByHover: open && state === "hover",
    lastOpenedByHover: () => lastOpenRef.current === "hover",
    setOpen,
    triggerHandlers: {
      onPointerEnter: (event) => enter(event, true),
      onPointerLeave: leave,
    },
    contentHandlers: {
      onPointerEnter: (event) => enter(event, false),
      onPointerLeave: leave,
    },
  };
}
