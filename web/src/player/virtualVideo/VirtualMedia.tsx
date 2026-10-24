import { SquareDashed } from "lucide-react";
import { useState } from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";

/**
 * The player frame's content for a virtual video: its image, as a file video's frame shows its thumbnail, or a quiet
 * placeholder without one. Nothing plays, so there is no play button, control bar or cursor change; links and the
 * description read where a file video's do.
 */
export default function VirtualMedia({ video }: { video: Video }) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  const url = video.thumbnailUrl;
  if (url !== undefined && url !== brokenUrl)
    return (
      <img
        src={url}
        alt=""
        onError={() => setBrokenUrl(url)}
        className="absolute inset-0 size-full object-contain"
      />
    );
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
      <SquareDashed className="size-8 text-muted-foreground" aria-hidden="true" />
      <p className="text-sm text-muted-foreground">{t.player.virtual.noThumbnail}</p>
    </div>
  );
}
