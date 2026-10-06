import { Folder } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { t, type UiText } from "../i18n";
import FolderArt from "../videoList/FolderArt";
import { folderUrl } from "./folderPath";

/**
 * folderLabel はフォルダカードの読み上げ名である（親 Issue のアクセシビリティ）。
 * 絶対パスが無い（ゲストの）応答では、パスを添えない。
 */
export function folderLabel(folder: FolderSummary, withPath: boolean): UiText {
  const label = t.folders.card.label(folder.name, folder.videoCount, folder.folderCount);
  return withPath && folder.rootPath !== undefined
    ? t.folders.card.labelWithPath(label, folder.rootPath)
    : label;
}

/**
 * FolderCard はフォルダ1件のカードである。動画カードと同じ格子の枡と境界を使い、
 * フォルダの絵柄と名前の目印で動画から区別する。カード全体が1つのリンクで、
 * キーボードのフォーカスは共通の輪郭でカードの縁に出る。
 * showPath は最上位（登録フォルダ）でパスを添えるとき。
 */
function FolderCard({ folder, showPath }: { folder: FolderSummary; showPath: boolean }) {
  return (
    <article data-folder-path={folder.path} className="flex min-w-0 flex-col">
      <Link
        to={folderUrl({ rootId: folder.rootId, path: folder.path })}
        aria-label={folderLabel(folder, showPath)}
        className="flex min-w-0 flex-1 flex-col overflow-hidden rounded-md border border-border bg-card text-card-foreground transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
      >
        <div className="relative aspect-video w-full">
          <FolderArt previews={folder.previews} />
        </div>
        <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
          <h3
            title={folder.name}
            className="flex min-w-0 items-start gap-1.5 text-sm font-medium"
          >
            <Folder
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
            />
            <span className="line-clamp-2 min-w-0 break-all">{folder.name}</span>
          </h3>
          <p className="text-xs text-muted-foreground tabular-nums">
            {t.folders.card.videos(folder.videoCount)}
            {" · "}
            {t.folders.card.folders(folder.folderCount)}
          </p>
          {showPath && folder.rootPath !== undefined && (
            // 同名の登録フォルダを見分けるのはパスの末尾なので、先頭の側を省略する。
            <p
              title={folder.rootPath}
              dir="rtl"
              className="truncate text-left text-xs text-muted-foreground"
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
