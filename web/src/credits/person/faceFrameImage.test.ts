import { afterEach, describe, expect, it, vi } from "vitest";

import { encodeFaceFrame } from "./faceFrameImage";

/** jsdom has no canvas, so a fake records the cut and answers toBlob with what the test chooses. */
function fakeCanvas(blob: Blob | null) {
  const context = { drawImage: vi.fn(), imageSmoothingQuality: "low" };
  const drawImage = context.drawImage;
  const canvas = {
    width: 0,
    height: 0,
    getContext: () => context,
    toBlob: (done: (value: Blob | null) => void, type: string, quality: number) => {
      canvas.encoded = { type, quality };
      done(blob);
    },
    encoded: undefined as { type: string; quality: number } | undefined,
  };
  const create = document.createElement.bind(document);
  vi.spyOn(document, "createElement").mockImplementation((tag: string) =>
    tag === "canvas" ? (canvas as unknown as HTMLCanvasElement) : create(tag),
  );
  return { canvas, context, drawImage };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("encodeFaceFrame", () => {
  it("enlarges a small cut to 800 px on its long side, smoothly, as a JPEG", async () => {
    const jpeg = new Blob(["face"], { type: "image/jpeg" });
    const { canvas, context, drawImage } = fakeCanvas(jpeg);
    const image = {} as HTMLImageElement;

    const blob = await encodeFaceFrame(image, {
      x: 10,
      y: 20,
      width: 100.4,
      height: 150.6,
    });

    expect(blob).toBe(jpeg);
    expect([canvas.width, canvas.height]).toEqual([533, 800]);
    expect(drawImage).toHaveBeenCalledWith(image, 10, 20, 100.4, 150.6, 0, 0, 533, 800);
    expect(context.imageSmoothingQuality).toBe("high");
    expect(canvas.encoded?.type).toBe("image/jpeg");
  });

  it("reduces a large cut to 800 px on its long side", async () => {
    const { canvas } = fakeCanvas(new Blob(["face"], { type: "image/jpeg" }));

    await encodeFaceFrame({} as HTMLImageElement, {
      x: 0,
      y: 0,
      width: 1200,
      height: 1800,
    });

    expect([canvas.width, canvas.height]).toEqual([533, 800]);
  });

  it("rejects when the frame cannot be encoded", async () => {
    fakeCanvas(null);

    await expect(
      encodeFaceFrame({} as HTMLImageElement, { x: 0, y: 0, width: 2, height: 3 }),
    ).rejects.toThrow();
  });
});
