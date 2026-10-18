import {
  type Announcements,
  type ClientRect,
  closestCenter,
  type Collision,
  type CollisionDetection,
  DndContext,
  type DragEndEvent,
  DragOverlay,
  type DragStartEvent,
  type DroppableContainer,
  KeyboardSensor,
  type Modifier,
  PointerSensor,
  type PointerSensorOptions,
  type PointerSensorProps,
  pointerWithin,
  TouchSensor,
  type TouchSensorOptions,
  type UniqueIdentifier,
  useDroppable,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  type SortingStrategy,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { Ban } from "lucide-react";
import {
  createContext,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type TouchEvent as ReactTouchEvent,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Badge } from "../ui/shadcn/badge";
import {
  type Axis,
  axisOf,
  type DropSign,
  nearestWithin,
  normalizeBetween,
  type OntoAnswer,
  signFor,
  zoneOf,
} from "./cardDragRules";
import { verticalOnly } from "./reorder";

// Owner-only drag layer; screens own server mutations. Cards support pointer drops, while list handles also support keyboard reordering.
//
// Mouse activation requires 8px, touch requires a 200ms hold, and handles require 4px; nested controls never start a drag.
// Resolve gaps to the nearest card so the centered insertion marker remains droppable.
// Measure sticky breadcrumb targets on every hit test because scroll deltas cannot predict their position.

export type CardSign = DropSign<UiText>;

export interface CardDrop {
  /** Preserve display order. */
  lifted: readonly string[];
  sign: Exclude<CardSign, { kind: "refused" }>;
}

type Mode = "cards" | "handle";

interface DragState {
  key: string;
  lifted: readonly string[];
  liftedSet: ReadonlySet<string>;
  mode: Mode;
  sign: CardSign | null;
  axis: Axis;
}

interface Probe {
  key: string;
  pointer: { x: number; y: number };
  rect: ClientRect;
  rects: readonly ClientRect[];
  crumb?: { refused: boolean };
}

/** Keep breadcrumb keys disjoint from card keys. */
const crumbPrefix = "crumb:";

export function crumbKey(index: number | "ellipsis"): string {
  return `${crumbPrefix}${String(index)}`;
}

/** Return null for non-crumb keys and the collapsed ellipsis. */
export function crumbIndexOf(key: string): number | null {
  if (!key.startsWith(crumbPrefix)) return null;
  const index = Number(key.slice(crumbPrefix.length));
  return Number.isInteger(index) ? index : null;
}

function isCrumb(id: UniqueIdentifier): boolean {
  return String(id).startsWith(crumbPrefix);
}

interface CrumbData {
  refused: boolean;
}

/**
 * Measure current breadcrumb bounds; sticky positioning invalidates lift-time bounds plus scroll
 * offsets.
 */
function crumbAt(
  pointer: { x: number; y: number } | null,
  containers: readonly DroppableContainer[],
): Probe | null {
  if (pointer === null) return null;
  for (const container of containers) {
    const node = container.node.current;
    if (!isCrumb(container.id) || node === null) continue;
    const box = node.getBoundingClientRect();
    const within =
      box.width > 0 &&
      pointer.x >= box.left &&
      pointer.x <= box.right &&
      pointer.y >= box.top &&
      pointer.y <= box.bottom;
    if (!within) continue;
    const data = container.data.current as CrumbData | undefined;
    const rect = {
      top: box.top,
      left: box.left,
      width: box.width,
      height: box.height,
      bottom: box.bottom,
      right: box.right,
    };
    return {
      key: String(container.id),
      pointer,
      rect,
      rects: [],
      crumb: { refused: data?.refused ?? true },
    };
  }
  return null;
}

/** Preserve lifted opacity and target marks while a post-drop menu is open. */
export interface HeldDrop {
  lifted: readonly string[];
  sign: CardSign;
}

export interface CardAttachment {
  key: string;
  node: ReactNode;
}

interface LayerValue {
  drag: DragState | null;
  held: HeldDrop | null;
  attached: CardAttachment | null;
}

const LayerContext = createContext<LayerValue>({
  drag: null,
  held: null,
  attached: null,
});

const controlSelector =
  'button, input, label, select, textarea, [role="checkbox"], [role="menuitem"], [data-preview-checkbox]';

const handleAttribute = "data-drag-handle";

/** Nested controls do not drag, but a control carrying listeners itself, such as a handle, may. */
function startsOnControl(target: EventTarget | null, currentTarget: EventTarget | null) {
  if (!(target instanceof Element) || !(currentTarget instanceof Element)) return false;
  // Portal events bubble through React; reject drag starts outside the card's DOM subtree.
  if (!currentTarget.contains(target)) return true;
  const control = target.closest(controlSelector);
  return control !== null && control !== currentTarget && currentTarget.contains(control);
}

function onHandle(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(`[${handleAttribute}]`) !== null;
}

const cardDistance = 8;

const handleDistance = 4;

/**
 * dnd-kit permits only one activator per event, so choose 8px card or 4px handle activation within
 * one sensor; touch uses its own sensor.
 */
class CardPointerSensor extends PointerSensor {
  static activators = [
    {
      eventName: "onPointerDown" as const,
      handler: (
        { nativeEvent, target, currentTarget }: ReactPointerEvent,
        { onActivation }: PointerSensorOptions,
      ) => {
        if (!nativeEvent.isPrimary || nativeEvent.button !== 0) return false;
        if (nativeEvent.pointerType === "touch") return false;
        if (startsOnControl(target, currentTarget)) return false;
        onActivation?.({ event: nativeEvent });
        return true;
      },
    },
  ];

  constructor(props: PointerSensorProps) {
    const distance = onHandle(props.event.target) ? handleDistance : cardDistance;
    super({
      ...props,
      options: { ...props.options, activationConstraint: { distance } },
    });
  }
}

class CardTouchSensor extends TouchSensor {
  static activators = [
    {
      eventName: "onTouchStart" as const,
      handler: (
        { nativeEvent, target, currentTarget }: ReactTouchEvent,
        { onActivation }: TouchSensorOptions,
      ) => {
        if (nativeEvent.touches.length > 1) return false;
        if (startsOnControl(target, currentTarget)) return false;
        onActivation?.({ event: nativeEvent });
        return true;
      },
    },
  ];
}

/** Keyboard lifting is restricted to handles. */
function useCardSensors() {
  return useSensors(
    useSensor(CardPointerSensor),
    useSensor(CardTouchSensor, { activationConstraint: { delay: 200, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
}

/** Cards remain stationary until drop. */
const noShift: SortingStrategy = () => null;

function edgeScroll() {
  const height = typeof window === "undefined" ? 0 : window.innerHeight;
  const y = height > 0 ? Math.min(0.5, 48 / height) : 0.1;
  return { threshold: { x: 0, y } };
}

function eventPoint(event: Event): { x: number; y: number } | null {
  if (typeof TouchEvent !== "undefined" && event instanceof TouchEvent) {
    const touch = event.touches[0] ?? event.changedTouches[0];
    return touch === undefined ? null : { x: touch.clientX, y: touch.clientY };
  }
  return event instanceof MouseEvent ? { x: event.clientX, y: event.clientY } : null;
}

const grabInset = 16;

/**
 * Place the thumbnail-sized row preview near the grab point, not the distant left edge of the full
 * row.
 */
const besidePointer: Modifier = ({ activatorEvent, activeNodeRect, transform }) => {
  const point = activatorEvent === null ? null : eventPoint(activatorEvent);
  if (point === null || activeNodeRect === null) return transform;
  return { ...transform, x: transform.x + point.x - activeNodeRect.left - grabInset };
};

function reducedMotion(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function sameSign(a: CardSign | null, b: CardSign | null): boolean {
  if (a === null || b === null) return a === b;
  if (a.kind !== b.kind || a.key !== b.key) return false;
  if (a.kind === "between" && b.kind === "between") return a.side === b.side;
  if (a.kind === "onto" && b.kind === "onto") return a.label === b.label;
  return true;
}

function modeOf(event: Event): Mode {
  return event instanceof KeyboardEvent || onHandle(event.target) ? "handle" : "cards";
}

/** Half of gap-3's 12px gap keeps a card within reach everywhere in the gap. */
const gapReach = 12;

/** Exclude lifted cards so the adjacent gap can target the next stationary card. */
function probeAt(
  pointer: { x: number; y: number } | null,
  collisions: Collision[],
  droppableRects: ReadonlyMap<UniqueIdentifier, ClientRect>,
  lifted: ReadonlySet<string>,
): { probe: Probe | null; collisions: Collision[] } {
  if (pointer === null) return { probe: null, collisions };
  let key = collisions[0]?.id ?? null;
  if (key === null) {
    const others = Array.from(droppableRects.entries()).filter(
      ([id]) => !lifted.has(String(id)),
    );
    key = nearestWithin(pointer, others, gapReach);
    if (key !== null) collisions = [{ id: key, data: { value: 0 } }];
  }
  const rect = key === null ? undefined : droppableRects.get(key);
  if (key === null || rect === undefined) return { probe: null, collisions };
  const probe = {
    key: String(key),
    pointer,
    rect,
    rects: Array.from(droppableRects.values()),
  };
  return { probe, collisions };
}

export interface CardDragLayerProps {
  order: readonly string[];
  liftedFor: (key: string) => readonly string[];
  /** Without reordering, the whole card is an onto target. */
  reorder: boolean;
  /** Null or absent means no onto target; reorderable cards split into before/after halves. */
  onto?: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  nameOf: (key: string) => string;
  preview: (key: string) => ReactNode;
  list?: boolean;
  onDrop: (drop: CardDrop) => void;
  onLift?: () => void;
  handle?: { announcements: Announcements };
  instructions: UiText;
  /** Active drag marks take precedence over retained post-drop marks. */
  held?: HeldDrop | null;
  attached?: CardAttachment | null;
  children: ReactNode;
}

function signAt(
  probe: Probe | null,
  state: DragState,
  props: CardDragLayerProps,
): { sign: CardSign | null; axis: Axis } {
  if (probe === null) return { sign: null, axis: state.axis };
  if (probe.crumb !== undefined) {
    const kind = probe.crumb.refused ? "refused" : "crumb";
    return { sign: { kind, key: probe.key }, axis: state.axis };
  }
  const axis = axisOf(probe.rects);
  const onto = props.onto?.(probe.key, state.lifted) ?? null;
  const split = props.reorder && onto === null ? "halves" : "quarters";
  const zone =
    axis === "x"
      ? zoneOf(probe.pointer.x, probe.rect.left, probe.rect.width, split)
      : zoneOf(probe.pointer.y, probe.rect.top, probe.rect.height, split);
  const sign = signFor({
    target: probe.key,
    lifted: state.liftedSet,
    reorder: props.reorder,
    onto,
    zone,
  });
  if (sign?.kind !== "between") return { sign, axis };
  return {
    sign: { kind: "between", ...normalizeBetween(props.order, sign.key, sign.side) },
    axis,
  };
}

/** One-based insertion position. */
function insertPosition(order: readonly string[], key: string, side: "before" | "after") {
  return order.indexOf(key) + (side === "after" ? 2 : 1);
}

function useAnnouncements(
  latest: { current: CardDragLayerProps },
  announced: { current: DragState | null },
  probe: { current: Probe | null },
): Announcements {
  return useMemo(() => {
    const handleTexts = () => latest.current.handle?.announcements;
    const spoken = (sign: CardSign | null, ended: boolean): string | undefined => {
      const props = latest.current;
      const count = props.order.length;
      if (sign === null || sign.kind === "refused") {
        if (ended) return t.organize.announce.cancelled;
        return sign === null
          ? undefined
          : t.organize.announce.over(props.nameOf(sign.key), t.organize.drop.refused);
      }
      if (sign.kind === "onto" || sign.kind === "crumb") {
        const name = props.nameOf(sign.key);
        const label = sign.kind === "onto" ? sign.label : t.organize.drop.putIn;
        return ended
          ? t.organize.announce.dropped(name)
          : t.organize.announce.over(name, label);
      }
      const position = insertPosition(props.order, sign.key, sign.side);
      return ended
        ? t.organize.announce.droppedAt(position, count)
        : t.organize.announce.between(position, count);
    };
    return {
      onDragStart: (event) => {
        const state = announced.current;
        if (state?.mode === "handle") return handleTexts()?.onDragStart(event);
        if (state === null) return undefined;
        return t.organize.announce.pickedUp(
          latest.current.nameOf(state.key),
          state.lifted.length,
        );
      },
      onDragOver: (event) => {
        const state = announced.current;
        if (state?.mode === "handle") return handleTexts()?.onDragOver?.(event);
        if (state === null) return undefined;
        return spoken(signAt(probe.current, state, latest.current).sign, false);
      },
      onDragEnd: (event) => {
        const state = announced.current;
        if (state?.mode === "handle") return handleTexts()?.onDragEnd(event);
        return spoken(state?.sign ?? null, true);
      },
      onDragCancel: (event) => {
        if (announced.current?.mode === "handle") {
          return handleTexts()?.onDragCancel(event);
        }
        return t.organize.announce.cancelled;
      },
    };
  }, [announced, latest, probe]);
}

/**
 * Mark Escape in capture so it cancels dragging without clearing selection; dnd-kit's bubble
 * handler still receives it.
 */
function useEscapeMarked(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") event.preventDefault();
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [active]);
}

export function CardDragLayer(props: CardDragLayerProps) {
  const {
    order,
    list = false,
    instructions,
    held = null,
    attached = null,
    children,
  } = props;
  const sensors = useCardSensors();
  const [drag, setDrag] = useState<DragState | null>(null);
  const current = useRef<DragState | null>(null);
  // Retain the last drag for announcements delivered after dragging ends.
  const announced = useRef<DragState | null>(null);
  const probe = useRef<Probe | null>(null);
  const latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  });

  const show = useCallback((next: DragState | null) => {
    current.current = next;
    if (next !== null) announced.current = next;
    setDrag(next);
  }, []);

  const detect = useCallback<CollisionDetection>((args) => {
    const cards = args.droppableContainers.filter((each) => !isCrumb(each.id));
    if (current.current?.mode === "handle") {
      return closestCenter({ ...args, droppableContainers: cards });
    }
    const crumb = crumbAt(args.pointerCoordinates, args.droppableContainers);
    if (crumb !== null) {
      probe.current = crumb;
      return [{ id: crumb.key, data: { value: 0 } }];
    }
    const cardRects = new Map(
      Array.from(args.droppableRects).filter(([id]) => !isCrumb(id)),
    );
    const found = probeAt(
      args.pointerCoordinates,
      pointerWithin({ ...args, droppableContainers: cards }),
      cardRects,
      current.current?.liftedSet ?? new Set(),
    );
    probe.current = found.probe;
    return found.collisions;
  }, []);

  const start = useCallback(
    (event: DragStartEvent) => {
      const key = String(event.active.id);
      const mode = modeOf(event.activatorEvent);
      const lifted = mode === "handle" ? [key] : latest.current.liftedFor(key);
      probe.current = null;
      show({ key, lifted, liftedSet: new Set(lifted), mode, sign: null, axis: "x" });
      latest.current.onLift?.();
    },
    [show],
  );

  const move = useCallback(() => {
    const state = current.current;
    if (state === null || state.mode === "handle") return;
    const next = signAt(probe.current, state, latest.current);
    if (sameSign(next.sign, state.sign) && next.axis === state.axis) return;
    show({ ...state, ...next });
  }, [show]);

  const end = useCallback(
    (event: DragEndEvent) => {
      const state = current.current;
      show(null);
      if (state === null) return;
      const { onDrop } = latest.current;
      if (state.mode === "handle") {
        const over = event.over === null ? null : String(event.over.id);
        if (over === null || over === state.key) return;
        const from = latest.current.order.indexOf(state.key);
        const to = latest.current.order.indexOf(over);
        const side = from < to ? "after" : "before";
        onDrop({ lifted: [state.key], sign: { kind: "between", key: over, side } });
        return;
      }
      const sign = state.sign;
      if (sign === null || sign.kind === "refused") return;
      onDrop({ lifted: state.lifted, sign });
    },
    [show],
  );

  const cancel = useCallback(() => show(null), [show]);
  const announcements = useAnnouncements(latest, announced, probe);
  useEscapeMarked(drag !== null);

  const value = useMemo(() => ({ drag, held, attached }), [attached, drag, held]);
  const handleMode = drag?.mode === "handle";
  const refused = drag?.sign?.kind === "refused";

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={detect}
      modifiers={handleMode ? [verticalOnly] : undefined}
      autoScroll={edgeScroll()}
      onDragStart={start}
      onDragMove={move}
      onDragOver={move}
      onDragEnd={end}
      onDragCancel={cancel}
      accessibility={{
        announcements,
        screenReaderInstructions: { draggable: instructions },
      }}
    >
      <SortableContext
        items={order as UniqueIdentifier[]}
        strategy={handleMode ? verticalListSortingStrategy : noShift}
      >
        <LayerContext value={value}>{children}</LayerContext>
      </SortableContext>
      {/* Portal the preview to body above the sidebar and top bar. */}
      {createPortal(
        <DragOverlay
          dropAnimation={reducedMotion() ? null : { duration: 150 }}
          modifiers={list ? [besidePointer] : undefined}
          className={refused ? "cursor-not-allowed" : "cursor-grabbing"}
        >
          {drag !== null && drag.mode === "cards" && (
            <CardDragPreview count={drag.lifted.length} refused={refused} list={list}>
              {props.preview(drag.key)}
            </CardDragPreview>
          )}
        </DragOverlay>,
        document.body,
      )}
    </DndContext>
  );
}

export interface CardDragBinding {
  ref: (node: HTMLElement | null) => void;
  listeners: Record<string, unknown>;
  handle: { ref: (node: HTMLElement | null) => void; props: Record<string, unknown> };
  style: CSSProperties | undefined;
  lifted: boolean;
  sorting: boolean;
  sign: CardSign | null;
  axis: Axis;
  attachment: ReactNode;
}

/** Must run within CardDragLayer. */
export function useCardDrag(key: string): CardDragBinding {
  const { drag, held, attached } = useContext(LayerContext);
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: key });
  const handleMode = drag?.mode === "handle";
  const cardListeners = useMemo(() => {
    const all: Record<string, unknown> = { ...listeners };
    delete all.onKeyDown;
    return all;
  }, [listeners]);
  return {
    ref: setNodeRef,
    listeners: cardListeners,
    handle: {
      ref: setActivatorNodeRef,
      props: { ...attributes, ...listeners, [handleAttribute]: "" },
    },
    style:
      !handleMode || transform === null
        ? undefined
        : { transform: `translate3d(0, ${String(transform.y)}px, 0)`, transition },
    lifted:
      drag === null
        ? held?.lifted.includes(key) === true
        : !handleMode && drag.liftedSet.has(key),
    sorting: handleMode && isDragging,
    sign: signOf(key, drag, held),
    axis: drag?.axis ?? "x",
    attachment: attached?.key === key ? attached.node : null,
  };
}

