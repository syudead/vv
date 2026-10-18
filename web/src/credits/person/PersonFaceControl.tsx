import { Camera, ImageUp, Trash2 } from "lucide-react";
import { type ChangeEvent, useRef, useState } from "react";

import { RequestFailed } from "../../api/client";
import {
  isCreditGone,
  type Person,
  personImagePath,
  removePersonImage,
  uploadPersonImage,
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
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Input } from "../../ui/shadcn/input";
import { Spinner } from "../../ui/shadcn/spinner";

const text = t.credits.personPage;

function uploadFailure(failure: unknown): UiText {
  if (failure instanceof RequestFailed) {
    if (failure.reason === "person_image_invalid") return text.face.refusedType;
    if (failure.reason === "person_image_too_large") {
      return text.face.tooLarge(formatBytes(failure.limit ?? UPLOAD_MAX_BYTES));
    }
  }
  return text.face.failed(errorText(failure));
}

/** The face with its heart and its upload and removal; a missing person calls onGone. */
export default function PersonFaceControl({
  person,
  onChanged,
  onGone,
}: {
  person: Person;
  onChanged: (next: Person | ((current: Person) => Person)) => void;
  onGone: () => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<UiText | null>(null);

  function pick() {
    if (!uploading) fileInput.current?.click();
  }

  async function upload(file: File) {
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      setError(
        refusal === "type"
          ? text.face.refusedType
          : text.face.tooLarge(formatBytes(UPLOAD_MAX_BYTES)),
      );
      return;
    }
    setError(null);
    setUploading(true);
    try {
      onChanged(await uploadPersonImage(person.id, file));
    } catch (failure) {
      if (isCreditGone(failure)) {
        onGone();
        return;
      }
      setError(uploadFailure(failure));
    } finally {
      setUploading(false);
    }
  }

  function choose(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Clear the value so choosing the same file again still fires change.
    event.target.value = "";
    if (file !== undefined) void upload(file);
  }

  async function remove() {
    setRemoveError(null);
    setRemoving(true);
    try {
      onChanged(await removePersonImage(person.id));
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

  const src =
    person.imageRevision === undefined
      ? undefined
      : personImagePath(person.id, person.imageRevision);

  return (
    <div className="flex flex-col gap-2">
      <div className="group relative">
        <PersonFace
          src={src}
          name={person.name}
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
          {src !== undefined && (
            <PersonFaceMark corner="bottom-end">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={text.face.change}
                    disabled={uploading}
                    className="bg-overlay"
                  >
                    <Camera aria-hidden="true" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={pick}>
                    <ImageUp aria-hidden="true" />
                    {text.face.upload}
                  </DropdownMenuItem>
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
                </DropdownMenuContent>
              </DropdownMenu>
            </PersonFaceMark>
          )}
          {uploading && (
            <div className="absolute inset-0 z-30 flex items-center justify-center bg-overlay">
              <Spinner aria-label={text.face.uploading} />
            </div>
          )}
        </PersonFace>
      </div>
      {error !== null && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Input
        ref={fileInput}
        type="file"
        accept={UPLOAD_TYPES}
        tabIndex={-1}
        aria-hidden="true"
        className="hidden"
        data-testid="person-face-input"
        onChange={choose}
      />
      {confirmRemove && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open && !removing) setConfirmRemove(false);
          }}
          title={text.face.removeTitle(person.name)}
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
