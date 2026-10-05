import { X } from "lucide-react";
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

/**
 * folderCrumbs は動画の置かれたフォルダまでの段を、登録フォルダから順に作る。
 * 登録フォルダの表示名が分からなければ段を作らない（途中からの道筋は出さない）。
 */
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

/**
 * VideoHeader は再生画面の上端の帯の中身で、詳細ページの型（DetailPage）の header に置く
 * （ui-design「Video header」、web/registry/rules/patterns.md の Detail page）。帯の高さ・下の
 * 線・上に留まる動きは型が持つ。
 *
 * 左から ロゴ（ホームへ）・動画の置かれたフォルダまでのパンくず（Breadcrumb）、右端に ×
 * （遷移元の一覧へ戻る）を置く。画面を閉じる × はここ 1 か所だけにする。パンくずの段は
 * すべてそのフォルダ画面へのリンクで、`md` より狭い幅では最後の段だけを残して途中を
 * 「…」に畳む（出し分けは CSS の幅の分岐だけで行う、library-ui.md 4）。
 */
export default function VideoHeader({
  folder,
  onClose,
}: {
  folder: VideoFolder | undefined;
  onClose: () => void;
}) {
  const crumbs = folderCrumbs(folder);
  const lastIndex = crumbs.length - 1;

  return (
    <header className="flex min-w-0 flex-1 items-center gap-1">
      <BrandHomeLink />

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
