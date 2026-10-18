import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../api/client";
import { formatDuration } from "../lib/format";
import {
  seekPosition,
  seekSpriteCell,
  type SpriteCell,
  spriteCellBackground,
} from "../lib/seekSprite";

export { seekSpriteCell, type SpriteCell };

const unavailableRetryMs = 5000;

interface PreviewOptions {
  durationMs: number;
  /** Without a sprite URL, retain the time/chapter preview without fetching images. */
  thumbnailUrl: string | undefined;
  chapterTitleAt?: (positionMs: number) => string | undefined;
  fetchSprite?: (url: string, signal: AbortSignal) => Promise<SeekThumbnailSprite>;
  fetchSheet?: (url: string, signal: AbortSignal) => Promise<Blob>;
}

export interface PreviewTarget {
  positionMs: number;
  leftPx: number;
}

export function seekPreviewTarget(
  clientX: number,
  rect: Pick<DOMRect, "left" | "width">,
  durationMs: number,
  previewWidth: number,
): PreviewTarget {
  const { localX, positionMs } = seekPosition(clientX, rect, durationMs);
  const halfWidth = Math.min(previewWidth / 2, rect.width / 2);
  return {
    positionMs,
    leftPx: Math.min(Math.max(localX, halfWidth), rect.width - halfWidth),
  };
}

type PreviewState = "hidden" | "loading" | "ready" | "unavailable";

