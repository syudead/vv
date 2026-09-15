import type { ReactNode } from "react";

import Header from "./Header";
import Sidebar from "./Sidebar";

/**
 * AppShell は 3 領域の骨格である（FR-001 / FR-002 / contracts/layout.md 1.）。
 *
 * 左にサイドバー（C1）、上にヘッダー（C5）、残りが子（コンテンツ）である。
 * 骨格は**どの画面にも同じ形で存在し、画面の中身を知らない** ── どの画面を
 * 包むかを決めるのは `App.tsx` の経路の側である（FR-015。再生画面は包まない）。
 *
 * **内側にスクロール容器を作らない。** サイドバーとヘッダーは `fixed` で画面に
 * 固定されているので、コンテンツはその分を `padding` で避けるだけでよく、
 * スクロールの持ち主は文書（ウィンドウ）のままである（R-501）。ここに
 * `overflow-y: auto` の器を置くと、004 の復元・密度アンカー・無限スクロールが
 * 3 つとも書き換えになる。
 *
 * 重なりの順序はサイドバー・ヘッダー（z-30）がコンテンツより前、ツールバー
 * （z-20）がヘッダーより後ろである（contracts/layout.md 1.）。
 */
export default function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="pt-[var(--size-header)] sm:pl-[var(--size-sidebar)]">
      <Sidebar />
      <Header />
      {children}
    </div>
  );
}
