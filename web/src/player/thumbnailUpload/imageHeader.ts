/** Header dimensions exclude EXIF orientation and are used for pixel-limit checks. */
export interface ImageHeader {
  width: number;
  height: number;
  animated: boolean;
}

/** Read enough header bytes to find JPEG dimensions after large EXIF prefixes. */
export const HEADER_READ_BYTES = 1 << 20;

/** Mirror the server's PNG/JPEG/WebP header checks; return null for unknown formats or absent dimensions. */
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
    // Skip repeated 0xFF marker padding.
    if (marker === 0xff) {
      offset += 1;
      continue;
    }
    // SOF0-SOF15 carry dimensions except DHT (C4), JPG (C8), and DAC (CC).
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
