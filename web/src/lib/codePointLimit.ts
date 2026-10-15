/**
 * limitCodePoints は入力欄の値を符号位置（コードポイント）で max 個までに収める。HTML の
 * maxLength は UTF-16 の単位で数えるので、絵文字などのサロゲートペアを 2 と数え、符号位置で
 * 数えるサーバーの上限より早く入力を止めてしまう。
 *
 * 上限を超えたときは、入力前の値と比べて新しく入った部分だけを切る。先頭や途中への
 * 入力で、もとからあった末尾を消さないためである。上限を超える貼り付けも、入る分
 * だけが入る。
 *
 * `caret` は入力後のカーソル位置（UTF-16 の単位）で、新しく入った部分はその手前で
 * 終わる。選択範囲を、選んだ文字で終わる語に置き換えたときも、入った部分を既存の
 * 末尾と取り違えないよう、共通の先頭と末尾をカーソルの前後に収める。
 */
export function limitCodePoints(
  next: string,
  max: number,
  previous = "",
  caret?: number,
): string {
  const after = Array.from(next);
  if (after.length <= max) return next;
  const before = Array.from(previous);
  // カーソル位置が分からないときは、先頭と末尾を縛らない。
  const cursor =
    caret === undefined ? undefined : Array.from(next.slice(0, caret)).length;
  let prefix = 0;
  while (
    prefix < before.length &&
    prefix < (cursor ?? after.length) &&
    before[prefix] === after[prefix]
  ) {
    prefix++;
  }
  let suffix = 0;
  while (
    suffix < before.length - prefix &&
    suffix < after.length - Math.max(prefix, cursor ?? prefix) &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  ) {
    suffix++;
  }
  const inserted = after.slice(prefix, after.length - suffix);
  const room = Math.max(max - prefix - suffix, 0);
  return [
    ...after.slice(0, prefix),
    ...inserted.slice(0, room),
    ...after.slice(after.length - suffix),
  ]
    .slice(0, max)
    .join("");
}
