/** PUT returns starting when enabled and off otherwise. */
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import type { AutoImportSettings } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { ScanProvider } from "../shell/ScanProvider";

import { OwnerAudience } from "./audience";

export const autoImportURL = "/api/settings/auto-import";

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function autoImportSettings(
  enabled: boolean,
  watch: AutoImportSettings["watch"] = { state: enabled ? "active" : "off" },
): AutoImportSettings {
  return { enabled, watch };
}

export function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

/** Include a router because settings descriptions contain links. */
export function renderWithScan(ui: ReactNode) {
  const wrap = (node: ReactNode) => (
    <MemoryRouter>
      <OwnerAudience>
        <ScanProvider>{node}</ScanProvider>
      </OwnerAudience>
    </MemoryRouter>
  );
  const view = render(wrap(ui));
  return { ...view, rerenderWith: (next: ReactNode) => view.rerender(wrap(next)) };
}

export function installAutoImportServer() {
  const server = {
    fetchMock: vi.fn<typeof fetch>(),
    current: autoImportSettings(true),
    route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
      const url = String(input);
      if (url === "/api/scans/current")
        return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url === autoImportURL && init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as { enabled: boolean };
        server.current = autoImportSettings(
          body.enabled,
          body.enabled ? { state: "starting" } : { state: "off" },
        );
        return Promise.resolve(json(server.current));
      }
      if (url === autoImportURL) return Promise.resolve(json(server.current));
      return Promise.reject(new Error(`unexpected request: ${url}`));
    },
    gets(): number {
      return server.fetchMock.mock.calls.filter(
        ([input, init]) => String(input) === autoImportURL && init?.method !== "PUT",
      ).length;
    },
    puts(): number {
      return server.fetchMock.mock.calls.filter(([, init]) => init?.method === "PUT")
        .length;
    },
  };

  beforeEach(() => {
    server.current = autoImportSettings(true);
    server.fetchMock.mockReset();
    server.fetchMock.mockImplementation((input, init) => server.route(input, init));
    vi.stubGlobal("fetch", server.fetchMock);
    installFakeEventSource();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  return server;
}
