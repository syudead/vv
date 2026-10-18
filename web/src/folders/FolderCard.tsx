import { Folder } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { t, type UiText } from "../i18n";
import FolderArt from "../videoList/FolderArt";
import { folderUrl } from "./folderPath";

/** Guest responses omit absolute paths, so omit the path from their label. */
export function folderLabel(folder: FolderSummary, withPath: boolean): UiText {
  const label = t.folders.card.label(folder.name, folder.videoCount, folder.folderCount);
  return withPath && folder.rootPath !== undefined
    ? t.folders.card.labelWithPath(label, folder.rootPath)
    : label;
}

/** showPath distinguishes registered roots by their paths. */
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
            // Truncate the beginning: path suffixes distinguish roots with the same name.
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
