/**
 * cardLinkState は動画とグループのカードのリンクが開いた先へ渡す状態である。`from` は戻る先の
 * 一覧の場所で、`autoplay` を渡すと再生画面が再生を始める（ホームの棚のカード。
 * specs/057-home/research.md R-14）。
 */
export function cardLinkState(
  backTo: string,
  autoplay: boolean | undefined,
): { from: string; autoplay?: true } {
  return autoplay === true ? { from: backTo, autoplay: true } : { from: backTo };
}
