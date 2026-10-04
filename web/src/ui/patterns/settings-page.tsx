import type { ReactNode } from "react";

// 設定ページ（骨格）。最大幅を固定した 1 列に、見出しと節（PageSection）を並べる。
// 節の間隔は骨格が持つ。規則は web/registry/rules/patterns.md の Settings page。

export interface SettingsPageProps {
  /** PageHeader（題と説明）。 */
  header: ReactNode;
  /** PageSection の並び。 */
  children: ReactNode;
}

export function SettingsPage({ header, children }: SettingsPageProps) {
  return (
    <div
      data-slot="settings-page"
      className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-3 sm:p-4"
    >
      {header}
      <div data-slot="settings-page-body" className="flex flex-col gap-8">
        {children}
      </div>
    </div>
  );
}
