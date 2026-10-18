import { Folder } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { t, type UiText } from "../i18n";
import { CollectionName, CollectionThumbnail } from "../library/GroupCard";
import { useCollectionLink } from "../videoList/useCollectionLink";
import { folderUrl } from "./folderPath";

function folderLabel(folder: FolderSummary): UiText {
  return t.folders.card.label(folder.name, folder.videoCount, folder.folderCount);
}

/** Uses the group card's thumbnail and name so every collection grid looks the same. */
function FolderCard({ folder, backTo }: { folder: FolderSummary; backTo: string }) {
  const { linkProps, onFrontChange } = useCollectionLink({
    to: folderUrl({ rootId: folder.rootId, path: folder.path }),
    backTo,
    name: folder.name,
    label: folderLabel(folder),
    videoPath: (id) => `/videos/${String(id)}`,
  });
  return (
    <article
      data-folder-path={folder.path}
      className="group relative flex min-w-0 flex-col gap-2"
    >
      <Link {...linkProps} className="flex min-w-0 flex-1 flex-col gap-2 rounded-md">
        <CollectionThumbnail
          previews={folder.previews}
          totalCount={folder.videoCount}
          onFrontChange={onFrontChange}
          ratio={null}
          face={
            <>
              <Folder aria-hidden="true" className="size-3 shrink-0" />
              <span>{t.folders.card.videos(folder.videoCount)}</span>
              <span>{t.folders.card.folders(folder.folderCount)}</span>
            </>
          }
        />
        <CollectionName name={folder.name} muted={false} />
      </Link>
    </article>
  );
}

export default memo(FolderCard);
