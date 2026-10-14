/**
 * ImageHeader は画像の見出しから読んだ大きさと、アニメーションの WebP かどうかである。
 * 大きさは向き（EXIF）を反映しない保存上の値で、画素数を数えるのに使う。
 */
export interface ImageHeader {
  width: number;
  height: number;
  animated: boolean;
}

/**
 * HEADER_READ_BYTES は見出しを探す先頭のバイト数である。JPEG は EXIF などの前置きの後ろに
 * 大きさを置くので、前置きが長くても届くよう広めに読む。
 */
export const HEADER_READ_BYTES = 1 << 20;

/**
 * readImageHeader は PNG・JPEG・WebP の先頭 `bytes` から見出しを読む。形式が分からないか、
 * 先頭に大きさが見つからなければ `null` を返す。サーバーの確かめ（PNG の IHDR、JPEG の SOF、
 * WebP の VP8・VP8L・VP8X、VP8X のアニメーションの印）と同じところを読み、プレビューを出す前に
 * 断れるようにする。
 */
export function readImageHeader(bytes: Uint8Array): ImageHeader | null {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (startsWith(bytes, [0x89, 0x50, 0x4e, 0x47]) && bytes.length >= 24)
    return { width: view.getUint32(16), height: view.getUint32(20), animated: false };
  if (startsWith(bytes, [0xff, 0xd8])) return readJpeg(bytes, view);
  if (ascii(bytes, 0, 4) === "RIFF" && ascii(bytes, 8, 12) === "WEBP")
    return readWebP(bytes, view);
  return null;
}

function readJpeg(bytes: Uint8Array, view: DataView): ImageHeader | null {
  let offset = 2;
  while (offset + 9 <= bytes.length) {
    if (bytes[offset] !== 0xff) return null;
    const marker = bytes[offset + 1] ?? 0;
    // 区切りの前の詰め物（0xFF の連続）は飛ばす。
    if (marker === 0xff) {
      offset += 1;
      continue;
    }
    // SOF0〜SOF15 のうち、DHT（C4）・JPG（C8）・DAC（CC）を除いたものが大きさを持つ。
    if (
      marker >= 0xc0 &&
      marker <= 0xcf &&
      marker !== 0xc4 &&
      marker !== 0xc8 &&
      marker !== 0xcc
    )
      return {
        width: view.getUint16(offset + 7),
        height: view.getUint16(offset + 5),
        animated: false,
      };
    offset += 2 + view.getUint16(offset + 2);
  }
  return null;
}

function readWebP(bytes: Uint8Array, view: DataView): ImageHeader | null {
  const chunk = ascii(bytes, 12, 16);
  if (chunk === "VP8X" && bytes.length >= 30)
    return {
      width: 1 + uint24(bytes, 24),
      height: 1 + uint24(bytes, 27),
      animated: ((bytes[20] ?? 0) & 0x02) !== 0,
    };
  if (chunk === "VP8L" && bytes.length >= 25) {
    const bits = view.getUint32(21, true);
    return {
      width: 1 + (bits & 0x3fff),
      height: 1 + ((bits >> 14) & 0x3fff),
      animated: false,
    };
  }
  if (chunk === "VP8 " && bytes.length >= 30)
    return {
      width: view.getUint16(26, true) & 0x3fff,
      height: view.getUint16(28, true) & 0x3fff,
      animated: false,
    };
  return null;
}

function startsWith(bytes: Uint8Array, prefix: number[]): boolean {
  return prefix.every((value, index) => bytes[index] === value);
}

function ascii(bytes: Uint8Array, start: number, end: number): string {
  return String.fromCharCode(...bytes.subarray(start, end));
}

function uint24(bytes: Uint8Array, offset: number): number {
  return (
    (bytes[offset] ?? 0) |
    ((bytes[offset + 1] ?? 0) << 8) |
    ((bytes[offset + 2] ?? 0) << 16)
  );
}
