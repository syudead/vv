import { NavLink } from "react-router";

import Icon from "./icons";
import type { InertNavItem, LiveNavItem } from "./navigation";

/**
 * NavItem はサイドバーの 1 行である（C3 / FR-005・FR-007）。
 *
 * `kind` で 2 形に分かれる。**分岐の根拠は navigation.ts の表だけ**で、ここは
 * 「どれが機能するか」を知らない（contracts/components.md 3.「機能が付くときの
 * 手順」）。表の行を live に変えれば、この部品を書き換えずに押せるようになる。
 *
 * `inert` は **`<span>` で描き、`disabled` も `aria-disabled` も `role="button"` も
 * `tabIndex` も付けない**（R-503）。「いまは使えない」という誤った意味を持たせない
 * ためである ── 裏側の機能そのものが無いので、条件は永遠に揃わない。
 */

/**
 * props は判別可能な合併である。
 *
 * **`inert` に件数を渡す口を作らない**（FR-005 / T021）。`count?: never` なので、
 * 表示のみの行へ件数を渡すと**型で落ちる**。表示のみの要素に件数が出ないことを
 * 呼び出し側の注意ではなく機械で守るための宣言である。
 */
type NavItemProps =
  | {
      item: LiveNavItem;
      /**
       * count は右端に出す件数である。**未取得・取り直しの最中は `undefined`** で
       * 渡され、そのときは何も出さない（T021）。0 と「まだ分からない」を同じ
       * 見た目にしない。
       */
      count?: number;
    }
  | { item: InertNavItem; count?: never };

/**
 * row は 2 形が共有する骨格である。**状態の表現は含めない** ── 表示のみの行は
 * hover でも変化しない（contracts/components.md 3.）ので、変化するクラスを
 * 共有側に置くと押せないものが手応えを返す。
 *
 * 当たり判定は `--size-tap`（44px）四方以上を保つ（FR-018 が 004 の FR-022 を
 * 引き継ぐ）。
 */
const row =
  "flex min-h-[var(--size-tap)] items-center gap-2.5 rounded-control px-3 text-sm";

/**
 * states は操作できる要素の 4 状態である（contracts/components.md 2.）。
 *
 * hover と active は **`::after` の覆い**で表す。「1 段明るく / 暗く」は地の色に
 * 対する相対の指示だが、選択中（`accent-surface`）と通常（サイドバーの
 * `surface-raised`）では地が違う ── 覆いなら、どちらの地の上でも同じ向きに
 * 1 段動き、選択中の色みも消えない。`isolate` + `-z-10` で覆いを中身の下に
 * 敷くので、文字とアイコンは覆われない。
 *
 * 動きを減らす設定では遷移だけを止め、**色の最終状態は常に適用する**
 * （FR-012）。操作の結果は判別できる。
 */
const states =
  "relative isolate after:pointer-events-none after:absolute after:inset-0 " +
  "after:-z-10 after:rounded-control after:transition-colors " +
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 " +
  "motion-reduce:after:transition-none " +
  "outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

export default function NavItem(props: NavItemProps) {
  if (props.item.kind === "inert") {
    return <InertRow item={props.item} />;
  }
  return <LiveRow item={props.item} count={props.count} />;
}

/** LiveRow は行き先を持つ 1 行である。選択中は地と文字の両方で示す。 */
function LiveRow({ item, count }: { item: LiveNavItem; count: number | undefined }) {
  return (
    <NavLink
      to={item.to}
      // `to` が "/" なので、`end` が無いとすべての経路で選択中になる。
      end
      data-nav-id={item.id}
      className={({ isActive }) =>
        `${row} ${states} ` + (isActive ? "bg-accent-surface text-accent" : "text-body")
      }
    >
      <Icon name={item.icon} />
      <span className="flex-1 truncate">{item.label}</span>
      {count !== undefined && (
        <span className="text-xs text-muted tabular-nums">{count}</span>
      )}
    </NavLink>
  );
}

/**
 * InertRow は表示のみの 1 行である。
 *
 * 色は `--color-inert` で、機能する行より淡い。**件数は出さない**（FR-005）し、
 * hover でも変化しない。見えない利用者にも同じ区別が届くよう、ラベルの直後に
 * 「（未実装）」を添える（FR-006）。
 */
function InertRow({ item }: { item: InertNavItem }) {
  return (
    <span data-nav-id={item.id} className={`${row} text-inert`}>
      <Icon name={item.icon} />
      <span className="flex-1 truncate">{item.label}</span>
      <span className="sr-only">（未実装）</span>
    </span>
  );
}
