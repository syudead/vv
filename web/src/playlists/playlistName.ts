// プレイリストの名前の規則（specs/044-playlists/research.md R-11）。サーバーと同じく、前後の
// 空白を取り除いた名前をコードポイントで数える。HTML の maxLength は UTF-16 の単位で数えるので
// 使わない（ui-design.md「New playlist」）。

/** playlistNameLimit は名前の最大の長さ（コードポイント）である。 */
export const playlistNameLimit = 100;

/**
 * playlistName は送る名前（トリムしたもの）を返す。空か 100 コードポイントを超えるなら null で、
 * 画面は送らずに名前の規則を欄の誤りとして出す。
 */
export function playlistName(text: string): string | null {
  const name = text.trim();
  const length = Array.from(name).length;
  if (length === 0 || length > playlistNameLimit) return null;
  return name;
}
