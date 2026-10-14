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

// カードと行を持ち上げて、ほかのカードの上か間に落とす共通の層である
// （specs/050-drag-organize/ui-design.md「Drag layer」、research.md R-7）。所有者の格子とリストを
// 包み、何を持ち上げたか、ポインタが何の上か、落としたら何になるかを決める。何を送るかは
// 画面が決める。ゲストの画面はこの層で包まない。
//
// - マウスは 8 px 動いたら、タッチは 200 ms 押したら持ち上がる。それより前に離せばクリック、
//   動かせばスクロールのまま。カードの中の操作（チェック、角の印、タグ、`×`）からは
//   持ち上がらない。リストのハンドルは 044 のまま 4 px で持ち上がる。
// - カードの隙間（`gap-3`）では、いちばん近いカードを落とす先にする。間の棒は隙間の中央に
//   立つので、その上で離しても落とせる。
// - 持ち上げたカードはその場で半透明になり、写しがポインタに付いてくる。Esc や何も無い所で
//   離すと何も送らない。
// - リストのハンドルは 044 の持ち上げ方（行が縦に動き、ほかの行が避ける）とキーボード
//   （Space、矢印、Space）を保ち、1 項目の並べ替えとして落とす。
// - 道筋の帯の段（CrumbDropTarget）も落とす先になる。段は帯が貼り付いて動くので、当たり判定の
//   たびにその場の箱で測る。

/** CardSign はカードに出す印である（上、間、拒否）。 */
export type CardSign = DropSign<UiText>;

/** CardDrop は受け入れた落とし方である。拒否と何もなしは画面に渡らない。 */
export interface CardDrop {
  /** 持ち上げたカード（画面の順）。 */
  lifted: readonly string[];
  sign: Exclude<CardSign, { kind: "refused" }>;
}

/** Mode は持ち上げ方である。cards はカードの層、handle はリストのハンドル。 */
type Mode = "cards" | "handle";

interface DragState {
  key: string;
  lifted: readonly string[];
  liftedSet: ReadonlySet<string>;
  mode: Mode;
  sign: CardSign | null;
  axis: Axis;
}

/** Probe はポインタの下のカードとその箱である（当たり判定のたびに覚え直す）。 */
interface Probe {
  key: string;
  pointer: { x: number; y: number };
  rect: ClientRect;
  rects: readonly ClientRect[];
  /** ポインタの下が道筋の段なら、その段が拒否か。カードなら省く。 */
  crumb?: { refused: boolean };
}

/** crumbPrefix は道筋の段の鍵の頭である。カードの鍵と混ざらない。 */
const crumbPrefix = "crumb:";

/** crumbKey は道筋の `index` 番目（0 始まり）の段の鍵である。 */
export function crumbKey(index: number | "ellipsis"): string {
  return `${crumbPrefix}${String(index)}`;
}

/** crumbIndexOf は段の鍵から段の番号を返す。段の鍵でないか、畳んだ「…」なら null。 */
export function crumbIndexOf(key: string): number | null {
  if (!key.startsWith(crumbPrefix)) return null;
  const index = Number(key.slice(crumbPrefix.length));
  return Number.isInteger(index) ? index : null;
}

/** isCrumb は落とす先が道筋の段かを返す。 */
function isCrumb(id: UniqueIdentifier): boolean {
  return String(id).startsWith(crumbPrefix);
}

/** CrumbData は道筋の段の落とす先に添える値である。 */
interface CrumbData {
  refused: boolean;
}

/**
 * crumbAt はポインタの下の道筋の段の Probe を返す。段の箱は今の場所で測る（帯は上に
 * 貼り付くので、持ち上げたときに測った箱とスクロールの量からは決まらない）。
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

/**
 * HeldDrop は落とした後も画面が保つ印である。落とした先のメニューが開いている間、持ち上げた
 * カードは半透明のまま、落とした先は輪と札のまま残る（ui-design.md「Drop menus」）。
 */
export interface HeldDrop {
  lifted: readonly string[];
  sign: CardSign;
}

/** CardAttachment はカード `key` に重ねて描くもの（落とした先のメニュー、名前の欄の窓）である。 */
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

/** controlSelector はカードの中で、持ち上げずにそれ自身の操作をする要素である。 */
const controlSelector =
  'button, input, label, select, textarea, [role="checkbox"], [role="menuitem"], [data-preview-checkbox]';

/** handleAttribute はリストのハンドルの印である。そこから持ち上げると handle の持ち上げ方になる。 */
const handleAttribute = "data-drag-handle";

/**
 * startsOnControl は押した場所が、listeners を付けた要素 `currentTarget` の中の操作かを返す。
 * ハンドルのように、listeners を付けた要素そのものが操作なら持ち上げる。
 */
