import { useEffect, useRef, useState } from "react";

import type { Person } from "../../api/credits";
import { t } from "../../i18n";
import { encodeReferenceURL } from "../../player/thumbnailEdit/editRules";
import { EditorDialog } from "../../ui/patterns/editor-dialog";
import { Spinner } from "../../ui/shadcn/spinner";
import { creditDisplayName } from "../displayName";
import {
  FacePreview,
  FaceSaveError,
  FaceSaveFooter,
  FrameStage,
  SizeControl,
  useFaceSave,
} from "./FaceFrameParts";
import {
  clampFaceFrame,
  type FaceFrame,
  faceFrameRect,
  initialFaceFrame,
} from "./faceFraming";

const text = t.credits.personPage.face;

type Picture =
  | { state: "loading" }
  | { state: "failed" }
  | { state: "ready"; image: HTMLImageElement; src: string };

/** A file decoded through an object URL that lives as long as the dialog, or a same-origin image URL. */
function usePicture(source: File | string): Picture {
  const [picture, setPicture] = useState<Picture>({ state: "loading" });
  useEffect(() => {
    const owned = typeof source !== "string";
    const url = owned ? URL.createObjectURL(source) : source;
    // Escape it as the thumbnail editor's reference is, so the URL never reads as markup.
    const src = encodeReferenceURL(url);
    const image = new Image();
    let current = true;
    image.onload = () => {
      if (current) setPicture({ state: "ready", image, src });
    };
    image.onerror = () => {
      if (current) setPicture({ state: "failed" });
    };
    image.src = src;
    return () => {
      current = false;
      if (owned) URL.revokeObjectURL(url);
    };
  }, [source]);
  return picture;
}

/** Frames a portrait in an uploaded, pasted or dropped image, or in the stored face, and saves it as the person's face. */
export default function FaceCropDialog({
  person,
  source,
  onClose,
  onSaved,
  onGone,
}: {
  person: Person;
  source: File | string;
  onClose: () => void;
  onSaved: (saved: Person) => void;
  onGone: () => void;
}) {
  const picture = usePicture(source);
  const [framing, setFraming] = useState<FaceFrame | null>(null);
  const { saving, error, save: saveFace } = useFaceSave(person, onSaved, onGone);
  const saveButton = useRef<HTMLButtonElement>(null);

  const image = picture.state === "ready" ? picture.image : null;
  const size =
    image === null ? null : { width: image.naturalWidth, height: image.naturalHeight };
  let current: FaceFrame | null = null;
  if (size !== null) {
    current =
      framing === null
        ? initialFaceFrame(size, undefined)
        : clampFaceFrame(size, framing);
  }

  function save() {
    if (image === null || size === null || current === null) return;
    void saveFace(image, faceFrameRect(size, current));
  }

  let stage;
  if (picture.state === "failed") {
    stage = <p className="text-sm text-muted-foreground">{text.refusedType}</p>;
  } else if (picture.state === "loading" || size === null || current === null) {
    stage = <Spinner aria-label={text.crop.loading} />;
  } else {
    stage = (
      <FrameStage
        src={picture.src}
        size={size}
        current={current}
        onFrame={setFraming}
        disabled={saving}
      />
    );
  }

  return (
    <EditorDialog
      open
      onClose={() => {
        if (!saving) onClose();
      }}
      title={text.crop.title}
      detail={creditDisplayName(person)}
      closeLabel={text.picker.close}
      closeDisabled={saving}
      initialFocus={saveButton}
      stage={stage}
      aside={
        <>
          {picture.state === "ready" && size !== null && current !== null && (
            <>
              <SizeControl
                size={size}
                current={current}
                onFrame={setFraming}
                disabled={saving}
              />
              <FacePreview
                src={picture.src}
                size={size}
                rect={faceFrameRect(size, current)}
              />
            </>
          )}
          <FaceSaveError error={error} />
        </>
      }
      footer={
        <FaceSaveFooter
          saveRef={saveButton}
          ready={current !== null}
          saving={saving}
          onSave={save}
          onCancel={onClose}
        />
      }
    />
  );
}
