import { SlidersHorizontal } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/ui/shadcn/popover";

// ツールバー（区画）。検索、絞り込みの操作、表示の切り替えと操作を 1 行に並べる。
// 置き場は 2 つ:
// - page（既定）: ページの中。狭い幅では折り返し、lg より狭い幅では表示の切り替えを
//   「View and sort」のポップオーバーに名前つきで畳む。
// - topBar: 共通のトップバーの中央（動画を見て回る一覧と管理表。画面が shell の
//   TopBarPortal で入れる）。折り返さず、検索欄が残りの幅を取り、表示の切り替えは
//   それぞれの inlineFrom の幅から行に並ぶ。それより狭い幅ではアイコンだけの
//   「View and sort」のポップオーバーに入る。
// 規則は web/registry/rules/patterns.md の Sections。

/** ToolbarBreakpoint は topBar の置き場で、表示の切り替えを行に並べ始める幅である。 */
export type ToolbarBreakpoint = "md" | "lg" | "xl";

/** ToolbarViewControl は表示の切り替えの 1 つで、畳んだときに見せる名前を持つ。 */
export interface ToolbarViewControl {
  /** 並びの中で一意な鍵。 */
  id: string;
  /** 見える名前（「Card size」）。ポップオーバーの中で操作の上に出す。 */
  label: string;
  /** 操作。行に並べるときは名前を出さないので、aria-label を付ける。id は付けない。 */
  control: ReactNode;
  /** topBar: ポップオーバーの中に置く形。省くと control をそのまま置く。 */
  compact?: ReactNode;
  /** topBar: compact が自分で見出しを持つ（並べ替えのラジオ）。label を重ねて出さない。 */
  compactLabelled?: boolean;
  /** topBar: 行に並べ始める幅。既定は lg。 */
  inlineFrom?: ToolbarBreakpoint;
  /** topBar: sm より狭い幅ではポップオーバーにも出さない（1 列で意味の無いカードの大きさ）。 */
  hideBelowSm?: boolean;
}

export interface ToolbarProps {
  /** 検索欄（Input）。sm より狭い幅では 1 行を占める。 */
  search?: ReactNode;
  /** 絞り込みの操作（outline の sm のボタン、適用中の Toggle）。 */
  children?: ReactNode;
  /** 表示の切り替え。lg から並べ、それより狭いと viewLabel のポップオーバーに名前つきで入る。 */
  view?: ToolbarViewControl[];
  /** 畳んだ表示の切り替えを開くボタンの文言（「View and sort」）。view と一緒に渡す。 */
  viewLabel?: string;
  /** 選択によらない操作。畳まない。 */
  actions?: ReactNode;
  /** 置き場。既定は page。topBar は TopBarPortal で共通のトップバーへ入れる。 */
  placement?: "page" | "topBar";
}

export function Toolbar(props: ToolbarProps) {
  return props.placement === "topBar" ? (
    <TopBarToolbar {...props} />
  ) : (
    <PageToolbar {...props} />
  );
}

function PageToolbar({ search, children, view, viewLabel, actions }: ToolbarProps) {
  const hasView = view !== undefined && view.length > 0;
  return (
    <div data-slot="toolbar" className="flex flex-wrap items-center gap-2">
      {search && (
        <div
          data-slot="toolbar-search"
          className="w-full sm:w-auto sm:max-w-md sm:min-w-search-min-sm sm:flex-1"
        >
          {search}
        </div>
      )}
      {children}
      {(hasView || actions) && (
        <div data-slot="toolbar-end" className="ml-auto flex items-center gap-2">
          {hasView && (
            <>
              <div data-slot="toolbar-view" className="hidden items-center gap-2 lg:flex">
                {view.map((item) => (
                  <Fragment key={item.id}>{item.control}</Fragment>
                ))}
              </div>
              <Popover>
                <PopoverTrigger asChild>
                  <Button variant="outline" size="sm" className="lg:hidden">
                    <SlidersHorizontal aria-hidden="true" />
                    {viewLabel}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="flex w-popover flex-col gap-4">
                  {view.map((item) => (
                    <div
                      key={item.id}
                      data-slot="toolbar-view-item"
                      className="flex flex-col items-start gap-1.5"
                    >
                      <span className="text-xs font-medium text-muted-foreground">
                        {item.label}
                      </span>
                      {item.control}
                    </div>
                  ))}
                </PopoverContent>
              </Popover>
            </>
          )}
          {actions}
        </div>
      )}
    </div>
  );
}

// クラスは静的な文字列でなければ生成されないので、幅ごとに書き出す。
const inlineClass: Record<ToolbarBreakpoint, string> = {
  md: "hidden md:block",
  lg: "hidden lg:block",
  xl: "hidden xl:block",
};
const compactClass: Record<ToolbarBreakpoint, string> = {
  md: "md:hidden",
  lg: "lg:hidden",
  xl: "xl:hidden",
};
const steps: ToolbarBreakpoint[] = ["md", "lg", "xl"];

function inlineStep(item: ToolbarViewControl): ToolbarBreakpoint {
  return item.inlineFrom ?? "lg";
}

/**
 * TopBarToolbar はトップバーの中央の 1 行である。検索欄が残りの幅を取り、操作は
 * 中央に寄せる。表示の切り替えはそれぞれの inlineFrom から行に並び、それより狭い幅では
 * アイコンだけの「View and sort」のポップオーバーに入る。ボタンは全部が行に並ぶ
 * 幅（いちばん広い inlineFrom）から隠す。
 */
function TopBarToolbar({ search, children, view, viewLabel, actions }: ToolbarProps) {
  const items = view ?? [];
  const widest = steps.reduce<ToolbarBreakpoint | undefined>(
    (found, step) => (items.some((item) => inlineStep(item) === step) ? step : found),
    undefined,
  );
  return (
    <div
      data-slot="toolbar"
      data-placement="top-bar"
      className="flex min-w-0 flex-1 items-center justify-center gap-1.5"
    >
      {search && (
        <div
          data-slot="toolbar-search"
          className="min-w-search-min flex-1 sm:max-w-md sm:min-w-search-min-sm"
        >
          {search}
        </div>
      )}
      {children}
      {items.map((item) => (
        <div
          key={item.id}
          data-slot="toolbar-view"
          className={inlineClass[inlineStep(item)]}
        >
          {item.control}
        </div>
      ))}
      {widest !== undefined && (
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={viewLabel}
              className={compactClass[widest]}
            >
              <SlidersHorizontal aria-hidden="true" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="flex w-popover-wide flex-col gap-4">
            {items.map((item) => (
              <div
                key={item.id}
                data-slot="toolbar-view-item"
                className={cn(
                  "flex flex-col items-stretch gap-1.5",
                  compactClass[inlineStep(item)],
                  item.hideBelowSm === true && "hidden sm:flex",
                )}
              >
                {item.compactLabelled !== true && (
                  <span className="text-xs font-medium text-muted-foreground">
                    {item.label}
                  </span>
                )}
                {item.compact ?? item.control}
              </div>
            ))}
          </PopoverContent>
        </Popover>
      )}
      {actions}
    </div>
  );
}
