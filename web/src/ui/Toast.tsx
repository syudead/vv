import { createContext, type ReactNode, useCallback, useContext, useState } from "react";

interface ToastItem {
  id: number;
  message: string;
}

const ToastContext = createContext<(message: string) => void>(() => undefined);

/** useToast は短い通知を出す関数を返す。 */
export function useToast(): (message: string) => void {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: ReactNode }) {
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
        className="pointer-events-none fixed inset-x-0 bottom-6 z-50 flex flex-col items-center gap-2"
      >
        {items.map((item) => (
          <div
            key={item.id}
            className="rounded-md bg-fg px-4 py-2.5 text-sm font-medium text-bg shadow-elevated animate-slide-up"
          >
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
