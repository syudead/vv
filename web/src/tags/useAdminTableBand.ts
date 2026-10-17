import {
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

/**
 * useAdminTableBand は管理表の画面（`AdminTablePage`）の、上部バーの下に留める帯と表の
 * 列の見出しの高さを測る。タグの画面と
 * 人物・団体の画面が使う。
 *
 * - `bandRef` は帯（`AdminTablePage` の `bandRef`）、`headRef` は表の列の見出しに渡す。
 * - `stuckBottom` は留まったときの帯の下端の表示域の中での位置（上部バーの高さ＋帯の高さ、
 *   px）、`rowsTop` はそれに列の見出しの高さを足したもので、行へスクロールするときに
 *   行が帯と見出しの下に隠れないよう差し引く。ブラウザがフォーカスした要素を表示域へ
 *   寄せるときも隠さないよう、文書の `scroll-padding-top` にも置く（離れるときに戻す）。
 * - `scrollMargin` は一覧の上端の文書の中での位置（px）で、`useListScrollMargin` が測る。
 * - `scrollListToTop` は、一覧の先頭が帯の下に隠れていれば先頭まで戻す。
 */
export function useAdminTableBand() {
  const bandRef = useRef<HTMLDivElement | null>(null);
  const [stuckBottom, setStuckBottom] = useState(0);
  const [headHeight, setHeadHeight] = useState(0);
  const [scrollMargin, setScrollMargin] = useState(0);

  // 列の見出しの高さを測る。表が無い間（読み込み中・空）は 0。
  const headRef = useCallback((head: HTMLTableSectionElement | null) => {
    if (head === null) return;
    const measure = () => setHeadHeight(head.offsetHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(head);
    return () => {
      observer.disconnect();
      setHeadHeight(0);
    };
  }, []);
  const rowsTop = stuckBottom + headHeight;

  // 帯の下端（上部バー＋帯の高さ）を測る。帯は幅や選択で高さが変わる。
  useLayoutEffect(() => {
    const band = bandRef.current;
    if (band === null) return;
    const measure = () => {
      const top = Number.parseFloat(getComputedStyle(band).top);
      setStuckBottom((Number.isFinite(top) ? top : 0) + band.offsetHeight);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(band);
    return () => observer.disconnect();
  }, []);

  // ブラウザがフォーカスした要素を表示域へ寄せるときも、帯の下に隠さない。
  // 文書のスクロールはこの画面のものではないので、離れるときに戻す。
  useEffect(() => {
    const root = document.documentElement;
    const previous = root.style.scrollPaddingTop;
    root.style.scrollPaddingTop = `${String(rowsTop)}px`;
    return () => {
      root.style.scrollPaddingTop = previous;
    };
  }, [rowsTop]);

  function scrollListToTop(listBox: HTMLElement | null) {
    const listTop = listBox?.getBoundingClientRect().top;
    if (listTop !== undefined && listTop < stuckBottom) {
      window.scrollTo({ top: Math.max(0, window.scrollY + listTop - stuckBottom) });
    }
  }

  return {
    bandRef,
    headRef,
    stuckBottom,
    rowsTop,
    scrollMargin,
    setScrollMargin,
    scrollListToTop,
  };
}

/**
 * useListScrollMargin は一覧（`listRef`）の上端の文書の中での位置を測って `set` へ渡す。
 * 上端は作成の行の有無やツールバーの折り返しで動くので、文書の大きさが変わるたびに
 * 測り直す。`enabled` が偽の間（一覧を描いていない）は測らない。
 */
export function useListScrollMargin(
  listRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  set: (margin: number) => void,
) {
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!enabled || list === null) return;
    const measure = () => set(list.getBoundingClientRect().top + window.scrollY);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [enabled, listRef, set]);
}
