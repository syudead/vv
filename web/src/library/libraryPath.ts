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

/**
 * UNSHELVED_PARAM はホームのどの棚にも一致しない動画だけに絞る問い合わせである
 * （`/library?unshelved=1`。docs/design-docs/home.md、ui-design.md「Home」）。
 */
export const UNSHELVED_PARAM = "unshelved";

/** UNSHELVED_PATH はホームの「Not on any shelf」の「See all」が開くライブラリの場所である。 */
export const UNSHELVED_PATH = `${LIBRARY_PATH}?${UNSHELVED_PARAM}=1`;

/** readUnshelved は問い合わせ部分が「棚に無い動画のみ」を名指すか（`unshelved=1`）を返す。 */
export function readUnshelved(search: string): boolean {
  return new URLSearchParams(search).get(UNSHELVED_PARAM) === "1";
}
