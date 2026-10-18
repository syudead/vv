import {
  type CSSProperties,
  type PointerEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
  type Video,
} from "../api/client";
import { unplayableText } from "../lib/format";
import {
  type SeekPosition,
  seekPosition,
  seekSpriteCell,
  type SpriteCell,
  spriteCellBackground,
} from "../lib/seekSprite";

//

export interface ScrubPreviewOptions {
  video: Video;
  /** Omit the band during selection mode. */
  selectionMode?: boolean;
  /** Pass suspendPreview to pause the loop on band entry. */
  onSuspend?: () => void;
  /** Pass resumePreview when leaving the band into the card. */
  onResume?: () => void;
}

export interface ScrubFrameData {
  url: string;
  sprite: SeekThumbnailSprite;
  cell: SpriteCell;
}

export interface ScrubBandHandlers {
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: (event: PointerEvent<HTMLElement>) => void;
}

export interface ScrubPreview {
  enabled: boolean;
  position: SeekPosition | null;
  /** Null until both metadata and sheet are available. */
  frame: ScrubFrameData | null;
  bandHandlers: ScrubBandHandlers;
  /** Abort in-flight requests when the pointer leaves the card. */
  leaveCard: () => void;
  /** End scrubbing without resuming the loop, for resource release. */
  reset: () => void;
  /** Release cached sheets when the card leaves the viewport. */
  cardRef: (element: Element | null) => void;
}

/**
 * Cache metadata and sheet object URLs across band visits; abort only on card exit and retry
 * failures only after reentry. Release resources on unmount or viewport exit, also clearing the
 * last position on viewport exit.
 */
