/**
 * stubRowHeights は jsdom に無い表示域の高さと行の高さを置き、窓の仮想化
 * （useWindowVirtualizer）が描く行を実際の画面に近い数に絞る。jsdom のままでは
 * 行の高さが 0 で、一覧が読み込んだ行の大半を描き、描画と役割での検索が重くなる。
 *
 * `data-index` を持つ要素（仮想化の行）の offsetHeight を `rowHeight` に、それ以外を
 * `otherHeight` の値にする。戻り値は元に戻す関数である。
 */
export function stubRowHeights({
  rowHeight = 40,
  viewportHeight = 200,
  otherHeight = () => 0,
}: {
  rowHeight?: number;
  viewportHeight?: number;
  otherHeight?: (element: HTMLElement) => number;
} = {}): () => void {
  const originalInnerHeight = window.innerHeight;
  const originalOffsetHeight = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "offsetHeight",
  );
  window.innerHeight = viewportHeight;
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.hasAttribute("data-index") ? rowHeight : otherHeight(this);
    },
  });
  return () => {
    window.innerHeight = originalInnerHeight;
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
  };
}
