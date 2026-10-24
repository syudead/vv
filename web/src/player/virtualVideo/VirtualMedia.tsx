import { ExternalLink, SquareDashed } from "lucide-react";
import { useState } from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { hostOf } from "../workInfoFetch/fetchRules";

/**
 * The player frame's content for a virtual video: its image, as a file video's frame shows its thumbnail, or a quiet
 * placeholder without one. Where a file video's frame starts playback, this one opens the video's first link in a
 * new tab; without a link it does nothing.
 */
export default function VirtualMedia({ video }: { video: Video }) {
  const link = video.workInfo?.urls[0];
  if (link === undefined) return <FrameImage video={video} />;
  return (
    <a
      href={link}
      target="_blank"
      rel="noreferrer"
      title={link}
      aria-label={t.player.virtual.openLink(hostOf(link))}
      className="group absolute inset-0 cursor-pointer"
    >
      <FrameImage video={video} />
      <span className="absolute inset-0 flex items-center justify-center">
        <span className="flex size-16 items-center justify-center rounded-full bg-overlay text-foreground opacity-80 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 motion-reduce:transition-none">
          <ExternalLink className="size-6" aria-hidden="true" />
        </span>
      </span>
    </a>
  );
}

function FrameImage({ video }: { video: Video }) {
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
