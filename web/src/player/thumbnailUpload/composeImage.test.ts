import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  drawComposition,
  drawOverflow,
  encodeComposition,
  loadUploadImage,
  type UploadImage,
} from "./composeImage";
import { type Fit, fillFrame, WHOLE_IMAGE } from "./framing";

// jsdom lacks canvas and decoding; fake the 2D context and createImageBitmap.

function pngHeader(width: number, height: number): Uint8Array {
  const bytes = new Uint8Array(24);
  bytes.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  const view = new DataView(bytes.buffer);
  view.setUint32(16, width);
  view.setUint32(20, height);
  return bytes;
}

function animatedWebP(): Uint8Array {
  const bytes = new Uint8Array(30);
  bytes.set(
    [..."RIFF"].map((char) => char.charCodeAt(0)),
    0,
  );
  bytes.set(
    [..."WEBP"].map((char) => char.charCodeAt(0)),
    8,
  );
  bytes.set(
    [..."VP8X"].map((char) => char.charCodeAt(0)),
    12,
  );
  bytes[20] = 0x02;
  return bytes;
}

/** Add arrayBuffer, which jsdom's Blob lacks. */
function blob(source: Uint8Array): Blob {
  const bytes = new Uint8Array(source);
  const file = new Blob([bytes]);
  const slice = (start?: number, end?: number) => {
    const part = bytes.slice(start, end);
    return { arrayBuffer: () => Promise.resolve(part.buffer) } as unknown as Blob;
  };
  return Object.assign(file, { slice });
}

function bitmap(width: number, height: number) {
  return { width, height, close: vi.fn() } as unknown as ImageBitmap & {
    close: ReturnType<typeof vi.fn>;
  };
}

