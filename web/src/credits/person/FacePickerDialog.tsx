import { ChevronLeft, ChevronRight, LoaderCircle } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
  useEffect,
  useRef,
  useState,
} from "react";

import { isCreditGone, type Person, uploadPersonImage } from "../../api/credits";
import { cn } from "../../lib/cn";
import {
  type FaceAppearance,
  faceCropPath,
  listPersonAppearances,
  videoFramePath,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import FittedBox from "../../player/thumbnailEdit/FittedBox";
import { EditorDialog } from "../../ui/patterns/editor-dialog";
import { Button } from "../../ui/shadcn/button";
import { Slider } from "../../ui/shadcn/slider";
import { Spinner } from "../../ui/shadcn/spinner";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import { creditDisplayName } from "../displayName";
import { encodeFaceFrame } from "./faceFrameImage";
import {
  clampFaceFrame,
  type FaceFrame,
  faceFrameRect,
  initialFaceFrame,
  maxFaceWidth,
  MIN_FACE_WIDTH,
  moveFaceFrame,
  type Rect,
  type Size,
} from "./faceFraming";
import { faceUploadFailure } from "./faceUploadFailure";

const text = t.credits.personPage.face.picker;

/** The faces offered, most like the person first; enough to choose from without paging. */
const FACE_CHOICES = 48;

const STEP_MS = 1000;

/** Arrow keys move the frame by this share of the image's width. */
const KEY_STEP = 0.01;

type Choosable = FaceAppearance & {
  videoId: number;
  focus: NonNullable<FaceAppearance["focus"]>;
};

function choosable(face: FaceAppearance): face is Choosable {
  return face.videoId !== undefined && face.focus !== undefined;
}

type Choices =
  | { state: "loading" }
  | { state: "failed"; reason: UiText }
  | { state: "ready"; faces: Choosable[] };

function useFaceChoices(personId: number, onGone: () => void): Choices {
  const [choices, setChoices] = useState<Choices>({ state: "loading" });
  const gone = useRef(onGone);
  gone.current = onGone;
  useEffect(() => {
    const controller = new AbortController();
    listPersonAppearances(
      personId,
      { limit: FACE_CHOICES, sort: "mostSimilar" },
      controller.signal,
    ).then(
      (list) => setChoices({ state: "ready", faces: list.items.filter(choosable) }),
      (failure: unknown) => {
        if (controller.signal.aborted) return;
        if (isCreditGone(failure)) gone.current();
        else setChoices({ state: "failed", reason: errorText(failure) });
      },
    );
    return () => controller.abort();
  }, [personId]);
  return choices;
}

type Frame =
  | { state: "loading" }
  | { state: "failed" }
  | { state: "ready"; image: HTMLImageElement; src: string };

/** The decoded frame at positionMs; the previous frame stays shown until the next one is ready. */
function useVideoFrame(videoId: number | undefined, positionMs: number): Frame {
  const [frame, setFrame] = useState<Frame>({ state: "loading" });
  useEffect(() => {
    if (videoId === undefined) return;
    const src = videoFramePath(videoId, positionMs);
    const image = new Image();
    let current = true;
    image.onload = () => {
      if (current) setFrame({ state: "ready", image, src });
    };
    image.onerror = () => {
      if (current) setFrame({ state: "failed" });
    };
    image.src = src;
    return () => {
      current = false;
    };
  }, [videoId, positionMs]);
  return frame;
}

function sizeOf(image: HTMLImageElement): Size {
  return { width: image.naturalWidth, height: image.naturalHeight };
}

/**
 * `Choose from videos…`: frames a portrait of the person in the original frame of one of their faces, a second
 * earlier or later when needed, and saves it as their face.
 */
export default function FacePickerDialog({
  person,
  onClose,
  onSaved,
  onGone,
}: {
  person: Person;
  onClose: () => void;
  onSaved: (saved: Person) => void;
  onGone: () => void;
}) {
  const choices = useFaceChoices(person.id, onGone);
  const faces = choices.state === "ready" ? choices.faces : [];
  const [chosenId, setChosenId] = useState<number | null>(null);
  const chosen = faces.find((face) => face.appearanceId === chosenId) ?? faces.at(0);
  const [positionMs, setPositionMs] = useState(0);
  const [framing, setFraming] = useState<{ id: number; frame: FaceFrame } | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const saveButton = useRef<HTMLButtonElement>(null);

  // A newly chosen face starts at the middle of its longest track and is framed anew.
  const shownId = chosen?.appearanceId;
  const [startedId, setStartedId] = useState<number | undefined>(undefined);
  if (chosen !== undefined && shownId !== startedId) {
    setStartedId(shownId);
    setPositionMs(Math.round(chosen.focus.time * 1000));
  }
  const frame = useVideoFrame(chosen?.videoId, positionMs);
  const image = frame.state === "ready" ? frame.image : null;
  const size = image === null ? null : sizeOf(image);
  let current: FaceFrame | null = null;
  if (size !== null && chosen !== undefined) {
    current =
      framing?.id === chosen.appearanceId
        ? clampFaceFrame(size, framing.frame)
        : initialFaceFrame(size, chosen.focus);
  }
  const setFrame = (next: FaceFrame) => {
    if (chosen !== undefined) setFraming({ id: chosen.appearanceId, frame: next });
  };

  async function save() {
    if (image === null || current === null || size === null || saving) return;
    setSaving(true);
    setError(null);
    try {
      const blob = await encodeFaceFrame(image, faceFrameRect(size, current));
      onSaved(await uploadPersonImage(person.id, blob));
    } catch (failure) {
      setSaving(false);
      if (isCreditGone(failure)) onGone();
      else setError(faceUploadFailure(failure));
    }
  }

  const ready = frame.state === "ready" && current !== null;
  return (
    <EditorDialog
      open
      onClose={() => {
        if (!saving) onClose();
      }}
      title={text.title}
      detail={creditDisplayName(person)}
      closeLabel={text.close}
      closeDisabled={saving}
      initialFocus={saveButton}
      stage={
        <FrameStage
          frame={frame}
          size={size}
          current={current}
          onFrame={setFrame}
          disabled={saving}
        />
      }
      aside={
        <>
          <FaceChoices
            choices={choices}
            chosenId={shownId}
            onChoose={(id) => {
              setChosenId(id);
              setError(null);
            }}
            disabled={saving}
          />
          {chosen !== undefined && (
            <MomentControls
              positionMs={positionMs}
              onPosition={(next) => {
                setPositionMs(next);
                setError(null);
              }}
              disabled={saving}
            />
          )}
          {size !== null && current !== null && (
            <SizeControl
              size={size}
              current={current}
              onFrame={setFrame}
              disabled={saving}
            />
          )}
          {frame.state === "ready" && size !== null && current !== null && (
            <FacePreview
              src={frame.src}
              size={size}
              rect={faceFrameRect(size, current)}
            />
          )}
          {error !== null && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </>
      }
      footer={
        <>
          <Button
            ref={saveButton}
            size="lg"
            className="w-full"
            aria-disabled={!ready || saving || undefined}
            aria-busy={saving || undefined}
            onClick={() => void save()}
          >
            {saving && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {saving ? text.saving : text.save}
          </Button>
          <Button variant="ghost" className="w-full" disabled={saving} onClick={onClose}>
            {text.cancel}
          </Button>
        </>
      }
    />
  );
}

function FaceChoices({
  choices,
  chosenId,
  onChoose,
  disabled,
}: {
  choices: Choices;
  chosenId: number | undefined;
  onChoose: (id: number) => void;
  disabled: boolean;
}) {
  if (choices.state === "loading") {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner aria-hidden="true" />
        {text.loading}
      </p>
    );
  }
  if (choices.state === "failed") {
    return (
      <p role="alert" className="text-sm text-destructive">
        {text.loadFailed(choices.reason)}
      </p>
    );
  }
  if (choices.faces.length === 0) {
    return <p className="text-sm text-muted-foreground">{text.noFaces}</p>;
  }
  return (
    <section className="flex flex-col gap-2" aria-labelledby="face-picker-faces">
      <h3 id="face-picker-faces" className="text-sm font-medium">
        {text.faces}
      </h3>
      <p className="text-sm text-muted-foreground">{text.facesHint}</p>
      <ToggleGroup
        type="single"
        value={chosenId === undefined ? "" : String(chosenId)}
        onValueChange={(value) => {
          if (value !== "") onChoose(Number(value));
        }}
        disabled={disabled}
        className="flex w-full gap-2 overflow-x-auto pb-1 lg:grid lg:grid-cols-5 lg:overflow-visible"
      >
        {choices.faces.map((face) => (
          <ToggleGroupItem
            key={face.appearanceId}
            value={String(face.appearanceId)}
            aria-label={text.face(face.title)}
            title={face.title}
            className="h-auto flex-none rounded-md p-0.5 first:rounded-md last:rounded-md data-[state=on]:ring-2 data-[state=on]:ring-primary"
          >
            {face.hasCrop ? (
              <img
                src={faceCropPath(face.appearanceId)}
                alt=""
                className="size-16 rounded-sm object-cover lg:aspect-square lg:size-auto lg:w-full"
              />
            ) : (
              <span className="flex size-16 items-center justify-center text-xs text-muted-foreground">
                {formatDuration(face.focus.time * 1000)}
              </span>
            )}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </section>
  );
}

function MomentControls({
  positionMs,
  onPosition,
  disabled,
}: {
  positionMs: number;
  onPosition: (positionMs: number) => void;
  disabled: boolean;
}) {
  return (
    <section className="flex flex-col gap-2" aria-labelledby="face-picker-frame">
      <h3 id="face-picker-frame" className="text-sm font-medium">
        {text.frame}
      </h3>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={text.earlier}
          disabled={disabled || positionMs <= 0}
          onClick={() => onPosition(Math.max(0, positionMs - STEP_MS))}
        >
          <ChevronLeft aria-hidden="true" />
        </Button>
        <span className="min-w-0 flex-1 text-center text-sm tabular-nums">
          {text.moment(formatDuration(positionMs))}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={text.later}
          disabled={disabled}
          onClick={() => onPosition(positionMs + STEP_MS)}
        >
          <ChevronRight aria-hidden="true" />
        </Button>
      </div>
    </section>
  );
}

function SizeControl({
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

/** The frame with the portrait outlined and the rest dimmed; the outline drags and takes arrow keys. */
function FrameStage({
  frame,
  size,
  current,
  onFrame,
  disabled,
}: {
  frame: Frame;
  size: Size | null;
  current: FaceFrame | null;
  onFrame: (frame: FaceFrame) => void;
  disabled: boolean;
}) {
  if (frame.state === "failed") {
    return <p className="text-sm text-muted-foreground">{text.frameFailed}</p>;
  }
  if (frame.state === "loading" || size === null || current === null) {
    return <Spinner aria-label={text.frameLoading} />;
  }
  return (
    <FittedBox aspect={size.width / size.height}>
      <img src={frame.src} alt="" draggable={false} className="size-full select-none" />
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
function FacePreview({ src, size, rect }: { src: string; size: Size; rect: Rect }) {
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
