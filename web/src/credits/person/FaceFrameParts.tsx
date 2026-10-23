import { LoaderCircle } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
  useRef,
  useState,
} from "react";

import { isCreditGone, type Person, uploadPersonImage } from "../../api/credits";
import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import FittedBox from "../../player/thumbnailEdit/FittedBox";
import { Button } from "../../ui/shadcn/button";
import { Slider } from "../../ui/shadcn/slider";
import { encodeFaceFrame } from "./faceFrameImage";
import {
  clampFaceFrame,
  type FaceFrame,
  faceFrameRect,
  maxFaceWidth,
  MIN_FACE_WIDTH,
  moveFaceFrame,
  type Rect,
  type Size,
} from "./faceFraming";
import { faceUploadFailure } from "./faceUploadFailure";

// The framing controls shared by choosing a face from videos and framing an uploaded image.

const text = t.credits.personPage.face.picker;

/** Arrow keys move the frame by this share of the image's width. */
const KEY_STEP = 0.01;

/** Cuts rect out of image and saves it as the face; a refusal stays for the dialog to show, a missing person calls onGone. */
export function useFaceSave(
  person: Person,
  onSaved: (saved: Person) => void,
  onGone: () => void,
) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  async function save(image: HTMLImageElement, rect: Rect) {
    if (saving) return;
    setSaving(true);
    setError(null);
    try {
      const blob = await encodeFaceFrame(image, rect);
      onSaved(await uploadPersonImage(person.id, blob));
    } catch (failure) {
      setSaving(false);
      if (isCreditGone(failure)) onGone();
      else setError(faceUploadFailure(failure));
    }
  }

  return { saving, error, clearError: () => setError(null), save };
}

export function FaceSaveError({ error }: { error: UiText | null }) {
  if (error === null) return null;
  return (
    <p role="alert" className="text-sm text-destructive">
      {error}
    </p>
  );
}

/** `Set as face`, held until ready, and `Cancel`. */
export function FaceSaveFooter({
  saveRef,
  ready,
  saving,
  onSave,
  onCancel,
}: {
  saveRef: RefObject<HTMLButtonElement | null>;
  ready: boolean;
  saving: boolean;
  onSave: () => void;
  onCancel: () => void;
}) {
  return (
    <>
      <Button
        ref={saveRef}
        size="lg"
        className="w-full"
        aria-disabled={!ready || saving || undefined}
        aria-busy={saving || undefined}
        onClick={onSave}
      >
        {saving && (
          <LoaderCircle
            aria-hidden="true"
            className="animate-spin motion-reduce:animate-none"
          />
        )}
        {saving ? text.saving : text.save}
      </Button>
      <Button variant="ghost" className="w-full" disabled={saving} onClick={onCancel}>
        {text.cancel}
      </Button>
    </>
  );
}

export function SizeControl({
  size,
  current,
  onFrame,
  disabled,
}: {
  size: Size;
  current: FaceFrame;
  onFrame: (frame: FaceFrame) => void;
  disabled: boolean;
}) {
  const max = maxFaceWidth(size);
  return (
    <section className="flex flex-col gap-2" aria-labelledby="face-picker-size">
      <h3 id="face-picker-size" className="text-sm font-medium">
        {text.size}
      </h3>
      <Slider
        aria-label={text.size}
        min={Math.min(MIN_FACE_WIDTH, max)}
        max={max}
        step={max / 200}
        value={[current.width]}
        disabled={disabled}
        onValueChange={([width]) => {
          if (width !== undefined) onFrame(clampFaceFrame(size, { ...current, width }));
        }}
      />
      <p className="text-sm text-muted-foreground">{text.sizeHint}</p>
    </section>
  );
}

/** The image with the portrait outlined and the rest dimmed; the outline drags and takes arrow keys. */
export function FrameStage({
  src,
  size,
  current,
  onFrame,
  disabled,
}: {
  src: string;
  size: Size;
  current: FaceFrame;
  onFrame: (frame: FaceFrame) => void;
  disabled: boolean;
}) {
  return (
    <FittedBox aspect={size.width / size.height}>
      <img src={src} alt="" draggable={false} className="size-full select-none" />
      <FrameOutline size={size} current={current} onFrame={onFrame} disabled={disabled} />
    </FittedBox>
  );
}

