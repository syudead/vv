import type { Video } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { Separator } from "../../ui/shadcn/separator";
import { FailureLine, PanelSection } from "../tabs/panelParts";
import { ThumbnailImage, WayButton, WayPreview, WayRow } from "../tabs/ThumbnailPanel";
import type { useThumbnailPosition } from "../tabs/useThumbnailPosition";
import type { VirtualThumbnail } from "./useVirtualThumbnail";

/**
 * The Thumbnail tab of a virtual video, which has no frames: the uploaded image with `Remove
 * image`, and an image file as the one way to replace it. The upload is the page's, shared with
 * the media frame's button, so the dialog and the preview belong to the tabs container.
 */
export default function VirtualThumbnailPanel({
  video,
  thumbnail,
  position,
  failure,
}: {
  video: Video;
  thumbnail: VirtualThumbnail;
  position: ReturnType<typeof useThumbnailPosition>;
  failure: UiText | null;
}) {
  const text = t.player.tabs;
  const words = t.player.virtual;
  const reverting = position.sending === "revert";
  const uploading = thumbnail.busy;
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-6 md:flex-row">
        <PanelSection title={text.currentThumbnail}>
          <div className="flex flex-col items-start gap-2">
            <div className="w-card-0 max-w-full">
              <ThumbnailImage url={video.thumbnailUrl} />
            </div>
            <p className="text-xs text-muted-foreground">
              {video.thumbnailImage === undefined
                ? words.noThumbnailYet
                : text.uploadedOrigin}
            </p>
            {video.thumbnailImage !== undefined && (
              <WayButton
                variant="ghost"
                className="-ml-2"
                label={words.removeImage}
                sending={reverting}
                waiting={uploading}
                onPress={() => {
                  if (!reverting && !uploading) position.send("revert");
                }}
              />
            )}
          </div>
        </PanelSection>
        <Separator className="md:hidden" />
        <Separator orientation="vertical" className="hidden md:block" />
        <div className="min-w-0 flex-1">
          <PanelSection title={text.replaceWith}>
            <ul className="flex flex-col">
              <WayRow
                preview={<WayPreview dashed />}
                title={text.anImage}
                note={text.uploadImageNote}
                label={text.uploadImage}
                sending={uploading}
                waiting={reverting}
                onPress={() => {
                  if (!reverting) thumbnail.pick();
                }}
              />
            </ul>
          </PanelSection>
        </div>
      </div>
      <FailureLine failure={failure} />
    </div>
  );
}
