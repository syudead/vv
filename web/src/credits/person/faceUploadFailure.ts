import { RequestFailed } from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { formatBytes } from "../../lib/format";
import { UPLOAD_MAX_BYTES } from "../../player/thumbnailUpload/uploadRules";

const text = t.credits.personPage.face;

/** Why a face could not be saved, for an uploaded image or a frame from a video. */
export function faceUploadFailure(failure: unknown): UiText {
  if (failure instanceof RequestFailed) {
    if (failure.reason === "person_image_invalid") return text.refusedType;
    if (failure.reason === "person_image_too_large") {
      return text.tooLarge(formatBytes(failure.limit ?? UPLOAD_MAX_BYTES));
    }
  }
  return text.failed(errorText(failure));
}
