import { t } from "../../i18n";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import type { ImageSearch } from "./useImageSearch";

/** The search field's pick-file dialog; the server reads any video, so the picker offers images and videos. */
export function ImageSearchDialog({ search }: { search: ImageSearch }) {
  const text = t.library.imageSearch.dialog;
  return (
    <ImageUploadDialog
      open={search.dialogOpen}
      onOpenChange={search.setDialogOpen}
      title={text.title}
      description={text.description}
      zone={text.zone}
      zoneHint={text.zoneHint}
      accept="image/png,image/jpeg,image/webp,video/*"
      onFile={search.takeDialogFile}
    />
  );
}
