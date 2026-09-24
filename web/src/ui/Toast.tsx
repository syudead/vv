import { createContext, type ReactNode, useCallback, useContext, useState } from "react";

import { cn } from "../lib/cn";

interface ToastItem {
  id: number;
  message: string;
}

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

  const show = useCallback((message: string) => {
    const id = Date.now() + Math.random();
    setItems((current) => [...current.slice(-2), { id, message }]);
    setTimeout(
      () => setItems((current) => current.filter((item) => item.id !== id)),
      2800,
    );
  }, []);

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
        {(placement === "playback" ? items.slice(-1) : items).map((item) => (
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
