/**
 * Supply viewport and row heights so virtualization renders a realistic subset instead of most
 * rows at zero height. Return a function restoring the original dimensions.
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
