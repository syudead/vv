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

// 一覧のカードと関連動画のサムネイルの下端に置く、スクラブの帯である
// （specs/032-card-scrub-preview/research.md R-3〜R-5、ui-design.md「Band」「Frame」）。

export interface ScrubPreviewOptions {
  video: Video;
  /** 選択モードの間は帯を置かない。 */
  selectionMode?: boolean;
  /** 帯に入ったときに呼ぶ。ループ再生の suspendPreview を渡す。 */
  onSuspend?: () => void;
  /** 帯からカードの中へ出たときに呼ぶ。ループ再生の resumePreview を渡す。 */
  onResume?: () => void;
}

/** ScrubFrameData は帯の位置のコマを出すのに要るもの一式である。 */
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

/** ScrubPreview は useScrubPreview の結果である。 */
export interface ScrubPreview {
  /** 帯を置くか。 */
  enabled: boolean;
  /** 帯にいる間のポインタの位置。帯の外では null。 */
  position: SeekPosition | null;
  /** 帯の位置のコマ。配置情報とシートが揃うまでは null。 */
  frame: ScrubFrameData | null;
  /** ScrubBand に渡すポインタの受け口。 */
  bandHandlers: ScrubBandHandlers;
  /** カードからポインタが出たときに呼ぶ。進行中の取得を打ち切る。 */
  leaveCard: () => void;
  /** 帯にいる状態を、ループ再生の再開を呼ばずに終える（解放の契機に使う）。 */
  reset: () => void;
  /** カードの要素。viewport から外れたら持っているシートを解放する。 */
  cardRef: (element: Element | null) => void;
}

/**
 * useScrubPreview はカード 1 枚の帯の状態を持つ。
 *
 * - 帯に初めて入ったときに配置情報を、続いて今の位置のコマが載るシートを取得し、
 *   object URL で持つ。持っているものは帯を出入りしても取り直さない。
 * - カードを出たら進行中の取得を打ち切る。帯を出てカードの中にいる間は取得を続ける。
 * - 失敗したら、カードを出て入り直すまで取り直さない。
 * - カードが viewport から外れたときと unmount で、object URL と配置情報を捨てる。
 *   viewport から外れたときは最後の位置も捨てる。
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

  // 非同期の取得から読む最新の値。
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

  // 今の位置（帯を出た後は最後の位置）のコマに要るものを、まだ無ければ取得する。
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

  // 選択モードに入るなどで帯が無くなったら、帯にいる状態を終え、最後の位置も捨てて
  // 進行中の取得を打ち切る。帯を出てカードの中にいるときと違い、取得を続ける先が無い。
  useEffect(() => {
    if (!enabled) leaveCard();
  }, [enabled, leaveCard]);

  // unmount と、別のスプライトを指すようになったときに、持っているシートを捨てる。
  // 取り直しで URL が変わったときにまだ位置を持っていれば（帯の中、または帯を出て
  // カードの中）、ポインタが動くのを待たずに新しい URL から取り直す。
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
          // 最後の位置も捨てる。残すと、外れている間に URL が変わったときに取り直し、
          // 次に外れるまで解放されない。
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

/**
 * ScrubFrame は帯の位置のコマを、サムネイルの object-contain と同じ枠に出す層である。
 * コマが揃うまでは何も出さない。
 */
export function ScrubFrame({ frame }: { frame: ScrubFrameData | null }) {
  if (frame === null) return null;
  const { url, sprite, cell } = frame;
  const background = spriteCellBackground(cell, sprite);
  const style: CSSProperties = {
    aspectRatio: `${String(sprite.frameWidth)} / ${String(sprite.frameHeight)}`,
    // 面の幅と高さの両方に収まる大きさ（object-contain と同じ収め方）。
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

/**
 * ScrubBand はサムネイルの面の下端 5 分の 1 に置く透明な帯である。ポインタの出入りと
 * 移動を受け取るだけで、クリックは下のリンクに届く。
 */
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
