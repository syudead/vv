import { SquareDashed, X } from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router";

import type { VideoFolder } from "../api/client";
import { folderUrl } from "../folders/folderPath";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import BrandHomeLink from "../ui/BrandHomeLink";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbSeparator,
} from "../ui/shadcn/breadcrumb";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

interface FolderCrumb {
  label: string;
  to: string;
}

/** Omit breadcrumbs when the registered folder's display name is unknown; do not show a partial path. */
export function folderCrumbs(folder: VideoFolder | undefined): FolderCrumb[] {
  if (folder?.rootName === undefined) return [];
  const segments = folder.path === "" ? [] : folder.path.split("/");
  return [
    { label: folder.rootName, to: folderUrl({ rootId: folder.rootId, path: "" }) },
    ...segments.map((segment, index) => ({
      label: segment,
      to: folderUrl({
        rootId: folder.rootId,
        path: segments.slice(0, index + 1).join("/"),
      }),
    })),
  ];
}

/** DetailPage owns header layout; CSS collapses intermediate breadcrumbs below md. */
export default function VideoHeader({
  folder,
  virtual = false,
  onClose,
}: {
  folder: VideoFolder | undefined;
  /** A virtual video has no folder; the band names what it is instead, not as a link. */
  virtual?: boolean;
  onClose: () => void;
}) {
  const crumbs = folderCrumbs(folder);
  const lastIndex = crumbs.length - 1;

  return (
    <header className="flex min-w-0 flex-1 items-center gap-1">
      <BrandHomeLink />

      {virtual && (
        <p className="flex min-w-0 items-center gap-1.5 px-1 text-sm text-muted-foreground">
          <SquareDashed className="size-4 shrink-0" aria-hidden="true" />
          <span className="truncate">{t.player.virtual.band}</span>
        </p>
      )}

      {crumbs.length > 0 && (
        <Breadcrumb
          aria-label={t.player.header.folder}
          className="flex min-w-0 flex-1 items-center overflow-hidden"
        >
          <BreadcrumbList className="flex-nowrap whitespace-nowrap">
            {crumbs.map((crumb, index) => {
              const last = index === lastIndex;
              return (
                <Fragment key={crumb.to}>
                  {lastIndex > 0 && last && (
                    <>
                      <BreadcrumbSeparator className="md:hidden" />
                      <BreadcrumbItem className="md:hidden">
                        <BreadcrumbEllipsis />
                      </BreadcrumbItem>
                    </>
                  )}
                  <BreadcrumbSeparator className={cn(!last && "hidden md:block")} />
                  <BreadcrumbItem className={cn(!last && "hidden md:inline-flex")}>
                    <BreadcrumbLink
                      asChild
                      title={crumb.label}
                      className={cn(
                        last ? "font-medium text-foreground" : "max-w-chip-label",
                      )}
                    >
                      <Link to={crumb.to}>{crumb.label}</Link>
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                </Fragment>
              );
            })}
          </BreadcrumbList>
        </Breadcrumb>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t.common.close}
            onClick={onClose}
            className="ml-auto [&_svg]:size-5"
          >
            <X aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.common.close}</TooltipContent>
      </Tooltip>
    </header>
  );
}
