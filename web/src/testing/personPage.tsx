/** A fake server for the person page; each test file sets the state its flow needs. */
import { render } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import type { Person } from "../api/credits";
import { installFakeEventSource } from "../api/fakeEventSource";
import type {
  PersonPluginCandidate,
  PersonPluginFetchedPerson,
  SkippedPersonValue,
} from "../api/personPluginFetches";
import type { WorkInfoPlugin } from "../api/workInfoPlugins";
import PersonEditPage from "../credits/person/edit/PersonEditPage";
import PersonPage from "../credits/person/PersonPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { video } from "./groupPage";

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function person(overrides: Partial<Person> = {}): Person {
  return {
    id: 7,
    name: "Aoi",
    aliases: [],
    externalIds: [],
    profile: {},
    favorite: false,
    videoCount: 1,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export const people: WorkInfoPlugin = {
  id: 5,
  kind: "person",
  sources: ["fanza"],
  autoLookup: true,
  name: "people",
  fileName: "people.wasm",
  operations: ["search", "fetch"],
  digest: "d",
  state: "approved",
  settings: [],
  network: true,
  updatedAt: 0,
};

export interface Sent {
  method: string;
  url: URL;
  body: BodyInit | null | undefined;
}

export const server = {
  person: null as Person | null,
  sent: [] as Sent[],
  /** When set, the face upload answers only once this settles. */
  uploadGate: null as Promise<void> | null,
  plugins: [] as WorkInfoPlugin[],
  candidates: [] as PersonPluginCandidate[],
  fetched: { name: "Aoi", aliases: [], externalIds: [] } as PersonPluginFetchedPerson,
  /** The person the apply answers with, and the values it leaves out. */
  applied: null as Person | null,
  skipped: [] as SkippedPersonValue[],
};

export function sentBody(request: Sent | undefined): unknown {
  return JSON.parse(String(request?.body));
}

async function respondLater(method: string, url: URL, body: unknown): Promise<Response> {
  if (url.pathname === "/api/people/7/image" && method === "PUT" && server.uploadGate) {
    // Answer with the person as it was when the upload was sent.
    const saved = { ...server.person, imageRevision: 3 } as Person;
    await server.uploadGate;
    return jsonResponse(saved);
  }
  return respond(method, url, body);
}

/** Without a candidate, the stored ID whose source the plugin accepts, as the server picks it. */
function pluginFetch(current: Person, body: unknown): Response {
  const request = body as { pluginId: number; source?: string };
  const accepted = current.externalIds.some((id) => people.sources.includes(id.source));
  if (request.source === undefined && !accepted)
    return jsonResponse({ code: "person_external_id_missing", message: "x" }, 409);
  return jsonResponse(
    { fetchId: "f1", expiresAt: 1_900_000_000, person: server.fetched },
    201,
  );
}

function pluginRoute(method: string, url: URL, body: unknown): Response | null {
  if (url.pathname === "/api/work-info-plugins" && method === "GET")
    return jsonResponse({ plugins: server.plugins });
  if (url.pathname === "/api/work-info-plugins/5/person-search")
    return jsonResponse({ candidates: server.candidates });
  if (server.person === null) return null;
  if (url.pathname === "/api/people/7/plugin-fetches")
    return pluginFetch(server.person, body);
  if (url.pathname === "/api/people/7/plugin-fetches/f1/apply") {
    server.person = server.applied ?? server.person;
    return jsonResponse({ person: server.person, skipped: server.skipped });
  }
  return null;
}

function respond(method: string, url: URL, body: unknown): Response {
  if (url.pathname === "/api/videos") {
    return jsonResponse({ items: [video(1)], total: 1 });
  }
  const plugin = pluginRoute(method, url, body);
  if (plugin !== null) return plugin;
  if (server.person === null) {
    return jsonResponse({ code: "not_found", reason: "person_not_found" }, 404);
  }
  if (url.pathname === "/api/people/7" && method === "GET")
    return jsonResponse(server.person);
  if (url.pathname === "/api/people/7/image" && method === "PUT") {
    server.person = { ...server.person, imageRevision: 3 };
    return jsonResponse(server.person);
  }
  if (url.pathname === "/api/favorites" && method === "PUT") {
    server.person = { ...server.person, favorite: true };
    return jsonResponse({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 0 });
  }
  return jsonResponse({ code: "not_found" }, 404);
}

class IdleIntersectionObserver {
  observe() {}
  disconnect() {}
}

export function installPersonServer() {
  beforeEach(() => {
    server.person = person();
    server.sent = [];
    server.uploadGate = null;
    server.plugins = [];
    server.candidates = [];
    server.fetched = { name: "Aoi", aliases: [], externalIds: [] };
    server.applied = null;
    server.skipped = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = new URL(String(input), "http://localhost");
        const method = init?.method ?? "GET";
        server.sent.push({ method, url, body: init?.body });
        const body: unknown =
          typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
        return respondLater(method, url, body);
      }),
    );
    vi.stubGlobal("IntersectionObserver", IdleIntersectionObserver);
    installFakeEventSource();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });
}

export function renderPage() {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/people/7"]}>
          <Routes>
            <Route path="/people/:id" element={<PersonPage />} />
            <Route path="/people/:id/edit" element={<PersonEditPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

export function sent(method: string, pathname: string): Sent[] {
  return server.sent.filter(
    (request) => request.method === method && request.url.pathname === pathname,
  );
}
