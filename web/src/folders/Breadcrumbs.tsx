import { ChevronRight } from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router";

import { cn } from "../lib/cn";
import Skeleton from "../ui/Skeleton";
import type { Crumb } from "./folderPath";

/** collapseFrom は狭い幅で途中の段を畳み始める段数である（ui-design.md）。 */
const collapseFrom = 4;

function Separator({ className }: { className?: string }) {
  return (
    <ChevronRight
      aria-hidden="true"
      className={cn("size-3.5 shrink-0 text-fg-subtle", className)}
    />
  );
}

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
 */
export default function Breadcrumbs({
  crumbs,
  suffix,
}: {
  crumbs: (Crumb | undefined)[];
  suffix?: string;
}) {
  const collapsible = crumbs.length >= collapseFrom;
  const lastIndex = crumbs.length - 1;

  return (
    <div className="sticky top-navbar z-20 -mx-3 -mt-3 flex h-10 items-center border-b border-border bg-bg/90 px-3 backdrop-blur-md sm:-mx-4 sm:px-4">
      <nav aria-label="パンくず" className="flex min-w-0 flex-1 items-center">
        <ol className="flex min-w-0 items-center gap-0.5 text-xs whitespace-nowrap">
          {crumbs.map((crumb, index) => {
            const hiddenWhenNarrow = collapsible && index < lastIndex - 1;
            const current = index === lastIndex;
            return (
              <Fragment key={index}>
                {collapsible && index === lastIndex - 1 && (
                  <li aria-hidden="true" className="flex items-center gap-0.5 md:hidden">
                    <span className="px-1 text-fg-muted">…</span>
                    <Separator />
                  </li>
                )}
                <li
                  className={cn(
                    "flex items-center gap-0.5",
                    current ? "min-w-0" : "min-w-0 shrink",
                    hiddenWhenNarrow && "hidden md:flex",
                  )}
                >
                  {crumb === undefined ? (
                    <Skeleton className="h-3.5 w-20" />
                  ) : crumb.to === undefined ? (
                    <span
                      aria-current="page"
                      title={crumb.label}
                      className="min-w-0 truncate px-1 font-medium text-fg"
                    >
                      {crumb.label}
                    </span>
                  ) : (
                    <Link
                      to={crumb.to}
                      title={crumb.label}
                      className="max-w-40 min-w-0 truncate rounded-sm px-1 py-0.5 text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg"
                    >
                      {crumb.label}
                    </Link>
                  )}
                  {!current && <Separator />}
                </li>
              </Fragment>
            );
          })}
        </ol>
        {suffix !== undefined && (
          <span className="shrink-0 pl-1 text-fg-muted">{suffix}</span>
        )}
      </nav>
    </div>
  );
}
