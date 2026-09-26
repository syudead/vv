import { memo } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import Skeleton from "../ui/Skeleton";
import FolderArt from "../videoList/FolderArt";
import { folderUrl } from "./folderPath";

/**
 * folderLabel はフォルダカードの読み上げ名である（親 Issue のアクセシビリティ）。
 * 絶対パスが無い（ゲストの）応答では、パスを添えない。
 */
export function folderLabel(folder: FolderSummary, withPath: boolean): string {
  const label = `${folder.name}、動画 ${String(folder.videoCount)} 本、フォルダ ${String(folder.folderCount)} 件`;
  return withPath && folder.rootPath !== undefined
    ? `${label}、${folder.rootPath}`
    : label;
}

/**
 * FolderCard はフォルダ1件のカードである。箱は動画カードと同じで、上半分だけが
 * フォルダの絵柄になる。showPath は最上位（登録フォルダ）でパスを添えるとき。
 */
function FolderCard({ folder, showPath }: { folder: FolderSummary; showPath: boolean }) {
  return (
    <article
      data-folder-path={folder.path}
      className="group relative flex flex-col overflow-hidden rounded-lg bg-surface shadow-card transition-[box-shadow,transform] duration-200 ease-out-quart hover:-translate-y-0.5 hover:shadow-card-hover has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-link motion-reduce:transition-none motion-reduce:hover:translate-y-0"
    >
      <Link
        to={folderUrl({ rootId: folder.rootId, path: folder.path })}
        aria-label={folderLabel(folder, showPath)}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full">
          <FolderArt previews={folder.previews} />
        </div>
        <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
          <h3
            title={folder.name}
            className="line-clamp-2 text-sm leading-5 font-medium break-all text-fg"
          >
            {folder.name}
          </h3>
          <p className="text-xs text-fg-muted tabular-nums">
            動画 {folder.videoCount.toLocaleString("ja-JP")} 本
            <span className="text-fg-subtle"> · </span>
            フォルダ {folder.folderCount.toLocaleString("ja-JP")} 件
          </p>
          {showPath && folder.rootPath !== undefined && (
            // 同名の登録フォルダを見分けるのはパスの末尾なので、先頭の側を省略する。
            <p
              title={folder.rootPath}
              dir="rtl"
              className="truncate text-left text-xs text-fg-muted"
            >
              <bdi dir="ltr">{folder.rootPath}</bdi>
            </p>
          )}
        </div>
      </Link>
    </article>
  );
}

export default memo(FolderCard);

/** FolderCardSkeleton は読み込み中のフォルダカードである。 */
export function FolderCardSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div
          key={index}
          aria-hidden="true"
          className="flex flex-col overflow-hidden rounded-lg bg-surface"
        >
          <div className="relative aspect-video w-full">
            <div className="absolute inset-x-3 top-3 bottom-2">
              <Skeleton className="absolute top-0 left-0 h-3 w-2/5 rounded-b-none" />
              <Skeleton className="absolute inset-x-0 top-2 bottom-0 rounded-tl-none" />
            </div>
          </div>
          <div className="flex flex-col gap-1.5 px-3 pt-2 pb-3">
            <Skeleton className="h-4 w-3/5" />
            <Skeleton className="h-3 w-2/5" />
          </div>
        </div>
      ))}
    </>
  );
}
