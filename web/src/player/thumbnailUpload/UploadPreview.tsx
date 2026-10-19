import { Expand, LoaderCircle, RotateCcw, RotateCw, Shrink } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type WheelEvent,
} from "react";

import { t } from "../../i18n";
import { thumbnailBackdropClass } from "../../ui/ThumbnailBackdrop";
import { VideoThumbnail } from "../../ui/VideoThumbnail";
import { EditorDialog } from "../../ui/patterns/editor-dialog";
import { Button } from "../../ui/shadcn/button";
import { Slider } from "../../ui/shadcn/slider";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import FittedBox from "../thumbnailEdit/FittedBox";
import AiTouchUpSection from "./AiTouchUpSection";
import {
  drawComposition,
  drawOverflow,
  encodeComposition,
  type UploadImage,
} from "./composeImage";
import {
  type Fit,
  fillFrame,
  frameSize,
  maxZoom,
  MIN_ZOOM,
  panBy,
  rotateBy,
  type Size,
  snapToFrame,
  WHOLE_IMAGE,
  wholeImage,
  zoomTo,
} from "./framing";
import { useAiTouchUp } from "./useAiTouchUp";

const KEY_STEP = 0.02;

const ZOOM_STEP = 1.25;

/** Edges and centres pull within this share of the frame's long side. */
const SNAP = 0.015;

/** The overflow canvas is drawn at a quarter of the output's resolution. */
const OVERFLOW_SCALE = 0.25;

const VIDEO_THUMBNAIL_ASPECT = 16 / 9;

type SetFit = (update: (current: Fit) => Fit) => void;

/** Use the same composition as the upload inside the card's 16:9 frame; the image moves, zooms and turns freely, and what falls outside the frame shows faintly around it. With ComfyUI set up, a touch-up result can replace the image before it is applied. */
export default function UploadPreview({
  videoId,
  image,
  name,
  aspect,
  narrow,
  applying,
  initialFit = WHOLE_IMAGE,
  applyLabel,
  onApply,
  onCancel,
}: {
  videoId: number;
  image: UploadImage;
  name: string;
  aspect: number | null;
  narrow: boolean;
  applying: boolean;
  initialFit?: Fit;
  /** Replaces "Use as thumbnail" when applying only keeps the framing. */
  applyLabel?: string;
  /** replaced is a touch-up result now standing for image; the caller takes over its bitmap. */
  onApply: (fit: Fit, replaced: UploadImage | null) => void;
  onCancel: () => void;
}) {
  const text = t.player.thumbnailUpload.preview;
  const [fit, setFit] = useState<Fit>(initialFit);
  const applyButton = useRef<HTMLButtonElement | null>(null);
  const touchUp = usePreviewTouchUp(videoId, image, aspect, fit, setFit);
  const busy = applying || touchUp.busy;
  const source = touchUp.source;
  const size = frameSize(source, aspect, fit.rotation);
  const update: SetFit = (change) => {
    if (!busy) setFit(change);
  };
  const shown = touchUp.shown ?? { image: source, fit };

  const footer = (
    <>
      <Button
        ref={applyButton}
        size="lg"
        className="w-full"
        aria-disabled={busy || undefined}
        aria-busy={applying || undefined}
        onClick={() => {
          if (!busy) onApply(fit, touchUp.handOver());
        }}
      >
        {applying && (
          <LoaderCircle
            aria-hidden="true"
            className="animate-spin motion-reduce:animate-none"
          />
        )}
        {applyLabel ?? text.apply}
      </Button>
      <Button variant="ghost" className="w-full" disabled={applying} onClick={onCancel}>
        {text.cancel}
      </Button>
    </>
  );

  return (
    <EditorDialog
      open
      onClose={() => {
        if (!applying) onCancel();
      }}
      title={text.title}
      detail={name}
      closeLabel={text.close}
      closeDisabled={applying}
      initialFocus={applyButton}
      stage={
        <PreviewStage
          image={shown.image}
          aspect={aspect}
          size={frameSize(shown.image, aspect, shown.fit.rotation)}
          fit={shown.fit}
          narrow={narrow}
          onFit={update}
        />
      }
      aside={
        <>
          <FramingControls
            image={source}
            size={size}
            fit={fit}
            unknownAspect={aspect === null}
            disabled={busy}
            onFit={update}
          />
          {touchUp.ready && (
            <AiTouchUpSection
              state={touchUp.state}
              after={touchUp.after}
              disabled={applying}
              onRun={touchUp.run}
              onCancel={touchUp.cancel}
              onShowAfter={touchUp.showAfter}
              onUse={touchUp.use}
              onDismiss={touchUp.dismiss}
            />
          )}
        </>
      }
      footer={footer}
    />
  );
}

