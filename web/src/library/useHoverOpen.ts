import { type PointerEvent, useCallback, useEffect, useRef, useState } from "react";

/** マウスを「+N」に留めてから一覧を開くまでの時間（010 の hover プレビューと同じ）。 */
export const HOVER_OPEN_DELAY_MS = 400;
/** 「+N」と一覧の両方から離れてから閉じるまでの時間（間の 6px を渡る猶予）。 */
export const HOVER_CLOSE_DELAY_MS = 200;

/**
 * HoverOpenState は一覧の開き方である。hover は留めたマウスで開いた状態、press は
 * 押して（タッチ・ペン・クリック・Enter・Space）開いた状態である。
 */
export type HoverOpenState = "closed" | "hover" | "press";

/** HoverHandlers は「+N」と一覧の両方に付けるポインタの受け口である。 */
export interface HoverHandlers {
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: (event: PointerEvent<HTMLElement>) => void;
}

/** HoverOpen は useHoverOpen の結果である。 */
export interface HoverOpen {
  open: boolean;
  /** 留めたマウスで開いたか。開いてもフォーカスを動かさず、閉じても返さない。 */
  openedByHover: boolean;
  /**
   * いま開いている、または最後に開いていた一覧が留めたマウスで開いたか。閉じたあとに
   * 走る onCloseAutoFocus で、フォーカスを返すかを決めるのに使う。
   */
  lastOpenedByHover: () => boolean;
  /**
   * Popover の onOpenChange に渡す。押す・Esc・外を押す・チップを押すはここを通り、
   * 開くなら押して開いた状態、閉じるならどの開き方でも閉じる。
   */
  setOpen: (open: boolean) => void;
  /** 「+N」（トリガー）に付ける。 */
  triggerHandlers: HoverHandlers;
  /** 一覧（PopoverContent）に付ける。 */
  contentHandlers: HoverHandlers;
}

/**
 * useHoverOpen は Popover の open をポインタから動かす（specs/041-tag-overflow-list/
 * research.md R-1、ui-design.md「Interaction」）。pointerType が mouse のポインタが
 * トリガーに 400ms 留まると開き、トリガーと一覧の両方から離れて 200ms たつと閉じる。
 * その間に戻れば開いたままにする。touch と pen のポインタでは開かない（幅ではなく
 * pointerType で分ける。library-ui.md「Width breakpoints in CSS」）。押して開いた
 * 一覧は、ポインタが離れても閉じない。
 *
 * enabled が false のあいだ（隠れるタグが無い、選択中）は閉じたままにする。
 */
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

  // ポインタが「+N」か一覧に入った。閉じる待ちを取り消し、閉じていれば開く待ちを始める。
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

  // ポインタが「+N」か一覧から出た。開く待ちを取り消し、留めて開いた一覧なら閉じる待ちを
  // 始める（もう一方に 200ms 以内に入れば enter が取り消す）。
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