function startsOnControl(target: EventTarget | null, currentTarget: EventTarget | null) {
  if (!(target instanceof Element) || !(currentTarget instanceof Element)) return false;
  // カードに重ねた窓やメニュー（ポータル）の中の出来事も React ではカードへ伝わる。カードの
  // 中に無い場所からは持ち上げない。
  if (!currentTarget.contains(target)) return true;
  const control = target.closest(controlSelector);
  return control !== null && control !== currentTarget && currentTarget.contains(control);
}

/** onHandle は押した場所がリストのハンドルかを返す。 */
function onHandle(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(`[${handleAttribute}]`) !== null;
}

/** cardDistance はカードと行が持ち上がるまでの距離（px）である。 */
const cardDistance = 8;

/** handleDistance はリストのハンドルが持ち上がるまでの距離（px）である（044 のまま）。 */
const handleDistance = 4;

/**
 * CardPointerSensor はマウスとペンで持ち上げる。カードと行は 8 px、ハンドルは 044 のまま
 * 4 px 動いたら持ち上がる。dnd-kit は同じ出来事の activator を 1 つしか持てないので、
 * 1 つのセンサーが押した場所で距離を決める。タッチは CardTouchSensor に任せる。
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

/** CardTouchSensor は 200 ms の長押しで持ち上げる。 */
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

