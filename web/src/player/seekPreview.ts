import { formatDuration } from "../lib/format";

const bucketMs = 1000;
const debounceMs = 150;
const cacheLimit = 12;

interface PreviewOptions {
  durationMs: number;
  thumbnailUrl: string;
  fetchImage?: (url: string, signal: AbortSignal) => Promise<Blob>;
}

export interface PreviewTarget {
  positionMs: number;
  requestPositionMs: number;
  leftPx: number;
}

export function seekPreviewTarget(
  clientX: number,
  rect: Pick<DOMRect, "left" | "width">,
  durationMs: number,
  previewWidth: number,
): PreviewTarget {
  const localX = Math.min(Math.max(clientX - rect.left, 0), rect.width);
  const ratio = rect.width > 0 ? localX / rect.width : 0;
  const lastPositionMs = Math.max(0, Math.ceil(durationMs) - 1);
  const positionMs = Math.min(lastPositionMs, Math.round(ratio * durationMs));
  const requestPositionMs = Math.min(
    lastPositionMs,
    Math.round(positionMs / bucketMs) * bucketMs,
  );
  const halfWidth = Math.min(previewWidth / 2, rect.width / 2);
  return {
    positionMs,
    requestPositionMs,
    leftPx: Math.min(Math.max(localX, halfWidth), rect.width - halfWidth),
  };
}

export function attachSeekPreview(
  progress: HTMLElement,
  options: PreviewOptions,
): () => void {
  const preview = document.createElement("div");
  preview.className = "vv-seek-preview";
  preview.dataset.state = "hidden";
  preview.setAttribute("aria-hidden", "true");

  const frame = document.createElement("div");
  frame.className = "vv-seek-preview-frame";
  const image = document.createElement("img");
  image.alt = "";
  const time = document.createElement("span");
  time.className = "vv-seek-preview-time";
  frame.append(image, time);
  preview.append(frame);
  progress.append(preview);

  const fetchImage = options.fetchImage ?? fetchThumbnail;
  const cache = new Map<number, string>();
  const unavailable = new Set<number>();
  let activeBucket: number | null = null;
  let activePointer: number | null = null;
  let visible = false;
  let request: AbortController | null = null;
  let timer: number | null = null;

  const cancelPending = () => {
    if (timer !== null) window.clearTimeout(timer);
    timer = null;
    request?.abort();
    request = null;
  };
  const setState = (state: "hidden" | "loading" | "ready" | "unavailable") => {
    preview.dataset.state = state;
  };
  const hide = () => {
    visible = false;
    activeBucket = null;
    cancelPending();
    image.removeAttribute("src");
    setState("hidden");
  };
  const show = (event: PointerEvent) => {
    const rect = progress.getBoundingClientRect();
    if (rect.width <= 0) return;
    visible = true;
    setState(
      preview.dataset.state === "hidden"
        ? "loading"
        : (preview.dataset.state as "loading" | "ready" | "unavailable"),
    );
    const nextBucket = seekPreviewTarget(
      event.clientX,
      rect,
      options.durationMs,
      0,
    ).requestPositionMs;
    if (activeBucket !== nextBucket && preview.dataset.state === "unavailable") {
      setState("loading");
    }
    const target = seekPreviewTarget(
      event.clientX,
      rect,
      options.durationMs,
      preview.offsetWidth,
    );
    preview.style.left = `${String(target.leftPx)}px`;
    time.textContent = formatDuration(target.positionMs);

    if (activeBucket === target.requestPositionMs) return;
    activeBucket = target.requestPositionMs;
    cancelPending();
    image.removeAttribute("src");

    const cached = cache.get(activeBucket);
    if (cached !== undefined) {
      image.src = cached;
      setState("ready");
      return;
    }
    if (unavailable.has(activeBucket)) {
      setState("unavailable");
      return;
    }

    setState("loading");
    const requestedBucket = activeBucket;
    timer = window.setTimeout(() => {
      timer = null;
      const controller = new AbortController();
      request = controller;
      const url = thumbnailRequestUrl(options.thumbnailUrl, requestedBucket);
      void fetchImage(url, controller.signal)
        .then((blob) => {
          if (controller.signal.aborted) return;
          const objectUrl = URL.createObjectURL(blob);
          remember(cache, requestedBucket, objectUrl);
          if (!visible || activeBucket !== requestedBucket) return;
          image.src = objectUrl;
          setState("ready");
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted) return;
          unavailable.add(requestedBucket);
          if (visible && activeBucket === requestedBucket) setState("unavailable");
          if (error instanceof Error && error.name === "AbortError") return;
        })
        .finally(() => {
          if (request === controller) request = null;
        });
    }, debounceMs);
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
      progress.setPointerCapture(event.pointerId);
    } catch {
      // Synthetic events and older browsers may not expose pointer capture.
    }
    show(event);
  };
  const onPointerEnd = (event: PointerEvent) => {
    if (activePointer !== event.pointerId) return;
    try {
      progress.releasePointerCapture(event.pointerId);
    } catch {
      // The capture may already have been released by the browser.
    }
    activePointer = null;
    const rect = progress.getBoundingClientRect();
    const remainsHovered =
      event.pointerType !== "touch" &&
      event.clientX >= rect.left &&
      event.clientX <= rect.right &&
      event.clientY >= rect.top &&
      event.clientY <= rect.bottom;
    if (remainsHovered) show(event);
    else hide();
  };

  progress.addEventListener("pointerenter", onPointerEnter);
  progress.addEventListener("pointermove", onPointerMove);
  progress.addEventListener("pointerleave", onPointerLeave);
  progress.addEventListener("pointerdown", onPointerDown);
  progress.addEventListener("pointerup", onPointerEnd);
  progress.addEventListener("pointercancel", onPointerEnd);

  return () => {
    hide();
    progress.removeEventListener("pointerenter", onPointerEnter);
    progress.removeEventListener("pointermove", onPointerMove);
    progress.removeEventListener("pointerleave", onPointerLeave);
    progress.removeEventListener("pointerdown", onPointerDown);
    progress.removeEventListener("pointerup", onPointerEnd);
    progress.removeEventListener("pointercancel", onPointerEnd);
    for (const objectUrl of cache.values()) URL.revokeObjectURL(objectUrl);
    preview.remove();
  };
}

function thumbnailRequestUrl(base: string, positionMs: number): string {
  const separator = base.includes("?") ? "&" : "?";
  return `${base}${separator}positionMs=${String(positionMs)}`;
}

async function fetchThumbnail(url: string, signal: AbortSignal): Promise<Blob> {
  const response = await fetch(url, { signal });
  if (!response.ok)
    throw new Error(`seek thumbnail request failed: ${String(response.status)}`);
  return response.blob();
}

function remember(
  cache: Map<number, string>,
  positionMs: number,
  objectUrl: string,
): void {
  cache.set(positionMs, objectUrl);
  if (cache.size <= cacheLimit) return;
  const oldest = cache.entries().next().value as [number, string] | undefined;
  if (oldest === undefined) return;
  cache.delete(oldest[0]);
  URL.revokeObjectURL(oldest[1]);
}
