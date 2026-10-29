import { useEffect, useState } from "react";

import {
  getMediaFolderUsage,
  type MediaFolder,
  type MediaFolderUsage as Usage,
} from "../api/client";
import { t } from "../i18n";
import { formatBytes } from "../lib/format";
import { Skeleton } from "../ui/shadcn/skeleton";

type State = { key: string; usage: Usage | null } | null;

/** Reads usage per row so a slow or unreachable volume never holds up the folder list. */
export default function MediaFolderUsage({ folder }: { folder: MediaFolder }) {
  const key = `${String(folder.id)}:${String(folder.version)}`;
  const [state, setState] = useState<State>(null);

  useEffect(() => {
    const controller = new AbortController();
    getMediaFolderUsage(folder.id, controller.signal).then(
      (usage) => setState({ key, usage }),
      () => {
        if (!controller.signal.aborted) setState({ key, usage: null });
      },
    );
    return () => controller.abort();
  }, [folder.id, key]);

  const text = t.settings.mediaFolders.usage;
  // A changed path keeps the old row mounted, so ignore usage read for the previous version.
  if (state?.key !== key) {
    return (
      <div role="status" aria-label={text.loading}>
        <Skeleton className="mt-1 h-4 w-1/2" />
      </div>
    );
  }
  const { usage } = state;
  return (
    <p className="mt-1 text-xs text-muted-foreground tabular-nums">
      {usage?.disk === undefined ? (
        text.unavailable
      ) : (
        <>
          <span className="font-medium text-foreground">
            {text.free(formatBytes(usage.disk.freeBytes))}
          </span>{" "}
          {text.ofTotal(formatBytes(usage.disk.totalBytes))}
        </>
      )}
      {usage !== null && (
        <>
          {" · "}
          {text.videos(formatBytes(usage.videoBytes))}
        </>
      )}
    </p>
  );
}
