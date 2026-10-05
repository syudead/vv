import type { ReactNode, Ref } from "react";

// ページ見出し（区画）。題・件数・説明と、ページの主操作を 1 行に置く。前に戻る操作や
// パンくずは leading に入れる。外の余白は骨格が持つので、この区画は持たない。
// variant="list" は一覧ページ（ListPage）の見出しで、大きな題を左に、件数を同じ行の
// 右端に置く。規則は web/registry/rules/patterns.md の Sections。

export interface PageHeaderProps {
  /** ページの題。h1 になる。 */
  title: ReactNode;
  /** 件数（「128 videos」）。題の後ろに小さく置く。 */
  count?: ReactNode;
  /** 題の下の 1 行の説明。 */
  description?: ReactNode;
  /** 題の上に置く戻る操作やパンくず。 */
  leading?: ReactNode;
  /** ページの主操作。default のボタンは 1 つまで。 */
  actions?: ReactNode;
  /**
   * 題の h1。渡すと h1 をフォーカスできるようにする（tabIndex -1）。一覧の最後の項目を
   * 片付けた後など、ほかにフォーカスを置く先が無いときに使う。
   */
  titleRef?: Ref<HTMLHeadingElement>;
  /**
   * 形。default は題の後ろに小さく件数を置く。list は一覧ページの形で、題を sm から
   * text-page-title に大きくし、件数を行の右端に置く（description と actions は持たない）。
   */
  variant?: "default" | "list";
}

export function PageHeader({
  title,
  count,
  description,
  leading,
  actions,
  titleRef,
  variant = "default",
}: PageHeaderProps) {
  if (variant === "list") {
    return (
      <header
        data-slot="page-header"
        data-variant="list"
        className="flex min-w-0 items-baseline justify-between gap-3"
      >
        <h1
          ref={titleRef}
          tabIndex={titleRef === undefined ? undefined : -1}
          className="min-w-0 truncate text-xl font-semibold tracking-tight sm:text-page-title"
        >
          {title}
        </h1>
        {count !== undefined && (
          <span
            data-slot="page-header-count"
            className="shrink-0 text-xs text-muted-foreground tabular-nums sm:text-sm"
          >
            {count}
          </span>
        )}
      </header>
    );
  }
  return (
    <header data-slot="page-header" className="flex flex-col gap-2">
      {leading && (
        <div data-slot="page-header-leading" className="flex items-center gap-2">
          {leading}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 items-baseline gap-2">
            <h1
              ref={titleRef}
              tabIndex={titleRef === undefined ? undefined : -1}
              className="min-w-0 truncate text-xl font-semibold"
            >
              {title}
            </h1>
            {count !== undefined && (
              <span
                data-slot="page-header-count"
                className="shrink-0 text-xs text-muted-foreground tabular-nums"
              >
                {count}
              </span>
            )}
          </div>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && (
          <div
            data-slot="page-header-actions"
            className="flex shrink-0 items-center gap-2"
          >
            {actions}
          </div>
        )}
      </div>
    </header>
  );
}
