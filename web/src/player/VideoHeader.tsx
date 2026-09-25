import { ChevronRight, X } from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router";

import type { VideoFolder } from "../api/client";
import { folderUrl } from "../folders/folderPath";
import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";

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

function Separator() {
  return <ChevronRight aria-hidden="true" className="size-3.5 shrink-0 text-fg-subtle" />;
}

/**
 * VideoHeader は再生画面の上端の帯である（ui-design「Video header」）。
 *
 * 左から ロゴ（ホームへ）・動画の置かれたフォルダまでのパンくず、右端に ×（遷移元の一覧へ
 * 戻る）を置く。画面を閉じる × はここ 1 か所だけにする。パンくずの段はすべてそのフォルダ
 * 画面へのリンクで、`md` より狭い幅では最後の段だけを残して途中を「…」に畳む
 * （出し分けは CSS の幅の分岐だけで行う、library-ui.md 4）。
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
    <header className="sticky top-0 z-40 flex h-navbar shrink-0 items-center gap-1 border-b border-border bg-bg/90 px-2 backdrop-blur-md sm:px-3">
      <Link
        to="/"
        aria-label="ホーム"
        className="flex h-8 shrink-0 items-center gap-2 rounded-sm px-2 text-base font-semibold tracking-tight text-fg select-none hover:bg-hover-wash"
      >
        <span className="size-2.5 rounded-full bg-accent" aria-hidden="true" />
        vv
      </Link>

      {crumbs.length > 0 && (
        <nav
          aria-label="フォルダ"
          className="flex min-w-0 flex-1 items-center overflow-hidden"
        >
          <ol className="flex min-w-0 items-center gap-0.5 text-sm whitespace-nowrap">
            {crumbs.map((crumb, index) => {
              const last = index === lastIndex;
              return (
                <Fragment key={crumb.to}>
                  {lastIndex > 0 && last && (
                    <li
                      aria-hidden="true"
                      className="flex items-center gap-0.5 md:hidden"
                    >
                      <Separator />
                      <span className="px-1 text-fg-subtle">…</span>
                    </li>
                  )}
                  <li
                    className={cn(
                      "flex min-w-0 items-center gap-0.5",
                      !last && "hidden shrink md:flex",
                    )}
                  >
                    <Separator />
                    <Link
                      to={crumb.to}
                      title={crumb.label}
                      className={cn(
                        "min-w-0 truncate rounded-sm px-1.5 py-0.5 transition-colors hover:bg-hover-wash hover:text-fg",
                        last ? "font-medium text-fg" : "max-w-40 text-fg-muted",
                      )}
                    >
                      {crumb.label}
                    </Link>
                  </li>
                </Fragment>
              );
            })}
          </ol>
        </nav>
      )}

      <IconButton label="閉じる" onClick={onClose} className="ml-auto [&>svg]:size-5!">
        <X aria-hidden="true" />
      </IconButton>
    </header>
  );
}