/** Prefer active drag marks over retained marks. */
function signOf(key: string, drag: DragState | null, held: HeldDrop | null) {
  const sign = drag === null ? (held?.sign ?? null) : drag.sign;
  return sign !== null && sign.key === key ? sign : null;
}

/**
 * Render in a relative card or row; insertion markers sit at gap centers, while refusal appears
 * only on the preview.
 */
export function CardDropMark({
  sign,
  axis,
  list = false,
}: {
  sign: CardSign | null;
  axis: Axis;
  list?: boolean;
}) {
  if (sign === null || sign.kind === "refused" || sign.kind === "crumb") return null;
  if (sign.kind === "onto") {
    return (
      <>
        <span
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 z-40 rounded-md ring-2 ring-primary"
        />
        {!list && (
          <span
            aria-hidden="true"
            className="pointer-events-none absolute inset-x-0 top-0 z-40 flex aspect-video items-center justify-center"
          >
            <span className="rounded-md bg-overlay px-2 py-1 text-xs font-medium text-foreground">
              {sign.label}
            </span>
          </span>
        )}
      </>
    );
  }
  const before = sign.side === "before";
  return (
    <span
      aria-hidden="true"
      data-drop-bar={sign.side}
      className={cn(
        "pointer-events-none absolute z-40 rounded-full bg-primary",
        list && "inset-x-0 h-0.5",
        list && (before ? "top-0" : "bottom-0"),
        !list && axis === "x" && "inset-y-0 w-0.5",
        !list &&
          axis === "x" &&
          (before ? "-left-1.5 -translate-x-1/2" : "-right-1.5 translate-x-1/2"),
        !list && axis === "y" && "inset-x-0 h-0.5",
        !list &&
          axis === "y" &&
          (before ? "-top-1.5 -translate-y-1/2" : "-bottom-1.5 translate-y-1/2"),
      )}
    />
  );
}

