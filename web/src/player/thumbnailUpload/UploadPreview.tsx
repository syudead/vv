import { Expand, LoaderCircle, Shrink } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
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
import { drawComposition, type UploadImage } from "./composeImage";
import {
  CENTERED_COVER,
  compositionSize,
  type Fit,
  MAX_ZOOM,
  panBy,
  zoomTo,
} from "./framing";

const KEY_STEP = 0.02;

const ZOOM_STEP = 0.25;

const VIDEO_THUMBNAIL_ASPECT = 16 / 9;

/** Use the same composition as the upload inside the card's 16:9 frame. Without a video aspect ratio, only Whole image is available. */
export default function UploadPreview({
  image,
  name,
  aspect,
  narrow,
  applying,
  initialFit = { mode: "contain" },
  applyLabel,
  onApply,
  onCancel,
}: {
  image: UploadImage;
  name: string;
  aspect: number | null;
  narrow: boolean;
  applying: boolean;
  initialFit?: Fit;
  /** Replaces "Use as thumbnail" when applying only keeps the framing. */
  applyLabel?: string;
  onApply: (fit: Fit) => void;
  onCancel: () => void;
}) {
  const text = t.player.thumbnailUpload.preview;
  const [fit, setFit] = useState<Fit>(initialFit);
  // Dialog content mounts later; receive the canvas when it is inserted.
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const [backdrop, setBackdrop] = useState<HTMLCanvasElement | null>(null);
  const applyButton = useRef<HTMLButtonElement | null>(null);
  const size = compositionSize(image, aspect);
  const drag = usePreviewDrag(image, size, setFit);

  useLayoutEffect(() => {
    if (canvas === null) return;
    drawComposition(canvas, image, aspect, fit);
    if (narrow && backdrop !== null) copyCanvas(canvas, backdrop);
  }, [canvas, backdrop, image, aspect, fit, narrow]);

  const cover = fit.mode === "cover";

  const onKeyDown = (event: KeyboardEvent<HTMLCanvasElement>) => {
    if (!cover || applying) return;
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
      setFit((current) => panBy(image, size, current, move[0], move[1]));
    } else if (event.key === "+" || event.key === "=" || event.key === "-") {
      event.preventDefault();
      const delta = event.key === "-" ? -ZOOM_STEP : ZOOM_STEP;
      setFit((current) => zoomTo(current, zoomOf(current) + delta));
    }
  };

  const onWheel = (event: WheelEvent<HTMLCanvasElement>) => {
    if (!cover || applying) return;
    setFit((current) =>
      zoomTo(current, zoomOf(current) * Math.exp(-event.deltaY * 0.002)),
    );
  };

  const stage = (
    <FittedBox aspect={VIDEO_THUMBNAIL_ASPECT}>
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
          aria-label={cover ? text.stageCover : text.stage}
          tabIndex={cover ? 0 : -1}
          data-fit={fit.mode}
          className={
            cover
              ? "relative size-full cursor-grab touch-none object-contain active:cursor-grabbing"
              : "relative size-full object-contain"
          }
          onKeyDown={onKeyDown}
          onWheel={onWheel}
          {...(cover && !applying ? drag : {})}
        />
      </VideoThumbnail>
    </FittedBox>
  );

  const aside = (
    <>
      <div className="flex flex-col gap-2">
        <p id="thumbnail-upload-fit" className="text-sm font-medium">
          {text.fit}
        </p>
        <ToggleGroup
          type="single"
          variant="outline"
          size="default"
          aria-labelledby="thumbnail-upload-fit"
          value={fit.mode}
          disabled={applying}
          onValueChange={(value) => {
            if (value === "contain") setFit({ mode: "contain" });
            if (value === "cover") setFit(CENTERED_COVER);
          }}
        >
          <ToggleGroupItem value="contain">
            <Shrink aria-hidden="true" />
            {text.contain}
          </ToggleGroupItem>
          <ToggleGroupItem value="cover" disabled={aspect === null}>
            <Expand aria-hidden="true" />
            {text.cover}
          </ToggleGroupItem>
        </ToggleGroup>
        <p className="text-sm text-muted-foreground">
          {aspect === null
            ? text.unknownAspect
            : cover
              ? text.coverHint
              : text.containHint}
        </p>
      </div>
      {cover && (
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium">{text.zoom}</p>
          <Slider
            aria-label={text.zoom}
            min={1}
            max={MAX_ZOOM}
            step={0.05}
            value={[fit.zoom]}
            disabled={applying}
            onValueChange={([value]) => setFit((current) => zoomTo(current, value ?? 1))}
          />
        </div>
      )}
    </>
  );

  const footer = (
    <>
      <Button
        ref={applyButton}
        size="lg"
        className="w-full"
        aria-disabled={applying || undefined}
        aria-busy={applying || undefined}
        onClick={() => {
          if (!applying) onApply(fit);
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
      stage={stage}
      aside={aside}
      footer={footer}
    />
  );
}

function zoomOf(fit: Fit): number {
  return fit.mode === "cover" ? fit.zoom : 1;
}

/** Convert pointer movement from the object-contain display rectangle to output-image pixels. */
function usePreviewDrag(
  image: UploadImage,
  size: { width: number; height: number },
  setFit: (update: (current: Fit) => Fit) => void,
) {
  const last = useRef<{ id: number; x: number; y: number } | null>(null);
  return {
    onPointerDown: (event: PointerEvent<HTMLCanvasElement>) => {
      event.currentTarget.setPointerCapture(event.pointerId);
      last.current = { id: event.pointerId, x: event.clientX, y: event.clientY };
    },
    onPointerMove: (event: PointerEvent<HTMLCanvasElement>) => {
      const from = last.current;
      if (from?.id !== event.pointerId) return;
      const box = event.currentTarget.getBoundingClientRect();
      const scale = Math.max(size.width / box.width, size.height / box.height);
      const dx = (event.clientX - from.x) * scale;
      const dy = (event.clientY - from.y) * scale;
      last.current = { id: event.pointerId, x: event.clientX, y: event.clientY };
      setFit((current) => panBy(image, size, current, dx, dy));
    },
    onPointerUp: () => {
      last.current = null;
    },
    onPointerCancel: () => {
      last.current = null;
    },
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
