import { Fragment, type Ref } from "react";
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

const separatorClass = "flex text-muted-foreground [&>svg]:size-crumb-separator";

/**
 * Breadcrumbs はトップバーの直下に固定するパンくず帯である。
 *
 * 段が多いとき、`md` より狭い幅では先頭の側を省略して「… › 親 › 現在地」だけを
 * 見せる（最上位へはサイドバーの「フォルダ」から戻れる）。
 * 出し分けは CSS の幅の分岐だけで行い、幅を監視しない（library-ui.md 4）。
 * undefined の段は名前がまだ分からない段で、骨組みで描く。
 *
 * suffix は検索中に現在地の直後へ続ける文字（「内を検索中」など、ui-design.md
 * 「Folder screen」）。段と違って省略しない — 検索中であることを見失わせないためである。
 *
 * 帯は一覧ページ（ListPage）の上の余白と左右の余白を打ち消して、ページの幅いっぱいに
 * 置く。帯の下は本体との間を 12px にする。
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
    <div className="sticky top-navbar z-20 -mx-3 -mt-4 -mb-1 flex h-10 items-center border-b border-border bg-background/90 px-3 backdrop-blur-md sm:-mx-4 sm:px-4">
      <Breadcrumb
        aria-label={t.folders.breadcrumbs}
        className="flex min-w-0 flex-1 items-center"
      >
        {/* 上流の一覧は折り返す。フォルダの道筋は一行に保ち、溢れた段を省略記号で切る。 */}
        <BreadcrumbList className="min-w-0 flex-nowrap gap-0.5 text-xs whitespace-nowrap wrap-normal">
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
                    <BreadcrumbSeparator className={cn(separatorClass, "md:hidden")} />
                  </>
                )}
                <BreadcrumbItem
                  className={cn(
                    current ? "min-w-0" : "min-w-0 shrink",
                    hiddenWhenNarrow && "hidden md:inline-flex",
                  )}
                >
                  {crumb === undefined ? (
                    <Skeleton className="h-3 w-16" />
                  ) : crumb.to === undefined ? (
                    // 現在地はリンクにしない（押せないリンクとして読ませない）ので、
                    // 上流の role="link" を外す。
                    <BreadcrumbPage
                      role={undefined}
                      aria-disabled={undefined}
                      title={crumb.label}
                      className="min-w-0 px-1"
                    >
                      {crumb.label}
                    </BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink asChild>
                      <Link
                        to={crumb.to}
                        title={crumb.label}
                        className="max-w-crumb min-w-0 px-1 py-0.5 hover:bg-accent"
                      >
                        {crumb.label}
                      </Link>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
                {!current && (
                  <BreadcrumbSeparator
                    className={cn(separatorClass, hiddenWhenNarrow && "hidden md:flex")}
                  />
                )}
              </Fragment>
            );
          })}
        </BreadcrumbList>
        {suffix !== undefined && (
          <span className="shrink-0 pl-1 text-xs text-muted-foreground">{suffix}</span>
        )}
      </Breadcrumb>
    </div>
  );
}

/**
 * FolderHeader はフォルダ画面の一覧ページの見出しである。題は見た目に出さない h1
 * （現在地はパンくずが見せる）で、その下にパンくずの帯を置く。帯が一覧ページの縦の並びに
 * 直に入って固定されるよう、包む要素を作らない。headingRef は着いたときにフォーカスを
 * 置く先（useArrival）。
 */
export function FolderHeader({
  title,
  headingRef,
  crumbs,
  suffix,
}: {
  title: string;
  headingRef?: Ref<HTMLHeadingElement>;
  crumbs: (Crumb | undefined)[];
  suffix?: UiText;
}) {
  return (
    <>
      <h1 ref={headingRef} tabIndex={-1} className="sr-only">
        {title}
      </h1>
      <Breadcrumbs crumbs={crumbs} suffix={suffix} />
    </>
  );
}