/** While a run or an unused result is pending, the framing and the apply wait; a used result becomes the image, framed whole. */
function usePreviewTouchUp(
  videoId: number,
  image: UploadImage,
  aspect: number | null,
  fit: Fit,
  setFit: (update: (current: Fit) => Fit) => void,
) {
  const ai = useAiTouchUp(videoId);
  const [replaced, setReplaced] = useState<UploadImage | null>(null);
  const [after, setAfter] = useState(true);
  const owned = useRef<UploadImage | null>(null);
  const handed = useRef(false);
  owned.current = replaced;
  useEffect(
    () => () => {
      if (!handed.current) owned.current?.bitmap.close();
    },
    [],
  );
  const source = replaced ?? image;
  const pending = ai.state.status === "result" ? ai.state.image : null;
  return {
    ready: ai.ready,
    state: ai.state,
    source,
    busy: ai.state.status === "running" || pending !== null,
    after,
    shown:
      pending !== null && after
        ? { image: pending, fit: { ...WHOLE_IMAGE, backdrop: fit.backdrop } }
        : null,
    run: (prompt: string) => {
      setAfter(true);
      ai.run(encodeComposition(source, aspect, fit), prompt);
    },
    cancel: ai.cancel,
    showAfter: setAfter,
    use: () => {
      const next = ai.take();
      if (next === null) return;
      replaced?.bitmap.close();
      setReplaced(next);
      setFit((current) => ({ ...WHOLE_IMAGE, backdrop: current.backdrop }));
    },
    dismiss: ai.dismiss,
    handOver: () => {
      handed.current = true;
      return replaced;
    },
  };
}

function PreviewStage({
  image,
  aspect,
  size,
  fit,
  narrow,
  onFit,
}: {
  image: UploadImage;
  aspect: number | null;
  size: Size;
  fit: Fit;
  narrow: boolean;
  onFit: SetFit;
}) {
  const text = t.player.thumbnailUpload.preview;
  // Dialog content mounts later; receive the canvases when they are inserted.
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const [backdrop, setBackdrop] = useState<HTMLCanvasElement | null>(null);
  const [overflow, setOverflow] = useState<HTMLCanvasElement | null>(null);
  const drag = usePreviewDrag(image, size, fit, onFit);

  useLayoutEffect(() => {
    if (canvas === null) return;
    drawComposition(canvas, image, aspect, fit);
    if (narrow && backdrop !== null) copyCanvas(canvas, backdrop);
  }, [canvas, backdrop, image, aspect, fit, narrow]);

  useLayoutEffect(() => {
    if (overflow !== null) drawOverflow(overflow, image, aspect, fit, OVERFLOW_SCALE);
  }, [overflow, image, aspect, fit]);

  const onKeyDown = (event: KeyboardEvent<HTMLCanvasElement>) => {
    const step = Math.max(size.width, size.height) * KEY_STEP;
    const moves: Record<string, [number, number]> = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    };
    const move = moves[event.key];
    if (move !== undefined) {
      event.preventDefault();
      onFit((current) => panBy(image, size, current, move[0], move[1]));
    } else if (event.key === "+" || event.key === "=" || event.key === "-") {
      event.preventDefault();
      const factor = event.key === "-" ? 1 / ZOOM_STEP : ZOOM_STEP;
      onFit((current) =>
        zoomTo(current, current.zoom * factor, maxZoom(image, size, current.rotation)),
      );
    }
  };

  const onWheel = (event: WheelEvent<HTMLCanvasElement>) => {
    onFit((current) =>
      zoomTo(
        current,
        current.zoom * Math.exp(-event.deltaY * 0.002),
        maxZoom(image, size, current.rotation),
      ),
    );
  };

  return (
    <FittedBox aspect={VIDEO_THUMBNAIL_ASPECT}>
      <div className="pointer-events-none absolute inset-0">
        <FittedBox aspect={size.width / size.height}>
          <div className="absolute -inset-full opacity-40">
            <canvas ref={setOverflow} aria-hidden="true" className="size-full" />
          </div>
        </FittedBox>
      </div>
      <VideoThumbnail data-testid="thumbnail-upload-preview">
        {narrow && (
          <canvas
            ref={setBackdrop}
            aria-hidden="true"
            className={thumbnailBackdropClass}
          />
        )}
        <canvas
          ref={setCanvas}
          role="img"
          aria-label={text.stage}
          tabIndex={0}
          className="relative size-full cursor-grab touch-none object-contain active:cursor-grabbing"
          onKeyDown={onKeyDown}
          onWheel={onWheel}
          {...drag}
        />
      </VideoThumbnail>
    </FittedBox>
  );
}