export function dragRowProps(drag: CardDragBinding | undefined, className: string) {
  return {
    ref: drag?.ref,
    ...drag?.listeners,
    className: cn(
      className,
      drag !== undefined && "relative motion-reduce:transition-none",
      drag?.lifted === true && "opacity-50",
    ),
  };
}

export function RowDropMarks({ drag }: { drag: CardDragBinding | undefined }) {
  if (drag === undefined) return null;
  return (
    <>
      <CardDropMark sign={drag.sign} axis="y" list />
      {drag.attachment}
    </>
  );
}

/** Stretch the inner article to the grid cell height. */
export function DraggableCard({ id, children }: { id: string; children: ReactNode }) {
  const drag = useCardDrag(id);
  return (
    <div
      ref={drag.ref}
      {...drag.listeners}
      data-drag-card=""
      className={cn(
        "relative grid min-w-0 rounded-md transition-opacity motion-reduce:transition-none",
        drag.lifted && "opacity-50",
      )}
    >
      {children}
      <CardDropMark sign={drag.sign} axis={drag.axis} />
      {drag.attachment}
    </div>
  );
}

/** Non-draggable cards still register as targets so onto can explicitly return refused. */
export function RefusingDropCard({ id, children }: { id: string; children: ReactNode }) {
  const { setNodeRef } = useDroppable({ id });
  return (
    <div ref={setNodeRef} className="relative grid min-w-0 rounded-md">
      {children}
    </div>
  );
}

