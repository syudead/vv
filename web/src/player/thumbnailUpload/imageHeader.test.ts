import { describe, expect, it } from "vitest";

import { readImageHeader } from "./imageHeader";

function bytes(...parts: (number[] | string)[]): Uint8Array {
  const values = parts.flatMap((part) =>
    typeof part === "string" ? [...part].map((char) => char.charCodeAt(0)) : part,
  );
  return new Uint8Array(values);
}

const be32 = (value: number) => [
  value >>> 24,
  (value >>> 16) & 255,
  (value >>> 8) & 255,
  value & 255,
];
const be16 = (value: number) => [value >>> 8, value & 255];
const le16 = (value: number) => [value & 255, value >>> 8];
const le24 = (value: number) => [value & 255, (value >>> 8) & 255, value >>> 16];

describe("readImageHeader", () => {
  it("PNG の IHDR から大きさを読む", () => {
    const png = bytes(
      [0x89],
      "PNG\r\n\x1a\n",
      be32(13),
      "IHDR",
      be32(12000),
      be32(11000),
    );
    expect(readImageHeader(png)).toEqual({
      width: 12000,
      height: 11000,
      animated: false,
    });
  });

  it("JPEG は前置きの区切りを飛ばし、SOF から大きさを読む", () => {
    const jpeg = bytes(
      [0xff, 0xd8],
      [0xff, 0xe1],
      be16(6),
      [1, 2, 3, 4],
      [0xff, 0xc4],
      be16(4),
      [0, 0],
      [0xff, 0xc0],
      be16(17),
      [8],
      be16(3000),
      be16(4000),
      [3],
    );
    expect(readImageHeader(jpeg)).toEqual({ width: 4000, height: 3000, animated: false });
  });

  it("WebP の VP8X はアニメーションの印と大きさを、VP8L・VP8 は大きさを読む", () => {
    const vp8x = (flags: number) =>
      bytes(
        "RIFF",
        [0, 0, 0, 0],
        "WEBP",
        "VP8X",
        [10, 0, 0, 0],
        [flags, 0, 0, 0],
        le24(1919),
        le24(1079),
      );
    expect(readImageHeader(vp8x(0x02))).toEqual({
      width: 1920,
      height: 1080,
      animated: true,
    });
    expect(readImageHeader(vp8x(0))).toMatchObject({ animated: false });

    const bits = 99 | (49 << 14);
    const vp8l = bytes(
      "RIFF",
      [0, 0, 0, 0],
      "WEBP",
      "VP8L",
      [5, 0, 0, 0],
      [0x2f],
      [bits & 255, (bits >>> 8) & 255, (bits >>> 16) & 255, bits >>> 24],
    );
    expect(readImageHeader(vp8l)).toEqual({ width: 100, height: 50, animated: false });

    const vp8 = bytes(
      "RIFF",
      [0, 0, 0, 0],
      "WEBP",
      "VP8 ",
      [10, 0, 0, 0],
      [0, 0, 0, 0x9d, 0x01, 0x2a],
      le16(640),
      le16(360),
    );
    expect(readImageHeader(vp8)).toEqual({ width: 640, height: 360, animated: false });
  });

  it("形式が分からないか大きさが見つからなければ null を返す", () => {
    expect(readImageHeader(bytes("GIF89a"))).toBeNull();
    expect(readImageHeader(bytes([0xff, 0xd8], [0xff, 0xe1], be16(100)))).toBeNull();
  });
});
