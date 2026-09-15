import Icon, { type IconName } from "./icons";

/**
 * IconButton は設定・フィルタ・表示切替の 3 つを描く（C7 / FR-005・FR-006）。
 *
 * **名前が Button であっても `<button>` にしない。** 3 つとも裏側の機能が無い
 * 表示のみの要素だからである（contracts/components.md 1. の表）。`disabled` や
 * `aria-disabled` を付けて `<button>` にすると「いまは使えない」という誤った
 * 意味を持つ ── 機能そのものが無いので、条件は永遠に揃わない（R-503）。
 * この部品は対話要素を作らず、`<span>` にアイコンと読み上げ用の文言を置く。
 *
 * 名前を Button のまま残すのは、contracts/components.md の C7 がこの名前で
 * 置き場所を定めているためである。機能が付く日には、この部品を `<button>` へ
 * 変えたうえで 4 状態を足す（そのときは FR-013 の見直しも要る）。
 */
export default function IconButton({
  name,
  label,
}: {
  name: IconName;
  /**
   * label は必須である。
   *
   * 3 か所で使い回す部品なので、文言を部品の中に固定すると漏斗も表示切替も
   * 「設定」と読まれ、**要素の区別が読み上げ利用者にだけ失われる**（FR-006）。
   * 見えている利用者はアイコンの形で区別できるので、この取り違えは目で見ても
   * 気づけない。
   */
  label: string;
}) {
  return (
    <span
      className={
        "inline-flex min-h-[var(--size-tap)] min-w-[var(--size-tap)] " +
        "items-center justify-center rounded-control text-inert"
      }
    >
      <Icon name={name} className="h-5 w-5" />
      <span className="sr-only">{label}（未実装）</span>
    </span>
  );
}
