import { FolderSearch } from "lucide-react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { smartFolderPagePath } from "../smartFolders/smartFolderPaths";

type VideoSmartFolder = NonNullable<Video["smartFolder"]>;

/**
 * SmartFolderLine は題名の上に置く、スマートフォルダの名前と何本目かの 1 行である。グループの行（GroupLine）と同じ形で、
 * 印が `FolderSearch` になる。行はフォルダのページへのリンクで、ゲストにも同じリンクを出す。
 * フォルダには手で外す操作が無く、編集はフォルダのページにあるので、行の後に操作は置かない。
 */
export default function SmartFolderLine({
  folder,
  backTo,
}: {
  folder: VideoSmartFolder;
  /** 動画ページの戻り先。フォルダのページへ `state.from` として引き継ぐ。 */
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