function percent(value: number): string {
  return `${String(value * 100)}%`;
}

function FrameOutline({
  size,
  current,
  onFrame,
  disabled,
}: {
  size: Size;
  current: FaceFrame;
  onFrame: (frame: FaceFrame) => void;
  disabled: boolean;
}) {
  const drag = useRef<{ x: number; y: number; frame: FaceFrame } | null>(null);
  const rect = faceFrameRect(size, current);
  const left = rect.x / size.width;
  const top = rect.y / size.height;
  const width = rect.width / size.width;
  const height = rect.height / size.height;

  function start(event: PointerEvent<HTMLDivElement>) {
    if (disabled) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { x: event.clientX, y: event.clientY, frame: current };
  }

  function move(event: PointerEvent<HTMLDivElement>) {
    const from = drag.current;
    const shown = event.currentTarget.parentElement?.clientWidth ?? 0;
    if (from === null || shown === 0) return;
    onFrame(
      moveFaceFrame(
        size,
        from.frame,
        event.clientX - from.x,
        event.clientY - from.y,
        shown,
      ),
    );
  }

  function key(event: KeyboardEvent<HTMLDivElement>) {
    const steps: Record<string, [number, number]> = {
      ArrowLeft: [-1, 0],
      ArrowRight: [1, 0],
      ArrowUp: [0, -1],
      ArrowDown: [0, 1],
    };
    const step = steps[event.key];
    if (step === undefined || disabled) return;
    event.preventDefault();
    onFrame(moveFaceFrame(size, current, step[0] * KEY_STEP, step[1] * KEY_STEP, 1));
  }

  return (
    <>
      <div
        className="absolute inset-x-0 top-0 bg-overlay opacity-60"
        style={{ height: percent(top) }}
      />
      <div
        className="absolute inset-x-0 bottom-0 bg-overlay opacity-60"
        style={{ height: percent(1 - top - height) }}
      />
      <div
        className="absolute left-0 bg-overlay opacity-60"
        style={{ top: percent(top), height: percent(height), width: percent(left) }}
      />
      <div
        className="absolute right-0 bg-overlay opacity-60"
        style={{
          top: percent(top),
          height: percent(height),
          width: percent(1 - left - width),
        }}
      />
      <div
        role="application"
        aria-label={text.area}
        tabIndex={disabled ? -1 : 0}
        className="absolute cursor-move touch-none rounded-sm border-2 border-primary"
        style={{
          left: percent(left),
          top: percent(top),
          width: percent(width),
          height: percent(height),
        }}
        onPointerDown={start}
        onPointerMove={move}
        onPointerUp={() => (drag.current = null)}
        onPointerCancel={() => (drag.current = null)}
        onKeyDown={key}
      />
    </>
  );
}

/** The portrait as the profile shows it, and the round crop the cards take from it. */
export function FacePreview({
  src,
  size,
  rect,
}: {
  src: string;
  size: Size;
  rect: Rect;
}) {
  // Cards show the portrait's square a third of the way down, as `object-face` places it.
  const square: Rect = {
    x: rect.x,
    y: rect.y + (rect.height - rect.width) / 3,
    width: rect.width,
    height: rect.width,
  };
  return (
    <section className="flex flex-col gap-2" aria-labelledby="face-picker-preview">
      <h3 id="face-picker-preview" className="text-sm font-medium">
        {text.preview}
      </h3>
      <div className="flex items-end gap-4">
        <PreviewCut
          src={src}
          size={size}
          rect={rect}
          className="aspect-portrait w-card-face rounded-md"
        />
        <PreviewCut
          src={src}
          size={size}
          rect={square}
          className="size-16 rounded-full"
        />
      </div>
    </section>
  );
}

function PreviewCut({
  src,
  size,
  rect,
  className,
}: {
  src: string;
  size: Size;
  rect: Rect;
  className: string;
}) {
  return (
    <div className={cn("relative shrink-0 overflow-hidden bg-muted", className)}>
      <img
        src={src}
        alt=""
        className="absolute max-w-none"
        style={{
          width: percent(size.width / rect.width),
          left: percent(-rect.x / rect.width),
          top: percent(-rect.y / rect.height),
        }}
      />
    </div>
  );
}