function FramingControls({
  image,
  size,
  fit,
  unknownAspect,
  disabled,
  onFit,
}: {
  image: UploadImage;
  size: Size;
  fit: Fit;
  unknownAspect: boolean;
  disabled: boolean;
  onFit: SetFit;
}) {
  const text = t.player.thumbnailUpload.preview;
  return (
    <>
      <div className="flex flex-col gap-2">
        <p className="text-sm font-medium">{text.position}</p>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => onFit(wholeImage)}
          >
            <Shrink aria-hidden="true" />
            {text.contain}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => onFit((current) => fillFrame(image, size, current))}
          >
            <Expand aria-hidden="true" />
            {text.cover}
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          {unknownAspect ? text.unknownAspect : text.moveHint}
        </p>
      </div>
      <div className="flex flex-col gap-2">
        <div className="flex items-baseline justify-between gap-2">
          <p className="text-sm font-medium">{text.zoom}</p>
          <span className="text-sm text-muted-foreground tabular-nums">
            {text.zoomValue(Math.round(fit.zoom * 100))}
          </span>
        </div>
        <Slider
          aria-label={text.zoom}
          min={Math.log2(MIN_ZOOM)}
          max={Math.log2(maxZoom(image, size, fit.rotation))}
          step={0.01}
          value={[Math.log2(fit.zoom)]}
          disabled={disabled}
          onValueChange={([value]) =>
            onFit((current) =>
              zoomTo(current, 2 ** (value ?? 0), maxZoom(image, size, current.rotation)),
            )
          }
        />
      </div>
      <div className="flex flex-col gap-2">
        <p className="text-sm font-medium">{text.turn}</p>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => onFit((current) => rotateBy(current, -90))}
          >
            <RotateCcw aria-hidden="true" />
            {text.turnLeft}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => onFit((current) => rotateBy(current, 90))}
          >
            <RotateCw aria-hidden="true" />
            {text.turnRight}
          </Button>
        </div>
      </div>
      <div className="flex flex-col gap-2">
        <p id="thumbnail-upload-backdrop" className="text-sm font-medium">
          {text.backdrop}
        </p>
        <ToggleGroup
          type="single"
          variant="outline"
          size="default"
          aria-labelledby="thumbnail-upload-backdrop"
          value={fit.backdrop}
          disabled={disabled}
          onValueChange={(value) => {
            if (value === "blur" || value === "black")
              onFit((current) => ({ ...current, backdrop: value }));
          }}
        >
          <ToggleGroupItem value="blur">{text.blur}</ToggleGroupItem>
          <ToggleGroupItem value="black">{text.black}</ToggleGroupItem>
        </ToggleGroup>
        <p className="text-sm text-muted-foreground">{text.backdropHint}</p>
      </div>
    </>
  );
}

/** Convert pointer movement from the object-contain display rectangle to output-image pixels; snapping applies to the unsnapped drag so an edge never sticks. */
function usePreviewDrag(image: UploadImage, size: Size, fit: Fit, onFit: SetFit) {
  const last = useRef<{ id: number; x: number; y: number; raw: Fit } | null>(null);
  const end = () => {
    last.current = null;
  };
  return {
    onPointerDown: (event: PointerEvent<HTMLCanvasElement>) => {
      event.currentTarget.setPointerCapture(event.pointerId);
      last.current = {
        id: event.pointerId,
        x: event.clientX,
        y: event.clientY,
        raw: fit,
      };
    },
    onPointerMove: (event: PointerEvent<HTMLCanvasElement>) => {
      const from = last.current;
      if (from?.id !== event.pointerId) return;
      const box = event.currentTarget.getBoundingClientRect();
      const scale = Math.max(size.width / box.width, size.height / box.height);
      const raw = panBy(
        image,
        size,
        from.raw,
        (event.clientX - from.x) * scale,
        (event.clientY - from.y) * scale,
      );
      last.current = { id: event.pointerId, x: event.clientX, y: event.clientY, raw };
      const threshold = Math.max(size.width, size.height) * SNAP;
      onFit(() => snapToFrame(image, size, raw, threshold));
    },
    onPointerUp: end,
    onPointerCancel: end,
  };
}

/** Downscale the blurred backdrop copy because fine detail is unnecessary. */
function copyCanvas(from: HTMLCanvasElement, to: HTMLCanvasElement): void {
  const width = Math.max(1, Math.round(from.width / 8));
  const height = Math.max(1, Math.round(from.height / 8));
  if (to.width !== width) to.width = width;
  if (to.height !== height) to.height = height;
  to.getContext("2d")?.drawImage(from, 0, 0, width, height);
}
