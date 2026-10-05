import { Fragment } from "react";
import { Link } from "react-router";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "../ui/shadcn/breadcrumb";
import { Skeleton } from "../ui/shadcn/skeleton";
import type { Crumb } from "./folderPath";

/** collapseFrom は狭い幅で途中の段を畳み始める段数である（ui-design.md）。 */
const collapseFrom = 4;

/**
 * Breadcrumbs はフォルダ画面の見出しの上に置くパンくずである。トップバーの直下に貼り付く
 * 帯で、下へ送っても上の段へ戻れる。ListPage の header の先頭（ページの直下の子）に置き、
 * ページの余白を打ち消して全幅にする。
 *
 * 段が多いとき、`md` より狭い幅では先頭の側を省略して「… › 親 › 現在地」だけを
 * 見せる（最上位へはサイドバーの「フォルダ」から戻れる）。
 * 出し分けは CSS の幅の分岐だけで行い、幅を監視しない（library-ui.md 4）。
 * undefined の段は名前がまだ分からない段で、骨組みで描く。
 *
 * suffix は検索中に現在地の直後へ続ける文字（「内を検索中」など、ui-design.md
 * 「Folder screen」）。段と違って省略しない — 検索中であることを見失わせないためである。
 */
export default function Breadcrumbs({
  crumbs,
  suffix,
}: {
  crumbs: (Crumb | undefined)[];
  suffix?: UiText;
}) {
  const collapsible = crumbs.length >= collapseFrom;
  const lastIndex = crumbs.length - 1;

  return (
    <div
      data-slot="folder-breadcrumbs"
      className="sticky top-navbar z-20 -mx-3 -mt-3 flex h-10 items-center border-b border-border bg-background px-3 sm:-mx-4 sm:-mt-4 sm:px-4"
    >
      <Breadcrumb aria-label={t.folders.breadcrumbs} className="min-w-0 flex-1">
        {/* 上流の一覧は折り返す。フォルダの道筋は一行に保ち、溢れた段を省略記号で切る。 */}
        <BreadcrumbList className="min-w-0 flex-nowrap whitespace-nowrap wrap-normal">
          {crumbs.map((crumb, index) => {
            const hiddenWhenNarrow = collapsible && index < lastIndex - 1;
            const current = index === lastIndex;
            return (
              <Fragment key={index}>
                {collapsible && index === lastIndex - 1 && (
                  <>
                    <BreadcrumbItem aria-hidden="true" className="md:hidden">
                      <BreadcrumbEllipsis />
                    </BreadcrumbItem>
                    <BreadcrumbSeparator className="md:hidden" />
                  </>
                )}
                <BreadcrumbItem
                  className={cn("min-w-0", hiddenWhenNarrow && "hidden md:inline-flex")}
                >
                  {crumb === undefined ? (
                    <Skeleton className="h-4 w-16" />
                  ) : crumb.to === undefined ? (
                    // 現在地はリンクにしない（押せないリンクとして読ませない）ので、
                    // 上流の role="link" を外す。
                    <BreadcrumbPage
                      role={undefined}
                      aria-disabled={undefined}
                      title={crumb.label}
                      className="truncate"
                    >
                      {crumb.label}
                    </BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink asChild>
                      <Link to={crumb.to} title={crumb.label} className="truncate">
                        {crumb.label}
                      </Link>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
                {!current && (
                  <BreadcrumbSeparator
                    className={cn(hiddenWhenNarrow && "hidden md:block")}
                  />
                )}
              </Fragment>
            );
          })}
          {suffix !== undefined && <BreadcrumbItem>{suffix}</BreadcrumbItem>}
        </BreadcrumbList>
      </Breadcrumb>
    </div>
  );
}
