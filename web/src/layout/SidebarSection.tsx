import type { ReactNode } from "react";

/**
 * SidebarSection は見出しと項目群を受ける器である（C4 / FR-004）。
 *
 * 見出しは `<h2>`、項目群は `<ul>` / `<li>` にする。`h1` はロゴが持つ（R-508）
 * ので、区画の見出しは 2 階層目である。
 *
 * **見出しを省ける形にする。** 原案のライブラリの区画には見出し文字が無い ──
 * そこに「ライブラリ」と書くのは原案に無いものを足すことであり、空の `<h2>` を
 * 置くのは読み上げに無音の見出しを差し込むことである。どちらも避ける。
 */
export default function SidebarSection({
  title,
  children,
}: {
  /** 省くと見出しを描かない（原案のライブラリの区画） */
  title?: string;
  /** NavItem を包んだ `<li>` の並び */
  children: ReactNode;
}) {
  return (
    <section className="px-2 py-2">
      {title !== undefined && (
        <h2 className="px-3 pb-1 text-xs font-semibold tracking-wide text-muted">
          {title}
        </h2>
      )}
      <ul className="flex flex-col gap-0.5">{children}</ul>
    </section>
  );
}