/** useCardSensors はカードの持ち上げ方である（R-7）。キーボードはハンドルだけが使う。 */
function useCardSensors() {
  return useSensors(
    useSensor(CardPointerSensor),
    useSensor(CardTouchSensor, { activationConstraint: { delay: 200, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
}

/** noShift はカードの層の並べ方である。落とすまでどのカードも動かさない。 */
const noShift: SortingStrategy = () => null;

/** edgeScroll は端から 48 px で窓を送る autoScroll の設定である（ui-design.md「Lifting」）。 */
function edgeScroll() {
  const height = typeof window === "undefined" ? 0 : window.innerHeight;
  const y = height > 0 ? Math.min(0.5, 48 / height) : 0.1;
  return { threshold: { x: 0, y } };
}

/** eventPoint は持ち上げを始めた出来事の画面上の点である。 */
function eventPoint(event: Event): { x: number; y: number } | null {
  if (typeof TouchEvent !== "undefined" && event instanceof TouchEvent) {
    const touch = event.touches[0] ?? event.changedTouches[0];
    return touch === undefined ? null : { x: touch.clientX, y: touch.clientY };
  }
  return event instanceof MouseEvent ? { x: event.clientX, y: event.clientY } : null;
}

/** grabInset はリストの写しの左端からつかんだ点までの距離（px）である。 */
const grabInset = 16;

/**
 * besidePointer はリストの写しを、行の幅の箱の左端ではなく、つかんだ点のすぐ左に置く。
 * 写しは行のサムネイルの幅しかないので、箱の左端のままではポインタから離れて見える。
 */
const besidePointer: Modifier = ({ activatorEvent, activeNodeRect, transform }) => {
  const point = activatorEvent === null ? null : eventPoint(activatorEvent);
  if (point === null || activeNodeRect === null) return transform;
  return { ...transform, x: transform.x + point.x - activeNodeRect.left - grabInset };
};

/** reducedMotion は動きを減らす設定かを返す。 */
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

/** modeOf は持ち上げを始めた出来事から持ち上げ方を決める。 */
function modeOf(event: Event): Mode {
  return event instanceof KeyboardEvent || onHandle(event.target) ? "handle" : "cards";
}

/**
 * gapReach は隙間でいちばん近いカードを落とす先にする距離（px）である。格子の `gap-3`
 * （12 px）の中のどこからでも、どちらかのカードが 6 px 以内にある。
 */
const gapReach = 12;

/**
 * probeAt はポインタの下か、隙間ならいちばん近い持ち上げていないカードの Probe を返す。
 * 持ち上げたカードの隣の隙間でも、その向こうのカードの間として落とせる。
 */
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
  /** 持ち上げられるカードの鍵（画面の順）。 */
  order: readonly string[];
  /** 押したカードから持ち上げるカード（画面の順）を決める。 */
  liftedFor: (key: string) => readonly string[];
  /** カードの間に落として並べ替えられる画面か。並べ替えない画面ではカード全体が「上」になる。 */
  reorder: boolean;
  /**
   * カード `target` の上に `lifted` を落とせるか。省くか null を返すと上に落とす先ではなく、
   * 並べ替えられる画面ではカードの半分ずつが間になる。
   */
  onto?: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  /** 読み上げに使うカードの名前。 */
  nameOf: (key: string) => string;
  /** 持ち上げたカードの写しの中身（CardDragFace）。 */
  preview: (key: string) => ReactNode;
  /** 写しをリストの行の幅（w-list-thumb）にする。 */
  list?: boolean;
  onDrop: (drop: CardDrop) => void;
  /** 持ち上がったときに呼ぶ（ホバープレビューを止める）。 */
  onLift?: () => void;
  /** リストのハンドルのキーボードの案内と、ハンドルで持ち上げたときの読み上げ。 */
  handle?: { announcements: Announcements };
  /** dnd-kit の操作の案内（読み上げ用）。ハンドルを持つ画面はそのキーボードの案内を渡す。 */
  instructions: UiText;
  /** 落とした後も保つ印（メニューを開いている間）。持ち上げている間は持ち上げの印が先。 */
  held?: HeldDrop | null;
  /** カード 1 枚に重ねて描くもの。 */
  attached?: CardAttachment | null;
  children: ReactNode;
}

/** signAt はポインタの下のカード `probe` に出す印と、その並びの向きを返す。 */
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

/** insertPosition は間の印の位置（1 始まり）である。 */
function insertPosition(order: readonly string[], key: string, side: "before" | "after") {
  return order.indexOf(key) + (side === "after" ? 2 : 1);
}

/** useAnnouncements はカードの層の読み上げである（ui-design.md「Words」の Announcement）。 */
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
 * useEscapeMarked は持ち上げている間の Esc に preventDefault を付ける。持ち上げを取り消すだけに
 * し、選択の解除（useEscapeClearsSelection）には渡さない。dnd-kit は文書の bubble で Esc を
 * 受けるので、capture で印を付けても取り消しは働く。
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

/**
 * CardDragLayer は所有者の格子かリストを包むドラッグの層である。中のカードは
 * DraggableCard で、リストの行は useCardDrag で層に加わる。
 */
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
  // 読み上げは持ち上げが終わった後にも読むので、最後の持ち上げを別に残す。
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
      {/* 写しは body に置き、サイドバーや上のバーより前に出す。 */}
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

/** CardDragBinding はカードか行を層に加える値である（useCardDrag の戻り値）。 */
export interface CardDragBinding {
  ref: (node: HTMLElement | null) => void;
  /** カードか行に広げるポインタとタッチの listeners。 */
  listeners: Record<string, unknown>;
  /** リストのハンドルに渡す値（reorder.tsx の ReorderHandle）。 */
  handle: { ref: (node: HTMLElement | null) => void; props: Record<string, unknown> };
  /** ハンドルで持ち上げている間の行の位置。 */
  style: CSSProperties | undefined;
  /** このカードが持ち上げたものに入っている間 true（半透明にする）。 */
  lifted: boolean;
  /** ハンドルで持ち上げているのがこの行のとき true（行を浮かせる）。 */
  sorting: boolean;
  /** このカードに出す印。 */
  sign: CardSign | null;
  axis: Axis;
  /** このカードに重ねて描くもの（CardAttachment）。 */
  attachment: ReactNode;
}

/** useCardDrag はカード `key` を層に加える。CardDragLayer の中で使う。 */
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

/** signOf はカード `key` に出す印である。持ち上げている間はその印、無ければ保った印。 */
function signOf(key: string, drag: DragState | null, held: HeldDrop | null) {
  const sign = drag === null ? (held?.sign ?? null) : drag.sign;
  return sign !== null && sign.key === key ? sign : null;
}

/**
 * CardDropMark はカードか行に出す印である（ui-design.md「Indicators」）。上はカードの箱の
 * `primary` の輪とサムネイルの札、間は隙間の中央の細い棒、拒否はカードに何も出さない。
 * 置く先は `relative` の箱（カードは DraggableCard、行は TableRow）。
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

/**
 * dragRowProps はリストの行（TableRow）を層に加える props である。行のクラス `className` に、
 * 持ち上げたら半透明にするクラスを足す。`drag` が無ければ行はそのまま。
 */
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

/** RowDropMarks はリストの行の印と、行に重ねたもの（メニューの位置の目印）である。 */
export function RowDropMarks({ drag }: { drag: CardDragBinding | undefined }) {
  if (drag === undefined) return null;
  return (
    <>
      <CardDropMark sign={drag.sign} axis="y" list />
      {drag.attachment}
    </>
  );
}

/**
 * DraggableCard は格子のカード 1 枚を層に加える。カード（`article`）を包み、持ち上げたら
 * 半透明にし、印を重ねる。包みは格子の 1 枡で、中のカードを枡の高さまで伸ばす。
 */
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

/**
 * CardDragFace は写しの中身である。カードのサムネイル（動画は VideoThumbnail、グループは
 * フォルダの絵柄）と、その下の 1 行の題名。
 */
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

/**
 * CardDragPreview はポインタに付いてくる写しである（ui-design.md「Lifting」）。カードの幅
 * （リストでは w-list-thumb）で、2 枚以上なら右上に件数の Badge、拒否の上では `Ban` の
 * Badge を出す。
 */
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
 * CrumbDropTarget は道筋の帯の段 1 つを落とす先にする（ui-design.md「Indicators」の Crumb）。
 * 落とせる段の上では段の文字に `primary-soft` の地を敷き、帯のほかの所は変えない。`refused`
 * の段（開いているグループ、畳んだ「…」）は印を出さず、写しに拒否の Badge を出す。
 * CardDragLayer の中で使う。
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
