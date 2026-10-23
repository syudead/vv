import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import type { ScrubFrameData } from "../../ui/ScrubPreview";
import { useSpriteFrame } from "./useSpriteFrame";

vi.mock("../../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/client")>()),
  fetchSeekThumbnailSprite: vi.fn(),
  fetchSeekThumbnailSheet: vi.fn(),
}));

/** Two sheets of four 10 s frames each. */
function sprite(video: string): SeekThumbnailSprite {
  return {
    intervalMs: 10_000,
    frameCount: 8,
    columns: 2,
    rows: 2,
    frameWidth: 320,
    frameHeight: 180,
    sheets: [`/${video}/sheet/0`, `/${video}/sheet/1`],
  };
}

interface Deferred<T> {
  url: string;
  signal: AbortSignal;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
}

let sprites: Array<Deferred<SeekThumbnailSprite>> = [];
let sheets: Array<Deferred<Blob>> = [];
let objectUrls = 0;
const revokeObjectURL = vi.fn();

beforeEach(() => {
  sprites = [];
  sheets = [];
  objectUrls = 0;
  revokeObjectURL.mockClear();
  vi.mocked(fetchSeekThumbnailSprite).mockImplementation(
    (url, signal) =>
      new Promise((resolve, reject) => {
        sprites.push({ url, signal, resolve, reject });
      }),
  );
  vi.mocked(fetchSeekThumbnailSheet).mockImplementation(
    (url, signal) =>
      new Promise((resolve, reject) => {
        sheets.push({ url, signal, resolve, reject });
      }),
  );
  Object.defineProperty(URL, "createObjectURL", {
    value: () => `blob:sheet-${String(++objectUrls)}`,
    configurable: true,
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    value: revokeObjectURL,
    configurable: true,
  });
});

afterEach(() => {
  vi.mocked(fetchSeekThumbnailSprite).mockReset();
  vi.mocked(fetchSeekThumbnailSheet).mockReset();
});

interface Props {
  url: string | undefined;
  positionMs: number | null;
}

/** Records every render's result, so a frame shown for one render only is still observed. */
function renderFrame(initial: Props) {
  const results: Array<ScrubFrameData | null> = [];
  const hook = renderHook(
    ({ url, positionMs }: Props) => {
      const frame = useSpriteFrame(url, positionMs);
      results.push(frame);
      return frame;
    },
    { initialProps: initial },
  );
  return { ...hook, results };
}

async function settle<T>(request: Deferred<T> | undefined, value: T) {
  await act(async () => {
    request?.resolve(value);
    await Promise.resolve();
  });
}

async function fail<T>(request: Deferred<T> | undefined) {
  await act(async () => {
    request?.reject(new Error("network"));
    await Promise.resolve();
  });
}

describe("useSpriteFrame", () => {
  it("reads nothing until a position exists, then draws the cell at that position", async () => {
    const { result, rerender } = renderFrame({ url: "/a/sprite", positionMs: null });
    expect(fetchSeekThumbnailSprite).not.toHaveBeenCalled();

    rerender({ url: "/a/sprite", positionMs: 15_000 });
    expect(sprites.map((request) => request.url)).toEqual(["/a/sprite"]);
    await settle(sprites[0], sprite("a"));
    expect(sheets.map((request) => request.url)).toEqual(["/a/sheet/0"]);
    await settle(sheets[0], new Blob());

    expect(result.current).toEqual({
      url: "blob:sheet-1",
      sprite: sprite("a"),
      cell: { frame: 1, sheet: 0, column: 1, row: 0 },
    });
  });

  it("never draws the previous video's frame after the sprite URL changes", async () => {
    const { result, results, rerender } = renderFrame({
      url: "/a/sprite",
      positionMs: 0,
    });
    await settle(sprites[0], sprite("a"));
    await settle(sheets[0], new Blob());
    expect(result.current?.url).toBe("blob:sheet-1");

    const shownForA = results.length;
    rerender({ url: "/b/sprite", positionMs: 0 });

    expect(results.slice(shownForA)).not.toContainEqual(
      expect.objectContaining({ url: "blob:sheet-1" }),
    );
    expect(result.current).toBeNull();
    expect(sprites[0]?.signal.aborted).toBe(true);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");
    expect(sprites[1]?.url).toBe("/b/sprite");
  });

  it("requests a failed sheet again once the position reaches another frame", async () => {
    const { result, rerender } = renderFrame({ url: "/a/sprite", positionMs: 0 });
    await settle(sprites[0], sprite("a"));
    await fail(sheets[0]);

    rerender({ url: "/a/sprite", positionMs: 5_000 });
    expect(sheets).toHaveLength(1);

    rerender({ url: "/a/sprite", positionMs: 10_000 });
    expect(sheets.map((request) => request.url)).toEqual(["/a/sheet/0", "/a/sheet/0"]);
    await settle(sheets[1], new Blob());
    expect(result.current?.cell.frame).toBe(1);
    expect(result.current?.url).toBe("blob:sheet-1");
  });
});
