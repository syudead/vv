import { SlidersHorizontal } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { Button } from "@/ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/ui/shadcn/popover";

// ツールバー（区画）。検索、絞り込みの操作、末尾に表示の切り替えと操作を 1 行に並べ、
// 狭い幅では折り返す。lg より狭い幅では表示の切り替え（表示形式・大きさ・並べ替え）を
// 「View and sort」のポップオーバーに畳む。ポップオーバーの中では各操作の上に見える名前を
// 出す。規則は web/registry/rules/patterns.md の Sections。

/** ToolbarViewControl は表示の切り替えの 1 つで、畳んだときに見せる名前を持つ。 */
export interface ToolbarViewControl {
  /** 並びの中で一意な鍵。 */
  id: string;
  /** 見える名前（「Card size」）。ポップオーバーの中で操作の上に出す。 */
  label: string;
  /** 操作。lg 以上では名前を出さないので、aria-label を付ける。id は付けない。 */
  control: ReactNode;
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
}

export function Toolbar({ search, children, view, viewLabel, actions }: ToolbarProps) {
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
