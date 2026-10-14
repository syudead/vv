import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  drawComposition,
  encodeComposition,
  loadUploadImage,
  type UploadImage,
} from "./composeImage";

// jsdom には canvas の描画も画像の復号も無いので、2D の文脈と createImageBitmap を偽物にする。

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

/** blob は先頭が source のファイルである。jsdom の Blob に無い arrayBuffer を足す。 */
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

/** fakeContext は呼ばれた描画を記録する 2D の文脈である。 */
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

describe("drawComposition", () => {
  it("動画の比率の大きさにし、黒で塗ってから、余白にぼかした画像、手前に画像を描く", () => {
    const { context, calls } = fakeContext();
    const canvas = canvasWith(context);
    drawComposition(canvas, portrait, 16 / 9, { mode: "contain" });
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

  it("枠いっぱいに広げると余白が無く、黒で塗ってから画像だけを描く", () => {
    const { context, calls } = fakeContext();
    drawComposition(canvasWith(context), portrait, 16 / 9, {
      mode: "cover",
      zoom: 1,
      panX: 0,
      panY: 1,
    });
    expect(calls).toEqual(["fill black", "draw alpha=1 filter=none"]);
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      0,
      1152 - 4096,
      2048,
      4096,
    );
  });

  it("ぼかしを描けない文脈では余白は黒だけで、文脈が無ければ何もしない", () => {
    const { context, calls } = fakeContext(false);
    drawComposition(canvasWith(context), portrait, 16 / 9, { mode: "contain" });
    expect(calls).toEqual(["fill black", "draw alpha=1 filter=-"]);
    expect(() =>
      drawComposition(canvasWith(null), portrait, 16 / 9, { mode: "contain" }),
    ).not.toThrow();
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

    await expect(encodeComposition(portrait, 16 / 9, { mode: "contain" })).resolves.toBe(
      png,
    );
    expect(toBlob).toHaveBeenLastCalledWith(expect.any(Function), "image/png");
    expect(context.drawImage).toHaveBeenLastCalledWith(
      portrait.bitmap,
      736,
      0,
      576,
      1152,
    );
    await expect(
      encodeComposition(portrait, 16 / 9, { mode: "contain" }),
    ).rejects.toThrow();
  });
});