export function CardDragFace({
  thumbnail,
  title,
}: {
  thumbnail: ReactNode;
  title: string;
}) {
  return (
    <>
      {thumbnail}
      <p className="truncate px-2 pt-2 pb-2 text-sm font-medium text-foreground">
        {title}
      </p>
    </>
  );
}

function CardDragPreview({
  count,
  refused,
  list,
  children,
}: {
  count: number;
  refused: boolean;
  list: boolean;
  children: ReactNode;
}) {
  return (
    <div
      data-drag-preview=""
      className={cn(
        "relative flex flex-col rounded-md bg-card shadow-elevated",
        list && "w-list-thumb",
      )}
    >
      {children}
      {refused ? (
        <Badge
          variant="destructive"
          aria-label={t.organize.drop.refused}
          className="absolute -top-2 -right-2"
        >
          <Ban aria-hidden="true" />
        </Badge>
      ) : (
        count > 1 && (
          <Badge aria-label={t.count.items(count)} className="absolute -top-2 -right-2">
            {formatNumber(count)}
          </Badge>
        )
      )}
    </div>
  );
}

/**
 * Use within CardDragLayer; refused crumbs affect only the preview badge, not the breadcrumb
 * appearance.
 */
export function CrumbDropTarget({
  id,
  refused,
  children,
}: {
  id: string;
  refused: boolean;
  children: ReactNode;
}) {
  const { drag } = useContext(LayerContext);
  const data = useMemo<CrumbData>(() => ({ refused }), [refused]);
  const { setNodeRef } = useDroppable({ id, data });
  const active = drag?.sign?.kind === "crumb" && drag.sign.key === id;
  return (
    <span
      ref={setNodeRef}
      data-crumb-drop={active ? "active" : ""}
      className={cn(
        "-mx-1 flex min-w-0 rounded-sm px-1",
        active && "bg-primary-soft text-primary",
      )}
    >
      {children}
    </span>
  );
}
