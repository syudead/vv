import { LoaderCircle, SquareDashed } from "lucide-react";
import { useState } from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import type { VirtualThumbnail } from "./useVirtualThumbnail";

/**
 * The player frame's content for a virtual video: its uploaded image, or a placeholder. Nothing here
 * plays, so there is no play button, control bar or cursor change; only the owner's upload button
 * can be pressed.
 */
export default function VirtualMedia({
  video,
  owner,
  thumbnail,
}: {
  video: Video;
  owner: boolean;
  thumbnail: VirtualThumbnail;
}) {
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
  const words = t.player.virtual;
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
      <SquareDashed className="size-8 text-muted-foreground" aria-hidden="true" />
      <p className="text-sm text-muted-foreground">
        {owner ? words.noThumbnailYet : words.noThumbnail}
      </p>
      {owner && (
        <Button
          variant="secondary"
          aria-disabled={thumbnail.busy || undefined}
          aria-busy={thumbnail.upload.sending || undefined}
          onClick={thumbnail.pick}
        >
          {thumbnail.busy && (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          )}
          {words.uploadImage}
        </Button>
      )}
    </div>
  );
}