/** Fetch each needed sheet once and retain object URLs until cleanup. Only detaching aborts in-flight sheet requests. */
export function attachSeekPreview(
  progress: HTMLElement,
  options: PreviewOptions,
  interactionTarget: HTMLElement = progress,
): () => void {
  const preview = document.createElement("div");
  preview.className = "vv-seek-preview";
  preview.dataset.state = "hidden";
  preview.setAttribute("aria-hidden", "true");

  const frame = document.createElement("div");
  frame.className = "vv-seek-preview-frame";
  const image = document.createElement("div");
  image.className = "vv-seek-preview-image";
  const pill = document.createElement("span");
  pill.className = "vv-seek-preview-time";
  const title = document.createElement("span");
  title.className = "vv-seek-preview-title";
  const time = document.createElement("span");
  pill.append(title, time);
  frame.append(image, pill);
  preview.append(frame);
  progress.append(preview);

  const fetchSprite = options.fetchSprite ?? fetchSeekThumbnailSprite;
  const fetchSheet = options.fetchSheet ?? fetchSeekThumbnailSheet;

  let sprite: SeekThumbnailSprite | null = null;
  let spriteRequest: AbortController | null = null;
  let spriteRetryAt = 0;
  const sheets = new Map<number, string>();
  const pendingSheets = new Map<number, AbortController>();
  const sheetRetryAt = new Map<number, number>();

  let visible = false;
  let detached = false;
  let activePointer: number | null = null;
  let positionMs = 0;
  let activeSheet: number | null = null;

  const setState = (state: PreviewState) => {
    preview.dataset.state = state;
  };
  const clearImage = () => {
    image.style.removeProperty("background-image");
  };

  const render = () => {
    if (!visible) return;
    const spriteUrl = options.thumbnailUrl;
    if (spriteUrl === undefined) {
      setState("unavailable");
      return;
    }
    if (sprite === null) {
      activeSheet = null;
      clearImage();
      if (spriteRequest !== null) {
        setState("loading");
      } else if (Date.now() < spriteRetryAt) {
        setState("unavailable");
      } else {
        setState("loading");
        loadSprite(spriteUrl);
      }
      return;
    }
    const cell = seekSpriteCell(sprite, positionMs);
    activeSheet = cell.sheet;
    const url = sheets.get(cell.sheet);
    if (url !== undefined) {
      showCell(url, cell, sprite);
      return;
    }
    clearImage();
    if (pendingSheets.has(cell.sheet)) {
      setState("loading");
      return;
    }
    const retryAt = sheetRetryAt.get(cell.sheet);
    if (retryAt !== undefined && Date.now() < retryAt) {
      setState("unavailable");
      return;
    }
    sheetRetryAt.delete(cell.sheet);
    setState("loading");
    loadSheet(sprite, cell.sheet);
  };

  const showCell = (url: string, cell: SpriteCell, layout: SeekThumbnailSprite) => {
    const background = spriteCellBackground(cell, layout);
    image.style.backgroundImage = `url("${url}")`;
    image.style.backgroundSize = background.backgroundSize;
    image.style.backgroundPosition = background.backgroundPosition;
    setState("ready");
  };

  const loadSprite = (spriteUrl: string) => {
    const controller = new AbortController();
    spriteRequest = controller;
    fetchSprite(spriteUrl, controller.signal)
      .then((loaded) => {
        if (controller.signal.aborted || detached) return;
        spriteRequest = null;
        sprite = loaded;
        preview.style.setProperty(
          "--vv-seek-frame-aspect",
          `${String(loaded.frameWidth)} / ${String(loaded.frameHeight)}`,
        );
        render();
      })
      .catch(() => {
        if (controller.signal.aborted || detached) return;
        spriteRequest = null;
        spriteRetryAt = Date.now() + unavailableRetryMs;
        if (visible && sprite === null) setState("unavailable");
      });
  };

  const loadSheet = (layout: SeekThumbnailSprite, sheet: number) => {
    const url = layout.sheets[sheet];
    if (url === undefined) {
      setState("unavailable");
      return;
    }
    const controller = new AbortController();
    pendingSheets.set(sheet, controller);
    fetchSheet(url, controller.signal)
      .then((blob) => {
        if (controller.signal.aborted || detached) return;
        pendingSheets.delete(sheet);
        sheets.set(sheet, URL.createObjectURL(blob));
        // Completing a nonvisible sheet must not change the displayed frame.
        if (activeSheet === sheet) render();
      })
      .catch(() => {
        if (controller.signal.aborted || detached) return;
        pendingSheets.delete(sheet);
        sheetRetryAt.set(sheet, Date.now() + unavailableRetryMs);
        if (visible && activeSheet === sheet) setState("unavailable");
      });
  };

  const hide = () => {
    visible = false;
    activeSheet = null;
    clearImage();
    setState("hidden");
  };
  const show = (event: PointerEvent) => {
    const rect = progress.getBoundingClientRect();
    if (rect.width <= 0) return;
    visible = true;
    if (preview.dataset.state === "hidden") setState("loading");
    const target = seekPreviewTarget(
      event.clientX,
      rect,
      options.durationMs,
      preview.offsetWidth,
    );
    preview.style.left = `${String(target.leftPx)}px`;
    time.textContent = formatDuration(target.positionMs);
    title.textContent = options.chapterTitleAt?.(target.positionMs) ?? "";
    positionMs = target.positionMs;
    render();
  };

  const onPointerEnter = (event: PointerEvent) => {
    if (event.pointerType !== "touch") show(event);
  };
  const onPointerMove = (event: PointerEvent) => {
    if (activePointer !== null || event.pointerType !== "touch") show(event);
  };
  const onPointerLeave = () => {
    if (activePointer === null) hide();
  };
  const onPointerDown = (event: PointerEvent) => {
    activePointer = event.pointerId;
    try {
      interactionTarget.setPointerCapture(event.pointerId);
    } catch {
      // Synthetic events and older browsers may not expose pointer capture.
    }
    show(event);
  };
  const onPointerEnd = (event: PointerEvent) => {
    if (activePointer !== event.pointerId) return;
    try {
      interactionTarget.releasePointerCapture(event.pointerId);
    } catch {
      // The capture may already have been released by the browser.
    }
    activePointer = null;
    const rect = interactionTarget.getBoundingClientRect();
    const remainsHovered =
      event.pointerType !== "touch" &&
      event.clientX >= rect.left &&
      event.clientX <= rect.right &&
      event.clientY >= rect.top &&
      event.clientY <= rect.bottom;
    if (remainsHovered) show(event);
    else hide();
  };

  interactionTarget.addEventListener("pointerenter", onPointerEnter);
  interactionTarget.addEventListener("pointermove", onPointerMove);
  interactionTarget.addEventListener("pointerleave", onPointerLeave);
  interactionTarget.addEventListener("pointerdown", onPointerDown);
  interactionTarget.addEventListener("pointerup", onPointerEnd);
  interactionTarget.addEventListener("pointercancel", onPointerEnd);

  return () => {
    hide();
    detached = true;
    spriteRequest?.abort();
    spriteRequest = null;
    for (const controller of pendingSheets.values()) controller.abort();
    pendingSheets.clear();
    for (const url of sheets.values()) URL.revokeObjectURL(url);
    sheets.clear();
    interactionTarget.removeEventListener("pointerenter", onPointerEnter);
    interactionTarget.removeEventListener("pointermove", onPointerMove);
    interactionTarget.removeEventListener("pointerleave", onPointerLeave);
    interactionTarget.removeEventListener("pointerdown", onPointerDown);
    interactionTarget.removeEventListener("pointerup", onPointerEnd);
    interactionTarget.removeEventListener("pointercancel", onPointerEnd);
    preview.remove();
  };
}
