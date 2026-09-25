// 見る人が変わるとき（初回設定・ログイン・ログアウト）は、SPA の遷移にせず
// ページごと移る。前の相手として読んだ控えをメモリに残さないためである
// （specs/016-single-account-auth/plan.md Structural Decisions 14）。
// テストで差し替えられるよう、ここに閉じる。

/** assignPage は url へページごと移る。 */
export function assignPage(url: string): void {
  window.location.assign(url);
}

/** reloadPage は今の URL をページごと読み直す。 */
export function reloadPage(): void {
  window.location.reload();
}

/** currentPath は今の URL のパス・問い合わせ・断片をつないだ値である。 */
export function currentPath(location: {
  pathname: string;
  search: string;
  hash: string;
}): string {
  return `${location.pathname}${location.search}${location.hash}`;
}

/** loginPath は、ログインの後に next へ戻るログイン画面の URL である。 */
export function loginPath(next: string): string {
  return `/login?${new URLSearchParams({ next })}`;
}