describe("loadUploadImage", () => {
  const decode = vi.fn<(file: Blob) => Promise<ImageBitmap>>();

  beforeEach(() => {
    decode.mockReset();
    vi.stubGlobal("createImageBitmap", decode);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("読めた画像を、向きを反映した大きさで返す", async () => {
    const decoded = bitmap(1000, 2000);
    decode.mockResolvedValueOnce(decoded);
    await expect(loadUploadImage(blob(pngHeader(2000, 1000)))).resolves.toEqual({
      image: { bitmap: decoded, width: 1000, height: 2000 },
    });
  });

  it("形式の分からない見出しとアニメーションの WebP は、復号せずに読めない画像として断る", async () => {
    for (const bytes of [new TextEncoder().encode("GIF89a"), animatedWebP()])
      await expect(loadUploadImage(blob(bytes))).resolves.toEqual({
        refusal: "thumbnail_upload_invalid",
      });
    expect(decode).not.toHaveBeenCalled();
  });

  it("見出しが 1 億 2000 万画素を越えれば、復号せずに大きすぎる画像として断る", async () => {
    await expect(loadUploadImage(blob(pngHeader(12000, 11000)))).resolves.toEqual({
      refusal: "thumbnail_upload_too_large",
    });
    expect(decode).not.toHaveBeenCalled();
  });

  it("復号できなければ読めない画像、復号した画像が大きすぎれば閉じて断る", async () => {
    decode.mockRejectedValueOnce(new Error("broken"));
    await expect(loadUploadImage(blob(pngHeader(10, 10)))).resolves.toEqual({
      refusal: "thumbnail_upload_invalid",
    });

    const huge = bitmap(12000, 11000);
    decode.mockResolvedValueOnce(huge);
    await expect(loadUploadImage(blob(pngHeader(10, 10)))).resolves.toEqual({
      refusal: "thumbnail_upload_too_large",
    });
    expect(huge.close).toHaveBeenCalled();
  });
});

function fakeContext(withFilter = true) {
  const calls: string[] = [];
  const context = {
    fillStyle: "",
    globalAlpha: 1,
    imageSmoothingQuality: "low",
    fillRect: vi.fn(() => calls.push(`fill ${context.fillStyle}`)),
    drawImage: vi.fn(() =>
      calls.push(
        `draw alpha=${String(context.globalAlpha)} filter=${context.filter ?? "-"}`,
      ),
    ),
    clearRect: vi.fn(() => calls.push("clear")),
    scale: vi.fn(),
    translate: vi.fn(),
    rotate: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(() => {
      context.globalAlpha = 1;
      if (withFilter) context.filter = "none";
    }),
    filter: withFilter ? "none" : undefined,
  } as {
    fillStyle: string;
    globalAlpha: number;
    imageSmoothingQuality: string;
    fillRect: ReturnType<typeof vi.fn>;
    drawImage: ReturnType<typeof vi.fn>;
    clearRect: ReturnType<typeof vi.fn>;
    scale: ReturnType<typeof vi.fn>;
    translate: ReturnType<typeof vi.fn>;
    rotate: ReturnType<typeof vi.fn>;
    save: ReturnType<typeof vi.fn>;
    restore: ReturnType<typeof vi.fn>;
    filter?: string;
  };
  if (!withFilter) delete context.filter;
  return { context, calls };
}

function canvasWith(context: unknown): HTMLCanvasElement {
  return {
    width: 0,
    height: 0,
    getContext: () => context,
  } as unknown as HTMLCanvasElement;
}

const portrait: UploadImage = { bitmap: bitmap(1000, 2000), width: 1000, height: 2000 };

const wide = { width: 2048, height: 1152 };

describe("drawComposition", () => {
  it("動画の比率の大きさにし、黒で塗ってから、余白にぼかした画像、手前に画像を描く", () => {
    const { context, calls } = fakeContext();
    const canvas = canvasWith(context);
    drawComposition(canvas, portrait, 16 / 9, WHOLE_IMAGE);
    expect([canvas.width, canvas.height]).toEqual([2048, 1152]);
    expect(calls).toEqual([
      "fill black",
      "draw alpha=0.5 filter=blur(61px)",
      "draw alpha=1 filter=none",
    ]);
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      736,
      0,
      576,
      1152,
    );
  });

  it("枠を覆うと余白が無く、黒で塗ってから画像だけを描く", () => {
    const { context, calls } = fakeContext();
    drawComposition(
      canvasWith(context),
      portrait,
      16 / 9,
      fillFrame(portrait, wide, WHOLE_IMAGE),
    );
    expect(calls).toEqual(["fill black", "draw alpha=1 filter=none"]);
    const [, x, y, width, height] = context.drawImage.mock.lastCall as [
      unknown,
      number,
      number,
      number,
      number,
    ];
    expect([x, y, width, height].map(Math.round)).toEqual([0, -1472, 2048, 4096]);
  });

  it("余白を黒にすると、ぼかした画像を描かない", () => {
    const { context, calls } = fakeContext();
    drawComposition(canvasWith(context), portrait, 16 / 9, {
      ...WHOLE_IMAGE,
      backdrop: "black",
    });
    expect(calls).toEqual(["fill black", "draw alpha=1 filter=none"]);
  });

  it("回すと置いた枠の中心で回し、元の向きの大きさで描く", () => {
    const { context } = fakeContext();
    const fit: Fit = { ...WHOLE_IMAGE, rotation: 90, backdrop: "black" };
    drawComposition(canvasWith(context), portrait, 16 / 9, fit);
    expect(context.translate).toHaveBeenLastCalledWith(1024, 576);
    expect(context.rotate).toHaveBeenLastCalledWith(Math.PI / 2);
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      -512,
      -1024,
      1024,
      2048,
    );
  });

  it("ぼかしを描けない文脈では余白は黒だけで、文脈が無ければ何もしない", () => {
    const { context, calls } = fakeContext(false);
    drawComposition(canvasWith(context), portrait, 16 / 9, WHOLE_IMAGE);
    expect(calls).toEqual(["fill black", "draw alpha=1 filter=-"]);
    expect(() =>
      drawComposition(canvasWith(null), portrait, 16 / 9, WHOLE_IMAGE),
    ).not.toThrow();
  });
});

describe("drawOverflow", () => {
  it("枠を真ん中に置いた 3 倍の広さに、縮めて画像だけを描く", () => {
    const { context, calls } = fakeContext();
    const canvas = canvasWith(context);
    drawOverflow(canvas, portrait, 16 / 9, WHOLE_IMAGE, 0.25);
    expect([canvas.width, canvas.height]).toEqual([1536, 864]);
    expect(calls).toEqual(["clear", "draw alpha=1 filter=none"]);
    expect(context.scale).toHaveBeenCalledWith(0.25, 0.25);
    expect(context.translate).toHaveBeenCalledWith(2048, 1152);
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      736,
      0,
      576,
      1152,
    );
  });
});

describe("encodeComposition", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("同じ絵を PNG にし、PNG にできなければ失敗する", async () => {
    const { context } = fakeContext();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
      context as unknown as RenderingContext,
    );
    const png = new Blob(["png"], { type: "image/png" });
    const toBlob = vi
      .spyOn(HTMLCanvasElement.prototype, "toBlob")
      .mockImplementationOnce((done) => done(png))
      .mockImplementationOnce((done) => done(null));

    await expect(encodeComposition(portrait, 16 / 9, WHOLE_IMAGE)).resolves.toBe(png);
    expect(toBlob).toHaveBeenLastCalledWith(expect.any(Function), "image/png");
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      736,
      0,
      576,
      1152,
    );
    await expect(encodeComposition(portrait, 16 / 9, WHOLE_IMAGE)).rejects.toThrow();
  });
});
