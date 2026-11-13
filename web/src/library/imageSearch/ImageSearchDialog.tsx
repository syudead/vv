import { t } from "../../i18n";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import type { FileGiving } from "./imageSearchEntry";

/** The search field's pick-file dialog; the server reads any video, so the picker offers images and videos. */
export function ImageSearchDialog({ giving }: { giving: FileGiving }) {
  const text = t.library.imageSearch.dialog;
  return (
    <ImageUploadDialog
      open={giving.dialogOpen}
      onOpenChange={giving.setDialogOpen}
      title={text.title}
      description={text.description}
      zone={text.zone}
      zoneHint={text.zoneHint}
      accept="image/png,image/jpeg,image/webp,video/*"
      onFile={giving.takeDialogFile}
    />
  );
}
