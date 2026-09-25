import { act, cleanup, render } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Zoom } from "../preferences/viewPreferences";
import { useZoomAnchor } from "./useZoomAnchor";

/** 倍率ごとにカードの高さが変わる一覧を模す（上から順に高さ `height` で並ぶ）。 */
function Harness({ onReady }: { onReady: (change: (zoom: Zoom) => void) => void }) {
  const [zoom, setZoom] = useState<Zoom>(1);
  const { listRef, capture } = useZoomAnchor(zoom);
  onReady((next) => {
    capture();
    setZoom(next);
  });
  return (
    <div ref={listRef} data-zoom={zoom}>
      <article data-folder-path="a" />
      <article data-video-id="1" />
      <article data-video-id="2" />
      <article data-video-id="3" />
    </div>
  );
}

describe("useZoomAnchor", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("倍率を変える前に上端にあったカードを、変えた後も上端へ戻す", () => {
    let scrollY = 500;
    vi.spyOn(window, "scrollY", "get").mockImplementation(() => scrollY);
    const scrollTo = vi.fn((options: ScrollToOptions) => {
      scrollY = options.top ?? scrollY;
    });
    vi.stubGlobal("scrollTo", scrollTo);
    // 各カードの文書上の位置は index × 高さ。高さは倍率 1 で 200、倍率 3 で 400。
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
      this: HTMLElement,
    ) {
      const list = this.parentElement;
      const zoom = Number(list?.dataset.zoom ?? 1);
      const height = zoom === 3 ? 400 : 200;
      const index = list === null ? 0 : Array.from(list.children).indexOf(this);
      const top = index * height - scrollY;
      return { top, bottom: top + height } as DOMRect;
    });

    let change: (zoom: Zoom) => void = () => undefined;
    render(<Harness onReady={(fn) => (change = fn)} />);
    // 上端（ナビバー 48px の下）にあるのは 3 番目のカード（id 2、文書上 400〜600）。
    act(() => change(3));
    // 倍率 3 では 3 番目のカードは文書上 800 から。ナビバーと余白を引いた位置へ。
    expect(scrollTo).toHaveBeenCalledWith({ top: 800 - 48 - 8, behavior: "auto" });
  });

  it("一番上にいるときは位置を動かさない", () => {
    vi.spyOn(window, "scrollY", "get").mockReturnValue(0);
    const scrollTo = vi.fn();
    vi.stubGlobal("scrollTo", scrollTo);
    let change: (zoom: Zoom) => void = () => undefined;
    render(<Harness onReady={(fn) => (change = fn)} />);
    act(() => change(3));
    expect(scrollTo).not.toHaveBeenCalled();
  });
});
