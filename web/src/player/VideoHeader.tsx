import { FolderMinus, SquareDashed, X } from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router";

import type { VideoFolder } from "../api/client";
import AssistantTrigger from "../assistant/AssistantTrigger";
import { useAudience } from "../auth/audience";
import { folderUrl } from "../folders/folderPath";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { usePeerName } from "../shell/PeerProvider";
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
import { ItemRemoteMark } from "../videoList/RemoteItemMark";

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
  remote = false,
  folderOff = false,
  onClose,
}: {
  folder: VideoFolder | undefined;
  /** A virtual video has no folder; the band names what it is instead, not as a link. */
  virtual?: boolean;
  /** The paired VVMDM holds the video; the band names it before the breadcrumb. */
  remote?: boolean;
  /** Every folder holding the video is switched off; the crumbs turn to plain text, since the Folders page leaves it out. */
  folderOff?: boolean;
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

      {remote && <RemoteBand />}

      {folderOff && <FolderOffBand />}

      {crumbs.length > 0 && (
        <Breadcrumb
          aria-label={t.player.header.folder}
          className="flex min-w-0 flex-1 items-center overflow-hidden"
        >
          <BreadcrumbList className="flex-nowrap whitespace-nowrap">
            {crumbs.map((crumb, index) => {
              const last = index === lastIndex;
              const crumbClass = last
                ? "font-medium text-foreground"
                : "max-w-chip-label";
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
                    {folderOff ? (
                      <span title={crumb.label} className={cn("truncate", crumbClass)}>
                        {crumb.label}
                      </span>
                    ) : (
                      <BreadcrumbLink asChild title={crumb.label} className={crumbClass}>
                        <Link to={crumb.to}>{crumb.label}</Link>
                      </BreadcrumbLink>
                    )}
                  </BreadcrumbItem>
                </Fragment>
              );
            })}
          </BreadcrumbList>
        </Breadcrumb>
      )}

      <div className="ml-auto flex shrink-0 items-center">
        <AssistantTrigger />
      </div>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t.common.close}
            onClick={onClose}
            className="[&_svg]:size-5"
          >
            <X aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.common.close}</TooltipContent>
      </Tooltip>
    </header>
  );
}

/** The owner reaches the switch from the band; a guest reads it only. */
function FolderOffBand() {
  const owner = useAudience() === "owner";
  const text = t.player.folderOff;
  const content = (
    <>
      <FolderMinus className="size-4 shrink-0" aria-hidden="true" />
      <span className="truncate" aria-hidden="true">
        {text.band}
      </span>
      <span className="sr-only">{text.detail}</span>
    </>
  );
  const className =
    "flex min-w-0 items-center gap-1.5 px-1 text-sm text-muted-foreground";
  return owner ? (
    <Link
      to="/settings/library"
      title={text.detail}
      className={cn(className, "rounded-sm hover:text-foreground")}
    >
      {content}
    </Link>
  ) : (
    <p title={text.detail} className={className}>
      {content}
    </p>
  );
}

function RemoteBand() {
  const name = t.list.card.remote.name(usePeerName());
  return (
    <p className="flex min-w-0 items-center gap-1.5 px-1 text-sm text-muted-foreground">
      <ItemRemoteMark size="md" />
      <span
        aria-hidden="true"
        title={name}
        className="max-w-chip-label truncate md:max-w-none"
      >
        {name}
      </span>
    </p>
  );
}
