import { FolderSearch } from "lucide-react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { smartFolderPagePath } from "../smartFolders/smartFolderPaths";

type VideoSmartFolder = NonNullable<Video["smartFolder"]>;

/** Smart folders have no manual removal; editing belongs on the folder page. */
export default function SmartFolderLine({
  folder,
  backTo,
}: {
  folder: VideoSmartFolder;
  /** Preserve the video's return destination as state.from. */
  backTo: string;
}) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Link
        to={smartFolderPagePath(folder.id)}
        state={{ from: backTo }}
        aria-label={t.player.smartFolder.line(folder.name, folder.position, folder.count)}
        data-smart-folder-line=""
        className="flex max-w-full min-w-0 items-center gap-1.5 rounded-sm text-xs text-muted-foreground hover:text-foreground sm:text-sm"
      >
        <FolderSearch className="size-4 shrink-0" aria-hidden="true" />
        <span className="truncate" title={folder.name}>
          {folder.name}
        </span>
        <span className="shrink-0" aria-hidden="true">
          ·
        </span>
        <span className="shrink-0 tabular-nums">
          {t.player.related.position(folder.position, folder.count)}
        </span>
      </Link>
    </div>
  );
}
