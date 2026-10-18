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
  observe: (el: Element, callback: Callback) => () => void;
}

const TagRowMeasureContext = createContext<TagRowMeasureContextValue | null>(null);

/** Share one ResizeObserver across the list's tag rows. */
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

/** Outside the provider, callers measure only during initial layout. */
export function useTagRowMeasure(): TagRowMeasureContextValue["observe"] | null {
  return useContext(TagRowMeasureContext)?.observe ?? null;
}
