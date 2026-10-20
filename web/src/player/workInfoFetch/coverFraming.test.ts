import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfoFetch } from "../../api/workInfoPlugins";
import { WHOLE_IMAGE } from "../thumbnailUpload/framing";
import { useCoverFraming } from "./coverFraming";

// The cover request and decoding are the boundaries.

const plugins = vi.hoisted(() => ({ getWorkInfoFetchCover: vi.fn() }));
vi.mock("../../api/workInfoPlugins", async (original) => ({
  ...(await original<typeof import("../../api/workInfoPlugins")>()),
  ...plugins,
}));

const compose = vi.hoisted(() => ({ loadUploadImage: vi.fn() }));
vi.mock("../thumbnailUpload/composeImage", () => compose);

const video = { id: 7, width: 1920, height: 1080 } as Video;

function result(fetchId: string) {
  return {
    fetchId,
    covers: [
      { width: 640, height: 360 },
      { width: 800, height: 450 },
    ],
  } as WorkInfoFetch;
}

function image() {
  return { bitmap: { close: vi.fn() }, width: 800, height: 450 };
}

describe("useCoverFraming", () => {
  beforeEach(() => {
    plugins.getWorkInfoFetchCover.mockReset();
    plugins.getWorkInfoFetchCover.mockResolvedValue(new Blob(["cover"]));
    compose.loadUploadImage.mockReset();
  });

  it("仕上げた画像で構図を決めるとそれがカバーになり、元の画像を閉じ、結果が変われば仕上げた画像も閉じる", async () => {
    const original = image();
    compose.loadUploadImage.mockResolvedValueOnce({ image: original });
    const { result: hook, rerender } = renderHook(
      ({ fetched }: { fetched: WorkInfoFetch }) => useCoverFraming(video, fetched, 1),
      { initialProps: { fetched: result("F1") } },
    );
    await waitFor(() => expect(hook.current.image).toBe(original));
    expect(plugins.getWorkInfoFetchCover).toHaveBeenCalledWith(
      7,
      "F1",
      1,
      expect.anything(),
    );

    const touched = image();
    const fit = { ...WHOLE_IMAGE, backdrop: "black" as const };
    act(() =>
      hook.current.keep(
        fit,
        touched as unknown as Parameters<typeof hook.current.keep>[1],
      ),
    );
    expect(hook.current.image).toBe(touched);
    expect(hook.current.fit).toEqual(fit);
    expect(original.bitmap.close).toHaveBeenCalled();

    compose.loadUploadImage.mockResolvedValueOnce({ image: image() });
    rerender({ fetched: result("F2") });
    expect(touched.bitmap.close).toHaveBeenCalled();
    expect(hook.current.fit).toEqual(WHOLE_IMAGE);
  });
});
