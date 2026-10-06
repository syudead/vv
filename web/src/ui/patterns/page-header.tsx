import type { ReactNode, Ref } from "react";

// ページ見出し（区画）。題・件数・説明と、ページの主操作を 1 行に置く。前に戻る操作や
// パンくずは leading に入れる。外の余白は骨格が持つので、この区画は持たない。
// 規則は web/registry/rules/patterns.md の Page header。

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
   * 題を読み上げだけにする（sr-only）。パンくずの最後の段が同じ名前を見せる画面で、
   * 同じ名前を 2 度見せないために使う。
   */
  titleHidden?: boolean;
}

export function PageHeader({
  title,
  count,
  description,
  leading,
  actions,
  titleRef,
  titleHidden = false,
}: PageHeaderProps) {
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
              className={
                titleHidden ? "sr-only" : "min-w-0 truncate text-xl font-semibold"
              }
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
