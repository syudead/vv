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
  /**
   * 見える個数の控え。鍵は行の幅とチップの幅を決める値（名前・仮・フォルダ由来）で、
   * 値は見えるチップの数である。一覧は画面の近くのカードだけを描くので、スクロールで
   * 戻ったカードは同じ鍵の控えを使い、チップの幅を測り直さない（issue 675）。
   */
  counts: Map<string, number>;
}

const TagRowMeasureContext = createContext<TagRowMeasureContextValue | null>(null);

/**
 * TagRowMeasureProvider は、カードのタグの行の幅を測るための、一覧に1つだけの
 * `ResizeObserver` と、見える個数の控えを提供する（specs/014-video-tags/ui-design.md
 * 「Overflow」）。一覧は画面の近くのカードだけを描くので、見張るのは描いているカードの
 * 行だけである。`CardTagRow` は `useTagRowMeasure` でここへ登録する。
 */
export function TagRowMeasureProvider({ children }: { children: ReactNode }) {
  const callbacks = useRef(new Map<Element, Callback>());
  const [counts] = useState(() => new Map<string, number>());
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
    <TagRowMeasureContext.Provider value={{ observe, counts }}>
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

/**
 * useTagRowCounts は一覧共有の見える個数の控えを返す。Provider の外では null を返し、
 * 呼び出し側は毎回測る。
 */
export function useTagRowCounts(): Map<string, number> | null {
  return useContext(TagRowMeasureContext)?.counts ?? null;
}
