import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Images are wired to an observer read when the module loads, so these cases install one first.
class FakeObserver {
  static instance: FakeObserver | null = null;
  readonly observed = new Set<Element>();
  readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    FakeObserver.instance = this;
  }
  observe(element: Element) {
    this.observed.add(element);
  }
  unobserve(element: Element) {
    this.observed.delete(element);
  }
  reveal(target: Element) {
    act(() => {
      this.callback(
        [{ target, isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
    });
  }
}

async function loadWithObserver() {
  vi.resetModules();
  FakeObserver.instance = null;
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  const thumbnail = await import("../ui/VideoThumbnail");
  const folderArt = await import("../videoList/FolderArt");
  return {
    VideoThumbnailImage: thumbnail.VideoThumbnailImage,
    FolderArt: folderArt.default,
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("images that load when near the screen", () => {
  it("a video thumbnail sets its source only once its frame comes near", async () => {
    const { VideoThumbnailImage } = await loadWithObserver();
    const { container } = render(
      <VideoThumbnailImage src="/api/videos/1/thumbnail?v=a" />,
    );
    const image = container.querySelector("img");
    expect(image).not.toBeNull();
    expect(image?.getAttribute("src")).toBeNull();

    FakeObserver.instance?.reveal(image as Element);
    expect(container.querySelector("img")?.getAttribute("src")).toBe(
      "/api/videos/1/thumbnail?v=a",
    );
    expect(FakeObserver.instance?.observed.size).toBe(0);
  });

  it("a folder's cover sheets load together once the art comes near", async () => {
    const { FolderArt } = await loadWithObserver();
    const previews = [
      { videoId: 1, thumbnailUrl: "/api/videos/1/thumbnail?v=a" },
      { videoId: 2, thumbnailUrl: "/api/videos/2/thumbnail?v=b" },
    ];
    const { container } = render(<FolderArt previews={previews} />);
    const sources = () =>
      [...container.querySelectorAll("img")].map((image) => image.getAttribute("src"));
    expect(sources()).toEqual([null, null]);

    const art = container.querySelector("[data-folder-art]");
    FakeObserver.instance?.reveal(art as Element);
    expect(sources()).toEqual(previews.map((preview) => preview.thumbnailUrl));
  });
});
