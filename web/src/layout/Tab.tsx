import { NavLink } from "react-router";

import type { InertNavItem, LiveNavItem, NavItem } from "./navigation";

/**
 * Tab はヘッダーに並ぶ 1 つである（C6 / FR-005）。
 *
 * 規約は NavItem（C3）と同じものを共有する（contracts/components.md 3.）──
 * `live` だけが対話要素になり、`inert` は `<span>` で `--color-inert`、ラベルの
 * 直後に「（未実装）」を添える。違うのは**選択中の示し方**だけで、サイドバーが
 * 地で示すのに対しタブは**下線**で示す。
 *
 * `live` は「動画」の 1 つだけである（navigation.ts）。この部品は「どれが
 * 機能するか」を知らず、`kind` で分岐するだけである。
 *
 * **件数を受け取る口を持たない。** タブに件数を出すのは原案にも spec にも
 * 無い（FR-005 が件数を出すのは NavItem「すべての動画」の 1 行だけと定めている）。
 */

/** tab は 2 形が共有する骨格である。状態の表現は含めない（NavItem と同じ理由）。 */
const tab =
  "relative flex min-h-[var(--size-tap)] items-center gap-2 px-1 text-sm whitespace-nowrap";

/**
 * states は操作できる要素の 4 状態である（contracts/components.md 2.）。
 *
 * 下線は選択中の印なので、hover / active は**文字の明るさ**で示す。帯の中の
 * 要素に地を敷くと、隣り合うタブの間隔が原案より詰まって見える。動きを減らす
 * 設定では遷移だけを止め、色の最終状態は常に適用する（FR-012）。
 */
const states =
  "text-muted transition-colors hover:text-body active:text-muted " +
  "motion-reduce:transition-none " +
  "rounded-control outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

export default function Tab({ item }: { item: NavItem }) {
  if (item.kind === "inert") {
    return <InertTab item={item} />;
  }
  return <LiveTab item={item} />;
}

/** LiveTab は行き先を持つタブである。選択中は下線と `--color-accent` で示す。 */
function LiveTab({ item }: { item: LiveNavItem }) {
  return (
    <NavLink
      to={item.to}
      // `to` が "/" なので、`end` が無いとすべての経路で選択中になる。
      end
      data-nav-id={item.id}
      className={({ isActive }) =>
        `${tab} ${states} ` +
        (isActive
          ? "text-accent after:absolute after:inset-x-1 after:-bottom-px after:h-0.5 after:rounded-control after:bg-accent"
          : "")
      }
    >
      {item.label}
    </NavLink>
  );
}

/** InertTab は表示のみのタブである。下線も 4 状態も持たない。 */
function InertTab({ item }: { item: InertNavItem }) {
  return (
    <span data-nav-id={item.id} className={`${tab} text-inert`}>
      {item.label}
      <span className="sr-only">（未実装）</span>
    </span>
  );
}
