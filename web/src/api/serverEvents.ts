import { getAuthSession } from "./auth";
import {
  type GroupChanged,
  reloadIfNoLongerOwner,
  type Scan,
  type VideoChanged,
} from "./client";
import type { components } from "./gen/openapi";

type SmartFolderChanged = components["schemas"]["SmartFolderChanged"];

/** Events invalidate local data; group IDs include ancestors. Reconnected opens require refresh because disconnected clients miss changes. */
export interface ServerEventHandlers {
  scan?: (scan: Scan) => void;
  video?: (id: number) => void;
  group?: (ids: readonly number[]) => void;
  smartFolder?: (ids: readonly number[]) => void;
  candidates?: () => void;
  activity?: () => void;
  home?: () => void;
  open?: (reconnected: boolean) => void;
}

const reconnectDelayMs = 3000;

interface Subscriber {
  handlers: ServerEventHandlers;
  opened: boolean;
}

const subscribers = new Set<Subscriber>();
let source: EventSource | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

function dispatch<K extends keyof ServerEventHandlers>(
  kind: K,
  ...args: Parameters<NonNullable<ServerEventHandlers[K]>>
): void {
  for (const { handlers } of [...subscribers]) {
    const handler = handlers[kind] as ((...values: typeof args) => void) | undefined;
    handler?.(...args);
  }
}

function dispatchOpen(): void {
  for (const subscriber of [...subscribers]) {
    const reconnected = subscriber.opened;
    subscriber.opened = true;
    subscriber.handlers.open?.(reconnected);
  }
}

function parse<T>(event: Event): T | undefined {
  try {
    return JSON.parse((event as MessageEvent<string>).data) as T;
  } catch {
    return undefined;
  }
}

function connect(): void {
  if (source !== null || typeof EventSource === "undefined") return;
  const current = new EventSource("/api/events");
  source = current;
  current.addEventListener("open", dispatchOpen);
  current.addEventListener("scan", (event) => {
    const scan = parse<Scan>(event);
    if (scan !== undefined) dispatch("scan", scan);
  });
  current.addEventListener("video", (event) => {
    const changed = parse<VideoChanged>(event);
    if (changed !== undefined) dispatch("video", changed.id);
  });
  current.addEventListener("group", (event) => {
    const changed = parse<GroupChanged>(event);
    if (changed !== undefined) dispatch("group", changed.ids);
  });
  current.addEventListener("smartFolder", (event) => {
    const changed = parse<SmartFolderChanged>(event);
    if (changed !== undefined) dispatch("smartFolder", changed.ids);
  });
  current.addEventListener("candidates", () => dispatch("candidates"));
  current.addEventListener("activity", () => dispatch("activity"));
  current.addEventListener("home", () => dispatch("home"));
  current.addEventListener("error", () => {
    // CLOSED means browser retries stopped; check audience explicitly because EventSource hides HTTP status.
    // Reload once after owner access expires; otherwise reconnect after a delay.
    if (current.readyState !== EventSource.CLOSED || source !== current) return;
    source = null;
    if (subscribers.size === 0) return;
    const scheduleReconnect = () => {
      if (source !== null || reconnectTimer !== undefined || subscribers.size === 0)
        return;
      reconnectTimer = setTimeout(() => {
        reconnectTimer = undefined;
        if (subscribers.size > 0) connect();
      }, reconnectDelayMs);
    };
    getAuthSession().then((session) => {
      if (!reloadIfNoLongerOwner(session.state)) scheduleReconnect();
    }, scheduleReconnect);
  });
}

function disconnect(): void {
  if (reconnectTimer !== undefined) clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  source?.close();
  source = null;
}

/** Share one connection across subscribers; subscribe before fetching initial data to avoid missing intervening changes. */
export function subscribeServerEvents(handlers: ServerEventHandlers): () => void {
  // An already-open connection means this subscriber missed no events; its next open is a reconnect.
  const subscriber: Subscriber = {
    handlers,
    opened: source !== null && source.readyState === EventSource.OPEN,
  };
  subscribers.add(subscriber);
  connect();
  return () => {
    subscribers.delete(subscriber);
    if (subscribers.size > 0) return;
    // Preserve the connection across immediate resubscription, including React effect replay.
    queueMicrotask(() => {
      if (subscribers.size === 0) disconnect();
    });
  };
}
