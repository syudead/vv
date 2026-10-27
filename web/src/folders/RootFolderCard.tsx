import { Folder } from "lucide-react";
import { memo, useId } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { t, type UiText } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import FolderArt from "../videoList/FolderArt";
import { ItemRemoteMark } from "../videoList/RemoteItemMark";
import { folderUrl } from "./folderPath";

/** Guest responses omit absolute paths, so omit the path from their label. */
function folderLabel(folder: FolderSummary, withPath: boolean): UiText {
  const label = t.folders.card.label(folder.name, folder.videoCount, folder.folderCount);
  return withPath && folder.rootPath !== undefined
    ? t.folders.card.labelWithPath(label, folder.rootPath)
    : label;
}

/** The media folder list keeps its own bordered card; showPath distinguishes roots by their paths. */
function RootFolderCard({
  folder,
  showPath,
}: {
  folder: FolderSummary;
  showPath: boolean;
}) {
  const remote = isRemoteId(folder.rootId);
  const remoteId = useId();
  return (
    <article data-folder-path={folder.path} className="flex min-w-0 flex-col">
      <Link
        to={folderUrl({ rootId: folder.rootId, path: folder.path })}
        aria-label={folderLabel(folder, showPath)}
        aria-describedby={remote ? remoteId : undefined}
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
            {remote && (
              <span className="mt-1 flex text-muted-foreground">
                <ItemRemoteMark id={remoteId} />
              </span>
            )}
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

export default memo(RootFolderCard);
