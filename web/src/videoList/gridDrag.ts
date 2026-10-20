import type { ClientRect } from "@dnd-kit/core";

/**
 * Grid cards keep their nodes here instead of registering with dnd-kit one by one; dnd-kit
 * re-renders every registered card several times when a drag starts or its target changes.
 */
export interface GridCards {
  register: (key: string, node: Element | null) => void;
  /** Measured on first use after a change, so a drag reads each card's layout once. */
  rects: () => ReadonlyMap<string, ClientRect>;
  invalidate: () => void;
}

export function createGridCards(): GridCards {
  const nodes = new Map<string, Element>();
  let measured: Map<string, ClientRect> | null = null;
  return {
    register: (key, node) => {
      if (node === null) nodes.delete(key);
      else nodes.set(key, node);
      measured = null;
    },
    rects: () => {
      if (measured !== null) return measured;
      measured = new Map();
      for (const [key, node] of nodes) {
        const { top, left, width, height } = node.getBoundingClientRect();
        measured.set(key, {
          top,
          left,
          width,
          height,
          bottom: top + height,
          right: left + width,
        });
      }
      return measured;
    },
    invalidate: () => {
      measured = null;
    },
  };
}

/** A value shared by many keys, where each key re-renders only when its own selection changes. */
export interface KeyedStore<V, S> {
  set: (value: V) => void;
  subscribe: (listener: () => void) => () => void;
  select: (key: string) => S;
}

export function createKeyedStore<V, S>(
  initial: V,
  pick: (key: string, value: V) => S,
  same: (previous: S, next: S) => boolean,
): KeyedStore<V, S> {
  let value = initial;
  const selected = new Map<string, S>();
  const listeners = new Set<() => void>();
  return {
    set: (next) => {
      if (Object.is(next, value)) return;
      value = next;
      for (const listener of [...listeners]) listener();
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    select: (key) => {
      const next = pick(key, value);
      const previous = selected.get(key);
      if (previous !== undefined && same(previous, next)) return previous;
      selected.set(key, next);
      return next;
    },
  };
}
