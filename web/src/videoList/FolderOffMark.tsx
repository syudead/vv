import { FolderMinus } from "lucide-react";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { isRemoteId } from "../lib/remoteId";
import { usePeerName } from "../shell/PeerProvider";

/** Line under a playlist row's title for a video whose media folder is switched off; a remote video names its VVMDM. */
export function FolderOffMark({
  video,
  className,
}: {
  video: Video | undefined;
  className?: string;
}) {
  const peerName = usePeerName();
  if (video?.folderOff !== true) return null;
  const name = isRemoteId(video.id) ? t.list.card.remote.name(peerName) : undefined;
  return (
    <span
      className={cn("flex items-center gap-1 text-xs text-muted-foreground", className)}
    >
      <FolderMinus aria-hidden="true" className="size-3 shrink-0" />
      {t.list.card.folderOff(name)}
    </span>
  );
}
