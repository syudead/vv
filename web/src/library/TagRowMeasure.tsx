import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";

type Callback = () => void;

interface TagRowMeasureContextValue {
  /** observe はその要素を見張りへ加える。戻り値で外す。 */
  observe: (el: Element, callback: Callback) => () => void;
}

const TagRowMeasureContext = createContext<TagRowMeasureContextValue | null>(null);

/**
 * TagRowMeasureProvider は、カードのタグの行の幅を測るための、一覧に1つだけの
 * `ResizeObserver` を提供する。`CardTagRow` は `useTagRowMeasure` でここへ登録する。
 */
export function TagRowMeasureProvider({ children }: { children: ReactNode }) {
  const callbacks = useRef(new Map<Element, Callback>());
  const [observer] = useState<ResizeObserver | null>(() =>
    typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver((entries) => {
          for (const entry of entries) callbacks.current.get(entry.target)?.();
        }),
  );

  useEffect(() => () => observer?.disconnect(), [observer]);

  const observe = useCallback(
    (el: Element, callback: Callback) => {
      callbacks.current.set(el, callback);
      observer?.observe(el);
      return () => {
        callbacks.current.delete(el);
        observer?.unobserve(el);
      };
    },
    [observer],
  );

  return (
    <TagRowMeasureContext.Provider value={{ observe }}>
      {children}
    </TagRowMeasureContext.Provider>
  );
}

/**
 * useTagRowMeasure は一覧共有の見張りへの登録関数を返す。`TagRowMeasureProvider`
 * の外（単体テストなど）では null を返し、呼び出し側は layout effect の初回の
 * 計測だけで済ませる。
 */
export function useTagRowMeasure(): TagRowMeasureContextValue["observe"] | null {
  return useContext(TagRowMeasureContext)?.observe ?? null;
}
