import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";

import { cn } from "../lib/cn";

interface ToastItem {
  id: number;
  message: string;
  expiresAt: number | null;
}

const toastDuration = 2800;

const ToastContext = createContext<(message: string) => void>(() => undefined);

/** useToast は短い通知を出す関数を返す。 */
export function useToast(): (message: string) => void {
  return useContext(ToastContext);
}

export function ToastProvider({
  children,
  placement = "default",
}: {
  children: ReactNode;
  placement?: "default" | "playback";
}) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const show = useCallback(
    (message: string) => {
      const id = Date.now() + Math.random();
      setItems((current) => {
        const expiresAt =
          placement === "default" || current.length === 0
            ? Date.now() + toastDuration
            : null;
        const next = [...current, { id, message, expiresAt }];
        return placement === "default" ? next.slice(-3) : next;
      });
    },
    [placement],
  );

  useEffect(() => {
    setItems((current) => {
      if (current.length === 0) return current;
      const now = Date.now();
      return current.map((item, index) => ({
        ...item,
        expiresAt:
          placement === "playback"
            ? index === 0
              ? (item.expiresAt ?? now + toastDuration)
              : null
            : (item.expiresAt ?? now + toastDuration),
      }));
    });
  }, [placement]);

  const nextExpiry =
    placement === "playback"
      ? (items[0]?.expiresAt ?? null)
      : items.reduce<number | null>((next, item) => {
          if (item.expiresAt === null) return next;
          return next === null ? item.expiresAt : Math.min(next, item.expiresAt);
        }, null);
  useEffect(() => {
    if (nextExpiry === null) return;
    const timeout = window.setTimeout(
      () => {
        const now = Date.now();
        setItems((current) => {
          if (placement === "default") {
            return current.filter(
              (item) => item.expiresAt === null || item.expiresAt > now,
            );
          }
          const remaining = current.slice(1);
          if (remaining.length === 0) return remaining;
          return remaining.map((item, index) =>
            index === 0 ? { ...item, expiresAt: now + toastDuration } : item,
          );
        });
      },
      Math.max(0, nextExpiry - Date.now()),
    );
    return () => window.clearTimeout(timeout);
  }, [nextExpiry, placement]);

  return (
    <ToastContext.Provider value={show}>
      {children}
      <div
        aria-live="polite"
        className={cn(
          "pointer-events-none fixed inset-x-0 z-50 flex flex-col gap-2",
          placement === "playback"
            ? "top-1.5 items-end px-3 sm:items-center sm:px-0"
            : "top-16 items-end px-3 lg:top-auto lg:bottom-20 lg:items-center lg:px-0",
        )}
      >
        {(placement === "playback" ? items.slice(0, 1) : items).map((item) => (
          <div
            key={item.id}
            className={cn(
              "rounded-md bg-elevated px-4 py-2.5 text-sm text-fg shadow-elevated animate-slide-up",
              placement === "playback" &&
                "max-w-[calc(100vw-8rem)] break-words sm:max-w-72",
            )}
          >
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
