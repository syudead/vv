import { Fragment, type ReactNode } from "react";
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

const collapseFrom = 4;

/**
 * Place first in ListPage's header for a full-width sticky breadcrumb. Unknown crumbs render
 * skeletons; suffix stays visible during collapse, keepFirst preserves the root, and target wraps
 * drop targets.
 */
export default function Breadcrumbs({
  crumbs,
  suffix,
  label = t.folders.breadcrumbs,
  keepFirst = false,
  target = noTarget,
}: {
  crumbs: (Crumb | undefined)[];
  suffix?: UiText;
  label?: UiText;
  keepFirst?: boolean;
  target?: (index: number | "ellipsis", content: ReactNode) => ReactNode;
}) {
  const collapsible = crumbs.length >= collapseFrom;
  const lastIndex = crumbs.length - 1;

  return (
    <div
      data-slot="folder-breadcrumbs"
      className="sticky top-navbar z-35 -mx-3 -mt-3 flex h-10 items-center border-b border-border bg-background px-3 sm:-mx-4 sm:-mt-4 sm:px-4"
    >
      <Breadcrumb aria-label={label} className="min-w-0 flex-1">
        {/* Override upstream wrapping to keep the path on one ellipsized line. */}
        <BreadcrumbList className="min-w-0 flex-nowrap whitespace-nowrap wrap-normal">
          {crumbs.map((crumb, index) => {
            const hiddenWhenNarrow =
              collapsible && index < lastIndex - 1 && !(keepFirst && index === 0);
            const current = index === lastIndex;
            return (
              <Fragment key={index}>
                {collapsible && index === lastIndex - 1 && (
                  <>
                    <BreadcrumbItem aria-hidden="true" className="md:hidden">
                      {target("ellipsis", <BreadcrumbEllipsis />)}
                    </BreadcrumbItem>
                    <BreadcrumbSeparator className="md:hidden" />
                  </>
                )}
                <BreadcrumbItem
                  className={cn("min-w-0", hiddenWhenNarrow && "hidden md:inline-flex")}
                >
                  {crumb === undefined ? (
                    <Skeleton className="h-4 w-16" />
                  ) : (
                    target(index, <CrumbContent crumb={crumb} />)
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

function noTarget(...[, content]: [number | "ellipsis", ReactNode]): ReactNode {
  return content;
}

function CrumbContent({ crumb }: { crumb: Crumb }) {
  return crumb.to === undefined ? (
    // Remove the upstream link role: the current location is not interactive.
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
  );
}
