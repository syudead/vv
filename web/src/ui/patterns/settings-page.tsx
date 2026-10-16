import type { ReactNode } from "react";
import { useId } from "react";
import { Slot } from "radix-ui";

import { cn } from "@/lib/cn";

// 設定ページ（骨格）。最大幅を固定した 1 列に、見出しと節（PageSection）を並べる。
// 節の間隔は骨格が持つ。nav（ページの一覧、SettingsNav）を渡すと、sm からは settings-nav の
// 幅の列を本文の左に置き、トップバーの下に貼り付ける。sm 未満は view が "list" なら一覧だけ、
// "page" なら本文だけを出す（幅は CSS が決め、useIsMobile は使わない）。本文だけのときの
// 一覧へ戻るリンクは header の PageHeader の leading に置き、骨格が sm から隠す。
// 規則は web/registry/rules/patterns.md の Settings page。

export interface SettingsPageProps {
  /** PageHeader（題と説明）。nav があるときは、leading に一覧へ戻るリンクだけを置く。 */
  header: ReactNode;
  /** PageSection の並び。 */
  children: ReactNode;
  /** ページの一覧（SettingsNav）。渡すと最大幅が max-w-4xl になり、sm から左の列になる。 */
  nav?: ReactNode;
  /**
   * sm 未満で何を出すか。"list" は一覧だけ（一覧の住所）、"page" は本文だけ（各ページの住所）。
   * sm からはどちらも一覧と本文を並べる。nav が無いときは使わない。
   */
  view?: "list" | "page";
}

export function SettingsPage({
  header,
  children,
  nav,
  view = "page",
}: SettingsPageProps) {
  const body = (
    <div data-slot="settings-page-body" className="flex flex-col gap-8">
      {children}
    </div>
  );
  if (nav === undefined) {
    return (
      <div
        data-slot="settings-page"
        className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-3 sm:p-4"
      >
        {header}
        {body}
      </div>
    );
  }
  const listView = view === "list";
  return (
    <div
      data-slot="settings-page"
      data-view={view}
      className="mx-auto w-full max-w-4xl p-3 sm:grid sm:grid-cols-settings-page sm:items-start sm:gap-6 sm:p-4"
    >
      <div
        data-slot="settings-page-nav"
        className={cn("min-w-0 sm:sticky sm:top-navbar", !listView && "hidden sm:block")}
      >
        {nav}
      </div>
      <div
        data-slot="settings-page-main"
        className={cn(
          "min-w-0 flex-col gap-6 sm:flex sm:**:data-[slot=page-header-leading]:hidden",
          listView ? "hidden" : "flex",
        )}
      >
        {header}
        {body}
      </div>
    </div>
  );
}

export interface SettingsNavProps {
  /** 一覧の見出し。nav の読み上げ名にもなる（「Settings」）。 */
  label: string;
  /** SettingsNavItem の並び。 */
  children: ReactNode;
}

/**
 * SettingsNav は設定ページの一覧（SettingsPage の nav）である。sm からは列の上の小さな見出し、
 * sm 未満で一覧だけを出すときはページの題の大きさの見出しになる。
 */
export function SettingsNav({ label, children }: SettingsNavProps) {
  const headingId = useId();
  return (
    <nav
      data-slot="settings-nav"
      aria-labelledby={headingId}
      className="flex flex-col gap-6 sm:gap-2"
    >
      <h2
        id={headingId}
        className="text-xl font-semibold sm:px-2 sm:text-sm sm:text-muted-foreground"
      >
        {label}
      </h2>
      {/* sm 未満は行の文字を見出しの左端にそろえ、塗りだけを余白へはみ出させる。 */}
      <ul className="-mx-2 flex flex-col gap-1 sm:mx-0">{children}</ul>
    </nav>
  );
}

export interface SettingsNavItemProps {
  /** 今のページの項目。aria-current="page" を付け、sm から塗る（sm 未満は印を出さない）。 */
  current?: boolean;
  /** 1 つのリンク（ルーターのリンク）。骨格が行の見た目を足す。 */
  children: ReactNode;
}

/** SettingsNavItem は一覧の 1 行である。子のリンクに行の見た目と aria-current を足す。 */
export function SettingsNavItem({ current = false, children }: SettingsNavItemProps) {
  return (
    <li>
      <Slot.Root
        data-slot="settings-nav-item"
        data-current={current || undefined}
        aria-current={current ? "page" : undefined}
        className="flex h-8 w-full items-center rounded-md px-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:data-current:bg-secondary sm:data-current:font-medium sm:data-current:text-foreground"
      >
        {children}
      </Slot.Root>
    </li>
  );
}
