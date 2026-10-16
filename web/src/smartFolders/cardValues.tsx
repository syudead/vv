import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { subscribeServerEvents } from "../api/serverEvents";
import { readSmartFolderCards } from "../api/smartFolders";
import { type CardValue, chunks, withAnswer } from "./cardValueRules";

// スマートフォルダのカードの値（specs/052-smart-folders/ui-design.md「Smart folder card and
// row」、research.md R-14）。一覧の項目は要約だけを持ち、画面に入ったカードの値を
// GET /api/smart-folders/cards でまとめて読む。

interface CardValues {
  values: ReadonlyMap<number, CardValue>;
  /** カード `id` が画面に入った。まだ求めていなければ値を読む。 */
  request: (id: number) => void;
}

const CardValuesContext = createContext<CardValues | null>(null);

const noValues: ReadonlyMap<number, CardValue> = new Map();

/**
 * SmartFolderCardsProvider は一覧 1 つのスマートフォルダのカードの値を持つ。カードが画面に
 * 入るたびに求めた id を同じ描画の間だけためて、60 件ずつ 1 回の読み出しにする。`smartFolder`
 * の知らせが名指したフォルダのうち、すでに求めたものの値を、知らせ 1 つにつき 1 回だけ骨組み
 * なしで読み直す。つなぎ直したら求めたものをすべて読み直す。失敗した読み出しは値を変えない
 * （カードは骨組みのまま、次の知らせで読み直す）。
 */
export function SmartFolderCardsProvider({ children }: { children: ReactNode }) {
  const [values, setValues] = useState<ReadonlyMap<number, CardValue>>(noValues);
  /** 一度でも求めた id（画面に入ったカード）。 */
  const asked = useRef(new Set<number>());
  /** 次の読み出しにまとめる id。 */
  const queued = useRef<number[]>([]);
  const flushTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const controllers = useRef(new Set<AbortController>());

  const read = useCallback((ids: readonly number[]) => {
    for (const chunk of chunks(ids)) {
      const controller = new AbortController();
      controllers.current.add(controller);
      readSmartFolderCards(chunk, controller.signal).then(
        (cards) => {
          controllers.current.delete(controller);
          if (controller.signal.aborted) return;
          setValues((current) => withAnswer(current, chunk, cards));
        },
        () => {
          controllers.current.delete(controller);
        },
      );
    }
  }, []);

  const request = useCallback(
    (id: number) => {
      if (asked.current.has(id)) return;
      asked.current.add(id);
      queued.current.push(id);
      if (flushTimer.current !== undefined) return;
      flushTimer.current = setTimeout(() => {
        flushTimer.current = undefined;
        const ids = queued.current;
        queued.current = [];
        read(ids);
      }, 0);
    },
    [read],
  );

  useEffect(() => {
    const unsubscribe = subscribeServerEvents({
      smartFolder: (ids) => {
        const shown = ids.filter((id) => asked.current.has(id));
        if (shown.length > 0) read(shown);
      },
      open: (reconnected) => {
        if (reconnected && asked.current.size > 0) read(Array.from(asked.current));
      },
    });
    const live = controllers.current;
    return () => {
      unsubscribe();
      clearTimeout(flushTimer.current);
      for (const controller of live) controller.abort();
      live.clear();
    };
  }, [read]);

  const context = useMemo(() => ({ values, request }), [request, values]);
  return <CardValuesContext value={context}>{children}</CardValuesContext>;
}

/**
 * useSmartFolderCardValue はカード `id` の値と、カードの要素に付ける `ref` である。要素が
 * 画面に入ったら値を求める。値が届くまでは undefined。SmartFolderCardsProvider の外では
 * 何も求めず、値はいつも undefined。
 */
export function useSmartFolderCardValue(id: number) {
  const context = useContext(CardValuesContext);
  const [node, setNode] = useState<Element | null>(null);
  const request = context?.request;
  useEffect(() => {
    if (node === null || request === undefined) return;
    if (typeof IntersectionObserver === "undefined") {
      request(id);
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      request(id);
      observer.disconnect();
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [id, node, request]);
  return { value: context?.values.get(id), ref: setNode };
}
