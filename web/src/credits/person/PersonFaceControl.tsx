import { Camera, Crop, Film, ImageUp, Trash2 } from "lucide-react";
import { useState } from "react";

import {
  isCreditGone,
  type Person,
  personFaceSrc,
  removePersonImage,
} from "../../api/credits";
import { updateFavorites } from "../../api/favorites";
import { errorText, t, type UiText } from "../../i18n";
import { formatBytes } from "../../lib/format";
import {
  UPLOAD_MAX_BYTES,
  UPLOAD_TYPES,
  uploadRefusal,
} from "../../player/thumbnailUpload/uploadRules";
import { DialogError } from "../../tags/DialogError";
import FavoriteToggle from "../../ui/FavoriteToggle";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Spinner } from "../../ui/shadcn/spinner";
import { creditDisplayName } from "../displayName";
import FaceCropDialog from "./FaceCropDialog";
import FacePickerDialog from "./FacePickerDialog";

const text = t.credits.personPage;

/** Take only the face from an image response, so a heart pressed meanwhile keeps its state. */
function withFaceOf(saved: Person) {
  return (current: Person): Person => ({
    ...current,
    imageRevision: saved.imageRevision,
  });
}

/**
 * The face with its heart, upload, reframing and removal; an image chosen, dropped or pasted in an ImageUploadDialog,
 * or the stored face, opens for framing. A missing person calls onGone.
 */
export default function PersonFaceControl({
  person,
  onChanged,
  onGone,
}: {
  person: Person;
  onChanged: (next: Person | ((current: Person) => Person)) => void;
  onGone: () => void;
}) {
  const [cropping, setCropping] = useState<File | string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<UiText | null>(null);
  const [picking, setPicking] = useState(false);
  const [uploading, setUploading] = useState(false);

  function pick() {
    setUploading(true);
  }

  function frame(file: File): UiText | null {
    const refusal = uploadRefusal(file);
    if (refusal === "type") return text.face.refusedType;
    if (refusal === "size") return text.face.tooLarge(formatBytes(UPLOAD_MAX_BYTES));
    setUploading(false);
    setCropping(file);
    return null;
  }

  async function remove() {
    setRemoveError(null);
    setRemoving(true);
    try {
      onChanged(withFaceOf(await removePersonImage(person.id)));
      setConfirmRemove(false);
    } catch (failure) {
      if (isCreditGone(failure)) {
        onGone();
        return;
      }
      setRemoveError(errorText(failure));
    } finally {
      setRemoving(false);
    }
  }

  async function toggleFavorite() {
    const favorite = !person.favorite;
    await updateFavorites([], [], favorite, { personIds: [person.id] });
    onChanged((current) => ({ ...current, favorite }));
  }

  const src = personFaceSrc(person);

  return (
    <div className="flex flex-col gap-2">
      <div className="group relative">
        <PersonFace
          src={src}
          name={creditDisplayName(person)}
          shape="profile"
          addLabel={text.face.add}
          onAdd={pick}
        >
          <PersonFaceMark corner="top-end">
            <FavoriteToggle
              favorite={person.favorite}
              label={person.favorite ? text.favoriteOn : text.favoriteOff}
              onToggle={toggleFavorite}
              variant="card"
            />
          </PersonFaceMark>
          <PersonFaceMark corner="bottom-end">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={text.face.change}
                  className="bg-overlay"
                >
                  <Camera aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setPicking(true)}>
                  <Film aria-hidden="true" />
                  {text.face.fromVideos}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={pick}>
                  <ImageUp aria-hidden="true" />
                  {text.face.upload}
                </DropdownMenuItem>
                {src !== undefined && (
                  <DropdownMenuItem onSelect={() => setCropping(src)}>
                    <Crop aria-hidden="true" />
                    {text.face.reframe}
                  </DropdownMenuItem>
                )}
                {src !== undefined && (
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setRemoveError(null);
                      setConfirmRemove(true);
                    }}
                  >
                    <Trash2 aria-hidden="true" />
                    {text.face.remove}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </PersonFaceMark>
        </PersonFace>
      </div>
      <ImageUploadDialog
        open={uploading}
        onOpenChange={setUploading}
        title={text.face.uploadTitle}
        description={text.face.uploadHint}
        accept={UPLOAD_TYPES}
        onFile={frame}
      />
      {cropping !== null && (
        <FaceCropDialog
          person={person}
          source={cropping}
          onClose={() => setCropping(null)}
          onSaved={(saved) => {
            setCropping(null);
            onChanged(withFaceOf(saved));
          }}
          onGone={onGone}
        />
      )}
      {picking && (
        <FacePickerDialog
          person={person}
          onClose={() => setPicking(false)}
          onSaved={(saved) => {
            setPicking(false);
            onChanged(withFaceOf(saved));
          }}
          onGone={onGone}
        />
      )}
      {confirmRemove && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open && !removing) setConfirmRemove(false);
          }}
          title={text.face.removeTitle(creditDisplayName(person))}
          description={text.face.removeText}
          cancelLabel={t.common.cancel}
          actionLabel={
            <>
              {removing && <Spinner aria-hidden="true" />}
              {removing ? text.face.removing : text.face.removeSubmit}
            </>
          }
          onConfirm={() => void remove()}
          pending={removing}
        >
          {removeError !== null && <DialogError message={removeError} />}
        </ConfirmDialog>
      )}
    </div>
  );
}
