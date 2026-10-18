/**
 * libraryPath はライブラリの画面の場所である（docs/design-docs/home.md）。
 * トップ（`/`）はホームに明け渡し、ライブラリは `/library` に置く。問い合わせ
 * （`tag`・`person`・`organization` など）は前と同じ。
 */

/** LIBRARY_PATH はライブラリの画面の経路である。 */
export const LIBRARY_PATH = "/library";

/**
 * libraryFilterPath は、問い合わせ `param` を `id` 1 つで絞ったライブラリの場所である
 * （`/library?tag=3` など）。
 */
export function libraryFilterPath(param: string, id: number): string {
  return `${LIBRARY_PATH}?${param}=${String(id)}`;
}
