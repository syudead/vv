import { LoaderCircle, SquareDashed } from "lucide-react";
import { useState } from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import type { VirtualThumbnail } from "./useVirtualThumbnail";
import VirtualLinks from "./VirtualLinks";

interface MediaProps {
  video: Video;
  owner: boolean;
  thumbnail: VirtualThumbnail;
}

/**
 * The player frame's content for a virtual video: its links over its uploaded image, or a
 * placeholder when it has neither. Nothing here plays, so there is no play button, control bar or
 * cursor change.
 */
export default function VirtualMedia(props: MediaProps) {
  const urls = props.video.workInfo?.urls ?? [];
  return (
    <>
      <FrameImage {...props} withLinks={urls.length > 0} />
      {urls.length > 0 && <VirtualLinks urls={urls} />}
    </>
  );
}

/** The links take the placeholder's place, so the owner then uploads from the Thumbnail tab. */
function FrameImage({
  video,
  owner,
  thumbnail,
  withLinks,
}: MediaProps & { withLinks: boolean }) {
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
  if (withLinks) return null;
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
