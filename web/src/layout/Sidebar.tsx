import Logo from "./Logo";

/**
 * Sidebar は左に固定される縦の帯である（C1 / FR-001・FR-003）。
 *
 * `position: fixed` にするのは、コンテンツがどれだけ流れても動かないこと
 * （FR-002）をビューポート基準で満たすためである。`sticky` は親の中でしか
 * 粘らないので、画面の高さいっぱいに居続ける帯には向かない（R-501）。
 *
 * **スクロールの持ち主は文書（ウィンドウ）のままである。** ここで独立して
 * 流れるのはサイドバーの中身だけで、コンテンツ側にスクロール容器を作らない
 * ── 004 が積んだ復元・密度アンカー・無限スクロールの 3 つが
 * `window.scrollY` とビューポート基準の観測に載っているからである（R-501）。
 *
 * 幅 640px 未満では **`display: none`** で消す（`hidden sm:block`）。
 * `visibility` や `opacity` を使わないのは、支援技術からも消えることが
 * ロゴの二重配置の前提だからである（contracts/layout.md 3.）。
 *
 * 中身（ライブラリ・コレクション・タグの項目）は US2 で入る。
 */
export default function Sidebar() {
  return (
    <aside
      className={
        "fixed inset-y-0 left-0 z-30 hidden h-dvh w-[var(--size-sidebar)] " +
        "overflow-y-auto border-r border-border bg-surface-raised sm:block"
      }
    >
      <div className="px-4 py-3">
        <Logo />
      </div>
    </aside>
  );
}
