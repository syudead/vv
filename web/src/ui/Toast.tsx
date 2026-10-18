import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { toast as sonner } from "sonner";

import { t, type UiText } from "../i18n";
import { Button } from "./shadcn/button";
import { Toaster } from "./shadcn/sonner";
import { Spinner } from "./shadcn/spinner";

// Show up to three list notifications and one at a time in playback, retaining the queue across navigation.
// Keep only the latest action notification; its default duration is eight seconds.
// （web/registry/rules/components.md「Sonner」）。

const visibleLimit = { default: 3, playback: 1 } as const;

export const toastDuration = 2800;

export const actionToastDuration = 8000;

type Placement = keyof typeof visibleLimit;

/** Disable the action and show progress until run settles, then dismiss the notification. */
export interface ToastAction {
  label: UiText;
  run: () => Promise<void>;
}

export interface ToastOptions {
  action?: ToastAction;
  /** Milliseconds; defaults to eight seconds with an action, 2.8 seconds otherwise. */
  duration?: number;
}

/** Returns a dismissal function. */
export type ShowToast = (message: UiText, options?: ToastOptions) => () => void;

const ToastContext = createContext<ShowToast>(() => () => undefined);

export function useToast(): ShowToast {
  return useContext(ToastContext);
}

interface ToastItem {
  /** Sonner ID changes when redisplayed after placement changes. */
  id: string;
  /** Stable dismissal handle survives redisplay. */
  handle: string;
  message: UiText;
  options: ToastOptions;
  /** Pending action state survives redisplay. */
  busy: { running: boolean };
}

/**
 * Reject duplicate actions and retain pending state across placement changes; dismiss the current
 * instance after completion.
 */
function ToastActionButton({
  item,
  action,
  onDone,
}: {
  item: ToastItem;
  action: ToastAction;
  onDone: () => void;
}) {
  const [running, setRunning] = useState(item.busy.running);
  return (
    <Button
      type="button"
      size="sm"
      variant="secondary"
      className="ml-auto"
      disabled={running}
      aria-busy={running || undefined}
      onClick={() => {
        item.busy.running = true;
        setRunning(true);
        void action.run().finally(onDone);
      }}
    >
      {running && <Spinner aria-hidden="true" />}
      {action.label}
    </Button>
  );
}

let sequence = 0;

function nextToastId(): string {
  sequence += 1;
  return `vv-toast-${String(sequence)}`;
}

const wideQuery = "(min-width: 64rem)";

function subscribeWide(onChange: () => void): () => void {
  if (typeof window.matchMedia !== "function") return () => undefined;
  const query = window.matchMedia(wideQuery);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

function readWide(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideQuery).matches;
}

/** Sonner positioning cannot use CSS breakpoints, so observe width only for notification placement. */
function useWide(): boolean {
  return useSyncExternalStore(subscribeWide, readWide, () => false);
}

/**
 * Playback uses top-center to avoid its close button; wide lists use bottom-center near selection
 * actions. Sonner restarts visible notifications when placement changes.
 */
function toasterPlacement(placement: Placement, wide: boolean) {
  if (placement === "playback") {
    return {
      position: "top-center",
      offset: { top: "0.375rem" },
      // Reserve four rem on each side to avoid the playback close button.
      mobileOffset: { top: "0.375rem", left: "4rem", right: "4rem" },
    } as const;
  }
  if (wide) {
    return {
      position: "bottom-center",
      // Place above the sticky selection bar.
      offset: { bottom: "calc(var(--spacing-selection-bar) + 2rem)" },
    } as const;
  }
  const top = "calc(var(--spacing-navbar) + 0.5rem)";
  return {
    position: "top-right",
    offset: { top, right: "0.75rem" },
    mobileOffset: { top, left: "0.75rem", right: "0.75rem" },
  } as const;
}

export function ToastProvider({
  children,
  placement = "default",
}: {
  children: ReactNode;
  placement?: Placement;
}) {
  const shown = useRef<ToastItem[]>([]);
  const waiting = useRef<ToastItem[]>([]);
  const limit = useRef<number>(visibleLimit[placement]);
  const wide = useWide();

  // The next action notification replaces this handle, whether visible or queued.
  const lastAction = useRef<string | null>(null);

  // Dismiss by stable handle so placement changes do not invalidate caller cleanup.
  const dismiss = useCallback((handle: string) => {
    if (lastAction.current === handle) lastAction.current = null;
    const index = waiting.current.findIndex((entry) => entry.handle === handle);
    if (index !== -1) {
      waiting.current.splice(index, 1);
      return;
    }
    const entry = shown.current.find((each) => each.handle === handle);
    if (entry !== undefined) sonner.dismiss(entry.id);
  }, []);

  const display = useCallback(
    (item: ToastItem) => {
      shown.current.push(item);
      const closed = () => {
        const index = shown.current.findIndex((entry) => entry.id === item.id);
        if (index === -1) return;
        shown.current.splice(index, 1);
        while (shown.current.length < limit.current) {
          const next = waiting.current.shift();
          if (next === undefined) return;
          display(next);
        }
      };
      const { action, duration } = item.options;
      sonner(item.message, {
        id: item.id,
        duration:
          duration ?? (action === undefined ? toastDuration : actionToastDuration),
        action:
          action === undefined ? undefined : (
            <ToastActionButton
              item={item}
              action={action}
              onDone={() => dismiss(item.handle)}
            />
          ),
        onAutoClose: closed,
        onDismiss: closed,
      });
    },
    [dismiss],
  );

  const enqueue = useCallback(
    (item: ToastItem) => {
      if (shown.current.length < limit.current) {
        display(item);
        return;
      }
      if (limit.current === visibleLimit.default) {
        // List notifications replace the oldest visible item instead of waiting.
        const oldest = shown.current.shift();
        if (oldest !== undefined) sonner.dismiss(oldest.id);
        display(item);
        return;
      }
      waiting.current.push(item);
    },
    [display],
  );

  const show = useCallback<ShowToast>(
    (message, options = {}) => {
      const id = nextToastId();
      const item: ToastItem = {
        id,
        handle: id,
        message,
        options,
        busy: { running: false },
      };
      if (options.action !== undefined) {
        if (lastAction.current !== null) dismiss(lastAction.current);
        lastAction.current = item.handle;
      }
      enqueue(item);
      return () => dismiss(item.handle);
    },
    [dismiss, enqueue],
  );

  // Entering playback retains the first notification and requeues the rest; returning to lists fills three slots.
  useEffect(() => {
    limit.current = visibleLimit[placement];
    const extra = shown.current.splice(limit.current);
    for (const item of extra) sonner.dismiss(item.id);
    // Redisplay with a new Sonner ID to avoid updating a dismissing toast; keep the stable handle for caller dismissal.
    const renamed = extra.map((item) => ({ ...item, id: nextToastId() }));
    waiting.current.unshift(...renamed);
    while (shown.current.length < limit.current) {
      const next = waiting.current.shift();
      if (next === undefined) break;
      display(next);
    }
  }, [display, placement]);

  // Clear all on unmount because Sonner retains notifications outside the provider and could show them in a later Toaster.
  useEffect(
    () => () => {
      shown.current = [];
      waiting.current = [];
      sonner.dismiss();
    },
    [],
  );

  return (
    <ToastContext.Provider value={show}>
      {children}
      <Toaster
        {...toasterPlacement(placement, wide)}
        expand
        visibleToasts={visibleLimit.default}
        customAriaLabel={t.common.notifications}
      />
    </ToastContext.Provider>
  );
}
