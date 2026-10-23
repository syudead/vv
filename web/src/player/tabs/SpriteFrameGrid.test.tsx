import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import SpriteFrameGrid from "./SpriteFrameGrid";

vi.mock("../../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/client")>()),
  fetchSeekThumbnailSprite: vi.fn(),
  fetchSeekThumbnailSheet: vi.fn(),
}));

/** Two sheets of four 10 s frames each. */
const sprite: SeekThumbnailSprite = {
  intervalMs: 10_000,
  frameCount: 8,
  columns: 2,
  rows: 2,
  frameWidth: 320,
  frameHeight: 180,
  sheets: ["/sheet/0", "/sheet/1"],
};

/** The one observer the grid creates; `enter` reports frames as visible. */
const observer = {
  callback: undefined as IntersectionObserverCallback | undefined,
  observed: [] as Element[],
  unobserved: [] as Element[],
};

function enter(label: string) {
  const item = screen.getByRole("button", { name: label }).closest("li")!;
  act(() =>
    observer.callback?.(
      [{ target: item, isIntersecting: true } as unknown as IntersectionObserverEntry],
      {} as IntersectionObserver,
    ),
  );
}

let sheetResult: () => Promise<Blob>;

beforeEach(() => {
  observer.callback = undefined;
  observer.observed = [];
  observer.unobserved = [];
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        observer.callback = callback;
      }
      observe(element: Element) {
        observer.observed.push(element);
      }
      unobserve(element: Element) {
        observer.unobserved.push(element);
      }
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = () => "blob:sheet";
      static revokeObjectURL = () => undefined;
    },
  );
  sheetResult = () => Promise.resolve(new Blob());
  vi.mocked(fetchSeekThumbnailSprite).mockResolvedValue(sprite);
  vi.mocked(fetchSeekThumbnailSheet).mockImplementation(() => sheetResult());
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.mocked(fetchSeekThumbnailSprite).mockReset();
  vi.mocked(fetchSeekThumbnailSheet).mockReset();
});

function sheetUrls(): string[] {
  return vi.mocked(fetchSeekThumbnailSheet).mock.calls.map(([url]) => url);
}

async function renderGrid(onPick = vi.fn()) {
  render(
    <SpriteFrameGrid
      spriteUrl="/sprite"
      sendingFrame={null}
      locked={false}
      currentMs={undefined}
      onPick={onPick}
    />,
  );
  await screen.findByRole("list", { name: "A frame from the video" });
  return onPick;
}

describe("SpriteFrameGrid", () => {
  it("シートはそのコマが見えてから一度だけ読み、押したコマのコマ番号と位置を渡す", async () => {
    const onPick = await renderGrid();
    expect(observer.observed).toHaveLength(8);
    expect(sheetUrls()).toEqual([]);

    enter("Use the frame at 0:50");
    enter("Use the frame at 1:00");
    expect(sheetUrls()).toEqual(["/sheet/1"]);

    fireEvent.click(screen.getByRole("button", { name: "Use the frame at 0:50" }));
    expect(onPick).toHaveBeenCalledWith(5, 50_000);
  });

  it("読めなかったシートは理由と Retry を出し、Retry でそのシートを読み直す", async () => {
    sheetResult = () => Promise.reject(new Error("network"));
    await renderGrid();
    enter("Use the frame at 0:00");
    const failure = await screen.findByRole("alert");
    expect(failure.textContent).toContain("Couldn't load the frames");

    sheetResult = () => Promise.resolve(new Blob());
    fireEvent.click(within(failure).getByRole("button", { name: "Retry" }));
    expect(sheetUrls()).toEqual(["/sheet/0", "/sheet/0"]);
    await vi.waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  });
});
