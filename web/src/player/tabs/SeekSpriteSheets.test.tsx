import { fireEvent, render, screen, within } from "@testing-library/react";
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

let sheetResult: () => Promise<Blob>;

beforeEach(() => {
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

function openSprite() {
  fireEvent.click(screen.getByRole("button", { name: "View seek sprite" }));
  return screen.findByRole("dialog", { name: "Seek sprite" });
}

describe("SeekSpriteSheets", () => {
  it("押すまで何も読まず、押すとスプライトと全シートを一度ずつ読んでダイアログに出す", async () => {
    render(<SeekSpriteSheets spriteUrl="/sprite" />);
    expect(fetchSeekThumbnailSprite).not.toHaveBeenCalled();

    const dialog = await openSprite();
    const images = await within(dialog).findAllByRole("img");
    expect(images.map((image) => image.getAttribute("alt"))).toEqual([
      "Seek sprite sheet 1 of 2",
      "Seek sprite sheet 2 of 2",
    ]);
    expect(images[0]!.getAttribute("src")).toBe("blob:sheet");
    expect(sheetUrls()).toEqual(["/sheet/0", "/sheet/1"]);
  });

  it("読めなかったシートは理由と Retry を出し、Retry でそのシートを読み直す", async () => {
    sheetResult = () => Promise.reject(new Error("network"));
    render(<SeekSpriteSheets spriteUrl="/sprite" />);
    const dialog = await openSprite();
    const failure = await within(dialog).findByRole("alert");
    expect(failure.textContent).toContain("Couldn't load the seek sprite");

    sheetResult = () => Promise.resolve(new Blob());
    fireEvent.click(within(failure).getByRole("button", { name: "Retry" }));
    await vi.waitFor(() => expect(within(dialog).queryByRole("alert")).toBeNull());
    expect(sheetUrls()).toEqual(["/sheet/0", "/sheet/1", "/sheet/0", "/sheet/1"]);
  });
});
