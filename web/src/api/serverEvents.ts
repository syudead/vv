import type { Processing, Scan, VideoChanged } from "./client";

/**
 * ServerEventHandlers はサーバーから届く変化の受け取り先である。
 *
 * - `scan`: 直近のスキャンが変わった
 * - `processing`: 取り込みの段階ごとの残りが変わった
 * - `video`: 動画の状態が変わった（最新の内容は取り直す）
 * - `open`: つながった、またはつなぎ直した。切れていた間の変化を取り直す合図
 */
export interface ServerEventHandlers {
  scan?: (scan: Scan) => void;
  processing?: (processing: Processing) => void;
  video?: (id: number) => void;
  open?: () => void;
}

/** reconnectDelayMs は、ブラウザがつなぎ直しを諦めたときに張り直すまでの待ち時間である。 */
const reconnectDelayMs = 3000;

const subscribers = new Set<ServerEventHandlers>();
let source: EventSource | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

function dispatch<K extends keyof ServerEventHandlers>(
  kind: K,
  ...args: Parameters<NonNullable<ServerEventHandlers[K]>>
): void {
  for (const handlers of [...subscribers]) {
    const handler = handlers[kind] as ((...values: typeof args) => void) | undefined;
    handler?.(...args);
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
  current.addEventListener("open", () => dispatch("open"));
  current.addEventListener("scan", (event) => {
    const scan = parse<Scan>(event);
    if (scan !== undefined) dispatch("scan", scan);
  });
  current.addEventListener("processing", (event) => {
    const processing = parse<Processing>(event);
    if (processing !== undefined) dispatch("processing", processing);
  });
  current.addEventListener("video", (event) => {
    const changed = parse<VideoChanged>(event);
    if (changed !== undefined) dispatch("video", changed.id);
  });
  current.addEventListener("error", () => {
    // 接続中の切断はブラウザが自分でつなぎ直す。CLOSED は諦めた状態
    // （応答が 200 でない等）なので、少し待って張り直す。
    if (current.readyState !== EventSource.CLOSED || source !== current) return;
    source = null;
    if (subscribers.size === 0) return;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = undefined;
      if (subscribers.size > 0) connect();
    }, reconnectDelayMs);
  });
}

function disconnect(): void {
  if (reconnectTimer !== undefined) clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
  source?.close();
  source = null;
}

/**
 * subscribeServerEvents はサーバーからの変化の知らせを受け取る。戻り値で購読をやめる。
 *
 * 接続は購読者のあいだで1本を共有し、購読者がいなくなったら閉じる。一定間隔で
 * 問い合わせる代わりに使う。購読してから初回の取得をすると、取得のあとに起きた
 * 変化を取りこぼさない。
 */
export function subscribeServerEvents(handlers: ServerEventHandlers): () => void {
  subscribers.add(handlers);
  connect();
  return () => {
    subscribers.delete(handlers);
    if (subscribers.size > 0) return;
    // React の開発時の再実行のように、外してすぐ付け直す場合に張り直さない。
    queueMicrotask(() => {
      if (subscribers.size === 0) disconnect();
    });
  };
}
