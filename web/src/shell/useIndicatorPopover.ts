import {
  type FocusEvent,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

const POINTER_CLOSE_DELAY_MS = 100;
const focusable = 'button, [href], input, [tabindex]:not([tabindex="-1"])';

function inside(container: HTMLElement | null, target: EventTarget | null): boolean {
  return container !== null && target instanceof Node && container.contains(target);
}

/**
 * Open and close the bottom-right indicator's summary from pointer, focus and key events, not from
 * hover or focus state, so the focus Radix restores on close cannot reopen it. `Tab` from the
 * trigger enters a summary that holds a control, and the summary stays open while focus is in it.
 */
export function useIndicatorPopover() {
  const [open, setOpen] = useState(false);
  const pointerCloseTimer = useRef<number | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const pointerInside = useRef(false);
  // After closing while focused, require focus to leave before it can reopen the summary.
  const focusOpenSuppressed = useRef(false);

  const clearPointerCloseTimer = useCallback(() => {
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
  }, []);

  useEffect(() => clearPointerCloseTimer, [clearPointerCloseTimer]);

  /** Forget every open reason, for an indicator that is no longer shown. */
  const reset = useCallback(() => {
    clearPointerCloseTimer();
    pointerInside.current = false;
    focusOpenSuppressed.current = false;
    setOpen(false);
  }, [clearPointerCloseTimer]);

  const close = () => {
    clearPointerCloseTimer();
    focusOpenSuppressed.current = document.activeElement === trigger.current;
    setOpen(false);
  };
  const enterPointerArea = () => {
    clearPointerCloseTimer();
    pointerInside.current = true;
    setOpen(true);
  };
  const leavePointerArea = () => {
    clearPointerCloseTimer();
    pointerCloseTimer.current = window.setTimeout(() => {
      pointerInside.current = false;
      close();
    }, POINTER_CLOSE_DELAY_MS);
  };
  const focusTrigger = () => {
    if (!focusOpenSuppressed.current) setOpen(true);
  };
  const blurTrigger = (event: FocusEvent<HTMLElement>) => {
    focusOpenSuppressed.current = false;
    // Let pointer handling close while the pointer remains over the trigger or summary.
    if (!pointerInside.current && !inside(content.current, event.relatedTarget)) {
      setOpen(false);
    }
  };
  const blurContent = (event: FocusEvent<HTMLElement>) => {
    const target = event.relatedTarget;
    if (inside(content.current, target) || target === trigger.current) return;
    if (!pointerInside.current) setOpen(false);
  };
  // The summary is portalled, so the next and previous focus stops are set here.
  const keyDownTrigger = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Tab" || event.shiftKey || !open) return;
    const first = content.current?.querySelector<HTMLElement>(focusable);
    if (first === null || first === undefined) return;
    event.preventDefault();
    first.focus();
  };
  const keyDownContent = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Tab" || !event.shiftKey) return;
    const first = content.current?.querySelector<HTMLElement>(focusable);
    if (first === undefined || first !== document.activeElement) return;
    event.preventDefault();
    trigger.current?.focus();
  };

  return {
    open,
    setOpen,
    trigger,
    content,
    reset,
    close,
    enterPointerArea,
    leavePointerArea,
    focusTrigger,
    blurTrigger,
    blurContent,
    keyDownTrigger,
    keyDownContent,
  };
}
