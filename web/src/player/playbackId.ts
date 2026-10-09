/**
 * newPlaybackId は 1 回の視聴の識別子を、RFC 4122 version 4 の 36 文字の形で作る。
 *
 * crypto.randomUUID は安全な文脈にしか無く、LAN の素の http で開いた画面では使えないので、
 * crypto.getRandomValues の 16 バイトから組み立てる
 * （specs/043-watch-history/research.md R-2）。
 */
export function newPlaybackId(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  // version 4 と RFC 4122 の variant を立てる。
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40;
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join("-");
}
