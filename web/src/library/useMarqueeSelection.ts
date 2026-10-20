import {
  type Dispatch,
  type RefObject,
  type SetStateAction,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import type { LibraryItem } from "../api/client";
import { itemKey } from "../api/libraryItems";
import { type Box, boxBetween, edgeSpeed, overlaps, type Point } from "./marquee";
import { emptySelection, type LibrarySelection, setItems } from "./selection";

/** Matches the card drag threshold, so a click on empty space never draws a box. */
const startDistance = 8;

const startsElsewhere =
  "[data-card-key], a, button, input, label, select, textarea, [role='checkbox'], [role='menuitem']";

interface Options {
  /** Presses inside it, outside cards and controls, start a box. */
  container: RefObject<HTMLElement | null>;
  /** The fixed element drawn as the box; hidden while no box is drawn. */
  box: RefObject<HTMLElement | null>;
  enabled: boolean;
  items: readonly LibraryItem[];
  selection: LibrarySelection;
  setSelection: Dispatch<SetStateAction<LibrarySelection>>;
}

/**
 * Mouse box selection over grid cards. Shift, Ctrl or ⌘ at the press adds to the selection;
 * Escape restores the selection from before the press.
 */
export function useMarqueeSelection(options: Options) {
  const { container, box, enabled } = options;
  const latest = useRef(options);
  useLayoutEffect(() => {
    latest.current = options;
  });

  useEffect(() => {
    const root = container.current;
    if (!enabled || root === null) return;
    const onDown = (event: PointerEvent) => {
      if (event.pointerType !== "mouse" || event.button !== 0 || !event.isPrimary) return;
      if (!(event.target instanceof Element) || event.target.closest(startsElsewhere))
        return;
      // Keep the browser from selecting text or dragging images under the box.
      event.preventDefault();
      trackBox(event, root, box, latest);
    };
    root.addEventListener("pointerdown", onDown);
    return () => root.removeEventListener("pointerdown", onDown);
  }, [box, container, enabled]);
}

function pagePoint(x: number, y: number): Point {
  return { x: x + window.scrollX, y: y + window.scrollY };
}

/** Cards keep their place while a box is drawn, so measure again only when cards are added. */
function cardBoxes(root: HTMLElement, cached: Map<string, Box>): Map<string, Box> {
  const cards = root.querySelectorAll<HTMLElement>("[data-card-key]");
  if (cards.length === cached.size) return cached;
  const boxes = new Map<string, Box>();
  for (const card of cards) {
    const rect = card.getBoundingClientRect();
    const key = card.dataset.cardKey;
    if (key === undefined) continue;
    const start = pagePoint(rect.left, rect.top);
    boxes.set(key, {
      left: start.x,
      top: start.y,
      right: start.x + rect.width,
      bottom: start.y + rect.height,
    });
  }
  return boxes;
}

function drawBox(element: HTMLElement | null, page: Box | null) {
  if (element === null) return;
  element.hidden = page === null;
  if (page === null) return;
  element.style.left = `${String(page.left - window.scrollX)}px`;
  element.style.top = `${String(page.top - window.scrollY)}px`;
  element.style.width = `${String(page.right - page.left)}px`;
  element.style.height = `${String(page.bottom - page.top)}px`;
}

/** The release lands on whatever is under the pointer; drop that one click. */
function swallowNextClick() {
  const swallow = (event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
  };
  window.addEventListener("click", swallow, { capture: true, once: true });
  setTimeout(() => window.removeEventListener("click", swallow, { capture: true }), 0);
}

function trackBox(
  down: PointerEvent,
  root: HTMLElement,
  box: RefObject<HTMLElement | null>,
  latest: RefObject<Options>,
) {
  const origin = pagePoint(down.clientX, down.clientY);
  const before = latest.current.selection;
  const base = down.shiftKey || down.ctrlKey || down.metaKey ? before : emptySelection;
  let pointer = { x: down.clientX, y: down.clientY };
  let drawing = false;
  let frame: number | null = null;
  let boxes = new Map<string, Box>();
  let hit = "";

  const update = () => {
    frame = null;
    const scroll = edgeSpeed(pointer.y, window.innerHeight);
    if (scroll !== 0) window.scrollBy(0, scroll);
    const page = boxBetween(origin, pagePoint(pointer.x, pointer.y));
    drawBox(box.current, page);
    boxes = cardBoxes(root, boxes);
    const keys = new Set(
      [...boxes].filter(([, card]) => overlaps(page, card)).map(([key]) => key),
    );
    const signature = [...keys].join("\n");
    if (signature !== hit) {
      hit = signature;
      const { items, setSelection } = latest.current;
      setSelection(
        setItems(
          base,
          items.filter((item) => keys.has(itemKey(item))),
          true,
        ),
      );
    }
    // Keep running while the pointer rests at an edge, so the page keeps scrolling.
    if (scroll !== 0) frame = requestAnimationFrame(update);
  };
  const schedule = () => {
    frame ??= requestAnimationFrame(update);
  };
  const onMove = (event: PointerEvent) => {
    pointer = { x: event.clientX, y: event.clientY };
    const moved = Math.hypot(pointer.x - down.clientX, pointer.y - down.clientY);
    if (!drawing && moved < startDistance) return;
    drawing = true;
    schedule();
  };
  const finish = () => {
    window.removeEventListener("pointermove", onMove);
    window.removeEventListener("pointerup", onUp);
    window.removeEventListener("pointercancel", finish);
    window.removeEventListener("keydown", onKey, true);
    window.removeEventListener("scroll", onScroll, true);
    if (frame !== null) cancelAnimationFrame(frame);
    drawBox(box.current, null);
  };
  const onUp = () => {
    if (drawing) swallowNextClick();
    finish();
  };
  const onKey = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    // Escape here only cancels the box; it must not also clear the selection.
    event.preventDefault();
    event.stopPropagation();
    latest.current.setSelection(before);
    finish();
  };
  const onScroll = () => {
    if (drawing) schedule();
  };
  window.addEventListener("pointermove", onMove);
  window.addEventListener("pointerup", onUp);
  window.addEventListener("pointercancel", finish);
  window.addEventListener("keydown", onKey, true);
  window.addEventListener("scroll", onScroll, true);
}
