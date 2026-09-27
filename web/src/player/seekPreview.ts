import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../api/client";
import { formatDuration } from "../lib/format";

/** 取得に失敗した配置情報・シートを始め直すまでの待ち時間。 */
const unavailableRetryMs = 5000;

interface PreviewOptions {
  durationMs: number;
  /** Video.seekThumbnailUrl。スプライトの配置情報を返す。 */
  thumbnailUrl: string;
  fetchSprite?: (url: string, signal: AbortSignal) => Promise<SeekThumbnailSprite>;
  fetchSheet?: (url: string, signal: AbortSignal) => Promise<Blob>;
}

export interface PreviewTarget {
  positionMs: number;
  leftPx: number;
}

/** SpriteCell は位置を受け持つコマと、それが載るシートの中の升目である。 */
export interface SpriteCell {
  frame: number;
  sheet: number;
  column: number;
  row: number;
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
  const halfWidth = Math.min(previewWidth / 2, rect.width / 2);
  return {
    positionMs,
    leftPx: Math.min(Math.max(localX, halfWidth), rect.width - halfWidth),
  };
}

/**
 * seekSpriteCell は元動画の論理時刻 positionMs（ミリ秒）のコマを、配置情報の間隔で
 * 決める（specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md §2）。間隔を
 * 固定値としては持たない。
 */
export function seekSpriteCell(
  sprite: Pick<SeekThumbnailSprite, "intervalMs" | "frameCount" | "columns" | "rows">,
  positionMs: number,
): SpriteCell {
  const frame = Math.min(
    Math.max(0, Math.floor(positionMs / sprite.intervalMs)),
    sprite.frameCount - 1,
  );
  const perSheet = sprite.columns * sprite.rows;
  const index = frame % perSheet;
  return {
    frame,
    sheet: Math.floor(frame / perSheet),
    column: index % sprite.columns,
    row: Math.floor(index / sprite.columns),
  };
}

type PreviewState = "hidden" | "loading" | "ready" | "unavailable";

/**
 * attachSeekPreview はシークバーにプレビューを取り付ける
 * （specs/021-seek-thumbnail-sprite/research.md R-5）。
 *
 * 最初に出すときに配置情報を取得し、必要になったシートを 1 回だけ取得して object URL
 * で持つ。持っているシートは取り付けの間ずっと保持し、取り外しで解放する。取得は
 * シートごとに高々 1 つで、進行中の取得を中断するのは取り外しのときだけである。
 */
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
  const time = document.createElement("span");
  time.className = "vv-seek-preview-time";
  frame.append(image, time);
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

  // 表示中の位置のコマを出す。シートを持っていなければ取得を始め、進行中なら
  // その完了を待つ。
  const render = () => {
    if (!visible) return;
    if (sprite === null) {
      activeSheet = null;
      clearImage();
      if (spriteRequest !== null) {
        setState("loading");
      } else if (Date.now() < spriteRetryAt) {
        setState("unavailable");
      } else {
        setState("loading");
        loadSprite();
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

  // 枠を 1 コマの箱とし、シートを横 columns 倍・縦 rows 倍で敷いて、列と行の分だけ
  // ずらす。背景の位置の割合は（箱 − シート）の大きさに対する割合なので、
  // column / (columns − 1) で箱の整数倍のずれになる。
  const showCell = (url: string, cell: SpriteCell, layout: SeekThumbnailSprite) => {
    image.style.backgroundImage = `url("${url}")`;
    image.style.backgroundSize = `${String(layout.columns * 100)}% ${String(layout.rows * 100)}%`;
    image.style.backgroundPosition = `${String(offsetPercent(cell.column, layout.columns))}% ${String(offsetPercent(cell.row, layout.rows))}%`;
    setState("ready");
  };

  const loadSprite = () => {
    const controller = new AbortController();
    spriteRequest = controller;
    fetchSprite(options.thumbnailUrl, controller.signal)
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
        // 表示中でないシートの取得が終わっても、表示は切り替えない。
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

function offsetPercent(index: number, count: number): number {
  return count > 1 ? (index / (count - 1)) * 100 : 0;
}
