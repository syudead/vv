import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import SeekSpriteSheets from "./SeekSpriteSheets";

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

/** The one observer the list creates; `enter` reports a sheet as visible. */
const observer = {
  callback: undefined as IntersectionObserverCallback | undefined,
  observed: [] as Element[],
  unobserved: [] as Element[],
};

function enter(index: number) {
  const item = screen.getAllByRole("listitem")[index]!;
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

async function renderSheets() {
  render(<SeekSpriteSheets spriteUrl="/sprite" />);
  await screen.findByRole("list", { name: "Seek preview" });
}

describe("SeekSpriteSheets", () => {
  it("シートは見えてから一度だけ読み、読めたシートを画像として出す", async () => {
    await renderSheets();
    expect(observer.observed).toHaveLength(2);
    expect(sheetUrls()).toEqual([]);

    enter(1);
    enter(1);
    expect(sheetUrls()).toEqual(["/sheet/1"]);
    const image = await screen.findByRole("img", { name: "Seek preview sheet 2 of 2" });
    expect(image.getAttribute("src")).toBe("blob:sheet");
    expect(screen.queryByRole("img", { name: "Seek preview sheet 1 of 2" })).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("読めなかったシートは理由と Retry を出し、Retry でそのシートを読み直す", async () => {
    sheetResult = () => Promise.reject(new Error("network"));
    await renderSheets();
    enter(0);
    const failure = await screen.findByRole("alert");
    expect(failure.textContent).toContain("Couldn't load the seek preview");

    sheetResult = () => Promise.resolve(new Blob());
    fireEvent.click(within(failure).getByRole("button", { name: "Retry" }));
    expect(sheetUrls()).toEqual(["/sheet/0", "/sheet/0"]);
    await vi.waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  });
});
