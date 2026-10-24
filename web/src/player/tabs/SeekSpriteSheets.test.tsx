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

/** Seven 10 s frames on two sheets of four. */
const sprite: SeekThumbnailSprite = {
  intervalMs: 10_000,
  frameCount: 7,
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
  it("空きコマを除いたコマを時刻順に並べ、シートは見えてから一度だけ読み、押すとそのコマをダイアログで大きく出す", async () => {
    await renderSheets();
    // Seven frames on two sheets of four: the sheet's last cell is padding.
    expect(screen.getAllByRole("listitem")).toHaveLength(7);
    expect(sheetUrls()).toEqual([]);

    enter(5);
    enter(6);
    expect(sheetUrls()).toEqual(["/sheet/1"]);
    const frame = screen.getByRole("button", { name: "Show the frame at 0:50 large" });
    await vi.waitFor(() => expect(frame.hasAttribute("disabled")).toBe(false));
    expect(
      screen
        .getByRole("button", { name: "Show the frame at 0:00 large" })
        .hasAttribute("disabled"),
    ).toBe(true);

    fireEvent.click(frame);
    const dialog = await screen.findByRole("dialog", { name: "Frame at 0:50" });
    const image = within(dialog).getByRole("img", { name: "Frame at 0:50" });
    expect(image.style.backgroundImage).toBe('url("blob:sheet")');
    // Frame 5 is the second cell of sheet 2's first row.
    expect(image.style.backgroundPosition).toBe("100% 0%");
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
