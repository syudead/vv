import { act } from "@testing-library/react";
import { vi } from "vitest";

/**
 * FakeEventSource はテストでサーバーからの変化の知らせを再現する。
 * `installFakeEventSource` で EventSource の代わりに置き、`emit` で知らせを送る。
 */
export class FakeEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  static instances: FakeEventSource[] = [];

  readonly url: string;
  readyState = FakeEventSource.CONNECTING;
  private readonly listeners = new Map<string, Set<(event: Event) => void>>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: Event) => void): void {
    const set = this.listeners.get(type) ?? new Set();
    set.add(listener);
    this.listeners.set(type, set);
  }

  removeEventListener(type: string, listener: (event: Event) => void): void {
    this.listeners.get(type)?.delete(listener);
  }

  close(): void {
    this.readyState = FakeEventSource.CLOSED;
  }

  dispatch(type: string, data?: unknown): void {
    const event =
      data === undefined
        ? new Event(type)
        : new MessageEvent(type, { data: JSON.stringify(data) });
    for (const listener of [...(this.listeners.get(type) ?? [])]) listener(event);
  }
}

/** currentEventSource は開いている接続を返す。無ければ例外を投げる。 */
export function currentEventSource(): FakeEventSource {
  const open = FakeEventSource.instances.filter(
    (source) => source.readyState !== FakeEventSource.CLOSED,
  );
  const source = open.at(-1);
  if (source === undefined) throw new Error("変化の知らせの接続がありません");
  return source;
}

/** emitServerEvent は開いている接続へ知らせを1つ送る。 */
export async function emitServerEvent(name: string, data?: unknown): Promise<void> {
  await act(async () => {
    const source = currentEventSource();
    if (name === "open") source.readyState = FakeEventSource.OPEN;
    source.dispatch(name, data);
    await Promise.resolve();
  });
}

/** installFakeEventSource は EventSource を FakeEventSource へ差し替える。 */
export function installFakeEventSource(): void {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
}
