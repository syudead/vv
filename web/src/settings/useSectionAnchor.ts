import { type RefObject, useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

/**
 * useSectionAnchor は Settings の節を `/settings/<page>#<id>` の行き先にする。返す ref を節を包む
 * 要素（`id` を付ける）に渡すと、その URL で移ってきたとき、節の見出し（PageSection が描く
 * h2）へスクロールしてフォーカスを移す。見出しは移ってきたときだけ focus できるようにする。
 * 節の上にある節が読み込みで伸び縮みしても見出しに留まるよう、少し後にも合わせ直す。
 */
export function useSectionAnchor(id: string): RefObject<HTMLDivElement | null> {
  const location = useLocation();
  const anchor = useRef<HTMLDivElement | null>(null);

  useLayoutEffect(() => {
    if (location.hash !== `#${id}`) return;
    const focusHeading = () => {
      const heading = anchor.current?.querySelector<HTMLHeadingElement>("h2");
      if (heading === null || heading === undefined) return;
      heading.tabIndex = -1;
      heading.scrollIntoView?.({ block: "start" });
      heading.focus();
    };
    focusHeading();
    const raf = window.requestAnimationFrame(focusHeading);
    const timers = [
      window.setTimeout(focusHeading, 0),
      window.setTimeout(focusHeading, 100),
    ];
    return () => {
      window.cancelAnimationFrame(raf);
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [id, location.hash, location.key]);

  return anchor;
}
