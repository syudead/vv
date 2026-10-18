import { Layers } from "lucide-react";

import type { Video } from "../api/client";
import { t } from "../i18n";

//

/** Show a count only for at least two versions visible to this audience. */
export function shownVersionsCount(video: Video): number | null {
  const count = video.versions?.count ?? 0;
  return count >= 2 ? count : null;
}

/** Noninteractive mark; the card link references its accessible text through aria-describedby. */
export function VersionsMark({ count, id }: { count: number; id?: string }) {
  return (
    <>
      <Layers aria-hidden="true" className="size-3 shrink-0" />
      <span id={id} className="sr-only">
        {t.player.versions.count(count)}
      </span>
    </>
  );
}
