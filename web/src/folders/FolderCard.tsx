import { memo, type PointerEvent, useState } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { cn } from "../lib/cn";
import Skeleton from "../ui/Skeleton";
import { folderUrl } from "./folderPath";

type Preview = FolderSummary["previews"][number];

/**
 * mosaicCells は件数ごとのモザイクの区切り方である（ui-design.md「Folder card」）。
 * 1件は全面、2件は左右、3件は左の大きな1枚と右の上下、4件は 2×2 に並べる。
 */
const mosaicCells: Record<number, string[]> = {
  1: ["col-span-2 row-span-2"],
  2: ["row-span-2", "row-span-2"],
  3: ["row-span-2", "", ""],
  4: ["", "", "", ""],
};

/**
 * FolderArt はタブ付きのフォルダの外形と、その中に敷き詰めたサムネイルを描く。
 * 2件以上あるときは、マウスを横に動かすと位置に応じた1件を枠いっぱいに映す
 * （フォルダの中身の下見）。装飾なので読み上げない。
 */
function FolderArt({ previews }: { previews: Preview[] }) {
  const shown = previews.slice(0, 4);
  const cells = mosaicCells[shown.length] ?? [];
  const [scrub, setScrub] = useState<number | null>(null);
  const scrubbable = shown.length > 1;
  // 取り込み後の再取得で件数が1件以下に減っても、残った位置で下見を出し続けない。
  const focused = !scrubbable || scrub === null ? undefined : shown[scrub];

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (!scrubbable || event.pointerType !== "mouse") return;
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = (event.clientX - rect.left) / rect.width;
    const index = Math.min(
      shown.length - 1,
      Math.max(0, Math.floor(ratio * shown.length)),
    );
    setScrub(index);
  };

  return (
    <div
      aria-hidden="true"
      data-folder-art=""
      onPointerMove={onPointerMove}
      onPointerLeave={() => setScrub(null)}
      className="absolute inset-x-2.5 top-2 bottom-2"
    >
      <div className="absolute top-0 left-0 h-2.5 w-2/5 rounded-t-md bg-elevated" />
      <div className="absolute inset-x-0 top-2 bottom-0 rounded-md rounded-tl-none bg-elevated p-1">
        <div className="relative grid h-full w-full grid-cols-2 grid-rows-2 gap-0.5 overflow-hidden rounded-sm bg-surface-hover">
          {shown.map((preview, index) => (
            <div
              key={preview.videoId}
              data-folder-preview=""
              className={cn("min-h-0 min-w-0 overflow-hidden bg-navbar", cells[index])}
            >
              <img
                src={preview.thumbnailUrl}
                alt=""
                loading="lazy"
                decoding="async"
                className="h-full w-full object-cover object-top"
              />
            </div>
          ))}
          {focused !== undefined && scrub !== null && (
            <div data-folder-scrub={scrub} className="absolute inset-0 bg-navbar">
              <img
                src={focused.thumbnailUrl}
                alt=""
                decoding="async"
                className="h-full w-full object-cover object-top"
              />
              <div className="absolute inset-x-1.5 bottom-1.5 flex gap-1">
                {shown.map((preview, index) => (
                  <span
                    key={preview.videoId}
                    className={cn(
                      "h-[3px] flex-1 rounded-full",
                      index === scrub ? "bg-fg" : "bg-fg/35",
                    )}
                  />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/** folderLabel はフォルダカードの読み上げ名である（親 Issue のアクセシビリティ）。 */
export function folderLabel(folder: FolderSummary, withPath: boolean): string {
  const label = `${folder.name}、動画 ${String(folder.videoCount)} 本、フォルダ ${String(folder.folderCount)} 件`;
  return withPath ? `${label}、${folder.rootPath}` : label;
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
          {showPath && (
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
            <div className="absolute inset-x-2.5 top-2 bottom-2">
              <Skeleton className="absolute top-0 left-0 h-2.5 w-2/5 rounded-b-none" />
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