export function useScrubPreview({
  video,
  selectionMode = false,
  onSuspend,
  onResume,
}: ScrubPreviewOptions): ScrubPreview {
  const spriteUrl = video.seekThumbnailUrl;
  const durationMs = video.durationMs ?? 0;
  const enabled =
    spriteUrl !== undefined &&
    durationMs > 0 &&
    unplayableText(video) === null &&
    !selectionMode;

  const [position, setPosition] = useState<SeekPosition | null>(null);
  const [sprite, setSprite] = useState<SeekThumbnailSprite | null>(null);
  const [sheets, setSheets] = useState<ReadonlyMap<number, string>>(() => new Map());

  // Latest values for async reads.
  const spriteRef = useRef<SeekThumbnailSprite | null>(null);
  const sheetsRef = useRef(new Map<number, string>());
  const positionRef = useRef<SeekPosition | null>(null);
  const spriteRequest = useRef<AbortController | null>(null);
  const pendingSheets = useRef(new Map<number, AbortController>());
  const unavailable = useRef(false);
  const inBand = useRef(false);
  const handlers = useRef({ onSuspend, onResume });
  handlers.current = { onSuspend, onResume };

  const abortRequests = useCallback(() => {
    spriteRequest.current?.abort();
    spriteRequest.current = null;
    for (const controller of pendingSheets.current.values()) controller.abort();
    pendingSheets.current.clear();
  }, []);

  const releaseAll = useCallback(() => {
    abortRequests();
    unavailable.current = false;
    if (spriteRef.current === null && sheetsRef.current.size === 0) return;
    for (const url of sheetsRef.current.values()) URL.revokeObjectURL(url);
    sheetsRef.current = new Map();
    spriteRef.current = null;
    setSheets(sheetsRef.current);
    setSprite(null);
  }, [abortRequests]);

  const ensure = useCallback(() => {
    if (unavailable.current || spriteUrl === undefined) return;
    const layout = spriteRef.current;
    if (layout === null) {
      if (spriteRequest.current !== null) return;
      const controller = new AbortController();
      spriteRequest.current = controller;
      fetchSeekThumbnailSprite(spriteUrl, controller.signal)
        .then((loaded) => {
          if (controller.signal.aborted) return;
          spriteRequest.current = null;
          spriteRef.current = loaded;
          setSprite(loaded);
          ensure();
        })
        .catch(() => {
          if (controller.signal.aborted) return;
          spriteRequest.current = null;
          unavailable.current = true;
        });
      return;
    }
    const target = positionRef.current;
    if (target === null) return;
    const sheet = seekSpriteCell(layout, target.positionMs).sheet;
    if (sheetsRef.current.has(sheet) || pendingSheets.current.has(sheet)) return;
    const url = layout.sheets[sheet];
    if (url === undefined) {
      unavailable.current = true;
      return;
    }
    const controller = new AbortController();
    pendingSheets.current.set(sheet, controller);
    fetchSeekThumbnailSheet(url, controller.signal)
      .then((blob) => {
        if (controller.signal.aborted) return;
        pendingSheets.current.delete(sheet);
        const next = new Map(sheetsRef.current);
        next.set(sheet, URL.createObjectURL(blob));
        sheetsRef.current = next;
        setSheets(next);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        pendingSheets.current.delete(sheet);
        unavailable.current = true;
      });
  }, [spriteUrl]);

  const move = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const next = seekPosition(
        event.clientX,
        event.currentTarget.getBoundingClientRect(),
        durationMs,
      );
      positionRef.current = next;
      setPosition(next);
      ensure();
    },
    [durationMs, ensure],
  );

  const onPointerEnter = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (!enabled || event.pointerType !== "mouse") return;
      if (!inBand.current) {
        inBand.current = true;
        handlers.current.onSuspend?.();
      }
      move(event);
    },
    [enabled, move],
  );

  const onPointerMove = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (!enabled || event.pointerType !== "mouse") return;
      if (!inBand.current) {
        onPointerEnter(event);
        return;
      }
      move(event);
    },
    [enabled, move, onPointerEnter],
  );

  const reset = useCallback(() => {
    inBand.current = false;
    setPosition(null);
  }, []);

  const onPointerLeave = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      if (event.pointerType !== "mouse" || !inBand.current) return;
      reset();
      handlers.current.onResume?.();
    },
    [reset],
  );

  const leaveCard = useCallback(() => {
    inBand.current = false;
    positionRef.current = null;
    setPosition(null);
    abortRequests();
    unavailable.current = false;
  }, [abortRequests]);

  // If the band disappears, clear position and abort; unlike band exit into the card, no preview destination remains.
  useEffect(() => {
    if (!enabled) leaveCard();
  }, [enabled, leaveCard]);

  // Release sheets on unmount or sprite identity changes.
  // Refetch a changed sprite URL immediately when a current or retained pointer position exists.
  useEffect(() => {
    if (positionRef.current !== null) ensure();
    return () => releaseAll();
  }, [ensure, releaseAll]);

  const observer = useRef<IntersectionObserver | null>(null);
  const cardRef = useCallback(
    (element: Element | null) => {
      observer.current?.disconnect();
      observer.current = null;
      if (element === null || typeof IntersectionObserver === "undefined") return;
      observer.current = new IntersectionObserver(
        (entries) => {
          if (!entries.some((entry) => !entry.isIntersecting)) return;
          // Clear the retained position to prevent an offscreen URL change from refetching and retaining sheets.
          positionRef.current = null;
          releaseAll();
        },
        { rootMargin: "100%" },
      );
      observer.current.observe(element);
    },
    [releaseAll],
  );

  let frame: ScrubFrameData | null = null;
  if (enabled && position !== null && sprite !== null) {
    const cell = seekSpriteCell(sprite, position.positionMs);
    const url = sheets.get(cell.sheet);
    if (url !== undefined) frame = { url, sprite, cell };
  }

  return {
    enabled,
    position: enabled ? position : null,
    frame,
    bandHandlers: { onPointerEnter, onPointerMove, onPointerLeave },
    leaveCard,
    reset,
    cardRef,
  };
}

export function ScrubFrame({ frame }: { frame: ScrubFrameData | null }) {
  if (frame === null) return null;
  const { url, sprite, cell } = frame;
  const background = spriteCellBackground(cell, sprite);
  const style: CSSProperties = {
    aspectRatio: `${String(sprite.frameWidth)} / ${String(sprite.frameHeight)}`,
    width: `min(100cqw, calc(100cqh * ${String(sprite.frameWidth)} / ${String(sprite.frameHeight)}))`,
    maxWidth: "100%",
    maxHeight: "100%",
    backgroundImage: `url("${url}")`,
    backgroundRepeat: "no-repeat",
    backgroundSize: background.backgroundSize,
    backgroundPosition: background.backgroundPosition,
  };
  return (
    <div
      aria-hidden="true"
      data-scrub-frame=""
      className="pointer-events-none absolute inset-0 flex items-center justify-center @container-size"
    >
      <div data-scrub-frame-image="" style={style} />
    </div>
  );
}

/** Handle pointer movement only; clicks must reach the underlying link. */
export function ScrubBand({ scrub }: { scrub: ScrubPreview }) {
  if (!scrub.enabled) return null;
  return (
    <div
      aria-hidden="true"
      data-scrub-band=""
      className="absolute inset-x-0 bottom-0 h-1/5"
      {...scrub.bandHandlers}
    />
  );
}
