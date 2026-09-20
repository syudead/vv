/** cn は条件付きクラス名を結合する。falsy は捨てる。 */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
