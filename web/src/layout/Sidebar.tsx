import { useVideoCount } from "./AppShell";
import Logo from "./Logo";
import NavItem from "./NavItem";
import { itemsIn, type NavSection } from "./navigation";
import SidebarSection from "./SidebarSection";

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
 * 中身は navigation.ts の表を**順序のまま**描くだけである（T019）。どの行が
 * 機能するかはこのファイルに書かれていない ── NavItem が `kind` で分岐する。
 */

/** sections は原案の区画とその見出しである。ライブラリの見出し文字は原案に無い。 */
const sections: { section: NavSection; title?: string }[] = [
  { section: "library" },
  { section: "collection", title: "コレクション" },
  { section: "tag", title: "タグ" },
];

export default function Sidebar() {
  // 件数は骨格の中を子（LibraryPage）から流れてくる（AppShell の context）。
  // 一覧がまだ読めていない間は undefined で、そのときは件数を出さない。
  const count = useVideoCount();

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

      {sections.map(({ section, title }) => (
        <SidebarSection key={section} title={title}>
          {itemsIn(section).map((item) =>
            // 件数を渡せるのは「すべての動画」の 1 行だけである。表示のみの行に
            // 渡す口は NavItem の型が塞いでいるので、ここでの取り違えは型で落ちる
            // （FR-005 / T021）。
            item.kind === "live" ? (
              <li key={item.id}>
                <NavItem
                  item={item}
                  count={item.id === "all-videos" ? count : undefined}
                />
              </li>
            ) : (
              <li key={item.id}>
                <NavItem item={item} />
              </li>
            ),
          )}
        </SidebarSection>
      ))}
    </aside>
  );
}
