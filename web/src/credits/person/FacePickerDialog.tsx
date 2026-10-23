import { ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { isCreditGone, type Person } from "../../api/credits";
import {
  type FaceAppearance,
  faceCropPath,
  listPersonAppearances,
  videoFramePath,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { EditorDialog } from "../../ui/patterns/editor-dialog";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
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
  type Size,
} from "./faceFraming";

const text = t.credits.personPage.face.picker;

/** The faces offered, most like the person first; enough to choose from without paging. */
const FACE_CHOICES = 48;

const STEP_MS = 1000;

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
    readChoosableFaces(personId, controller.signal).then(
      (faces) => setChoices({ state: "ready", faces }),
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

/** Pages through the faces, most alike first, until FACE_CHOICES of them can be framed or none are left. */
async function readChoosableFaces(
  personId: number,
  signal: AbortSignal,
): Promise<Choosable[]> {
  const faces: Choosable[] = [];
  let cursor: string | undefined;
  do {
    const page = await listPersonAppearances(
      personId,
      { limit: FACE_CHOICES, sort: "mostSimilar", cursor },
      signal,
    );
    faces.push(...page.items.filter(choosable));
    cursor = page.nextCursor;
  } while (cursor !== undefined && faces.length < FACE_CHOICES);
  return faces.slice(0, FACE_CHOICES);
}

type Frame =
  | { state: "loading" }
  | { state: "failed"; src: string }
  | { state: "ready"; image: HTMLImageElement; src: string };

/**
 * The decoded frame at positionMs. A frame read for another face or moment counts as loading, so the stage and
 * Set as face never use a frame other than the one chosen.
 */
function useVideoFrame(videoId: number | undefined, positionMs: number): Frame {
  const [frame, setFrame] = useState<Frame>({ state: "loading" });
  const src = videoId === undefined ? undefined : videoFramePath(videoId, positionMs);
  useEffect(() => {
    if (src === undefined) return;
    const image = new Image();
    let current = true;
    image.onload = () => {
      if (current) setFrame({ state: "ready", image, src });
    };
    image.onerror = () => {
      if (current) setFrame({ state: "failed", src });
    };
    image.src = src;
    return () => {
      current = false;
    };
  }, [src]);
  return frame.state !== "loading" && frame.src === src ? frame : { state: "loading" };
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
  const {
    saving,
    error,
    clearError,
    save: saveFace,
  } = useFaceSave(person, onSaved, onGone);
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

  function save() {
    if (image === null || current === null || size === null) return;
    void saveFace(image, faceFrameRect(size, current));
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
        <PickerStage
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
              clearError();
            }}
            disabled={saving}
          />
          {chosen !== undefined && (
            <MomentControls
              positionMs={positionMs}
              onPosition={(next) => {
                setPositionMs(next);
                clearError();
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
          <FaceSaveError error={error} />
        </>
      }
      footer={
        <FaceSaveFooter
          saveRef={saveButton}
          ready={ready}
          saving={saving}
          onSave={save}
          onCancel={onClose}
        />
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

/** The chosen frame once it has loaded, its failure, or a spinner meanwhile. */
function PickerStage({
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
    <FrameStage
      src={frame.src}
      size={size}
      current={current}
      onFrame={onFrame}
      disabled={disabled}
    />
  );
}
