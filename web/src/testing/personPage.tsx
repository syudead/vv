/** A fake server for the person page; each test file sets the state its flow needs. */
import { render } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import type { Person } from "../api/credits";
import { installFakeEventSource } from "../api/fakeEventSource";
import type { FaceAppearance, FaceCandidate } from "../api/personFaces";
import type {
  PersonPluginCandidate,
  PersonPluginFetchedPerson,
  SkippedPersonValue,
} from "../api/personPluginFetches";
import type { WorkInfoPlugin } from "../api/workInfoPlugins";
import PersonEditPage from "../credits/person/edit/PersonEditPage";
import PersonFacesPage from "../credits/person/PersonFacesPage";
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
    provisional: false,
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
  faceCandidates: [] as FaceCandidate[],
  appearances: [] as FaceAppearance[],
  /** Served as the second page of appearances, after the cursor `page-2`. */
  laterAppearances: [] as FaceAppearance[],
  /** The person a confirm answers with, also served at its own address. */
  mergeTarget: null as Person | null,
  /** Whether face recognition is on, as the settings report it. */
  faceRecognition: true,
  /** How many videos the person performs in. */
  videoCount: 1,
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

function faceRoute(method: string, url: URL): Response | null {
  if (url.pathname === "/api/people/7/face-candidates") {
    return jsonResponse({ items: server.faceCandidates });
  }
  if (url.pathname === "/api/people/7/appearances") {
    if (url.searchParams.get("cursor") === "page-2") {
      return jsonResponse({
        items: server.laterAppearances,
        total: server.appearances.length + server.laterAppearances.length,
      });
    }
    return jsonResponse({
      items: server.appearances,
      total: server.appearances.length + server.laterAppearances.length,
      nextCursor: server.laterAppearances.length > 0 ? "page-2" : undefined,
    });
  }
  if (url.pathname === "/api/people/7/similar-people") {
    return jsonResponse({
      items: [
        {
          person: person({ id: 9, name: "Mio" }),
          similarity: 0.71,
          faceCount: 6,
          sampleAppearanceIds: [],
          firstSeenTitle: "Beach day",
        },
      ],
    });
  }
  const move = /^\/api\/people\/(7|9)\/appearances\/move$/.exec(url.pathname);
  if (move !== null && method === "POST") {
    return jsonResponse(move[1] === "7" ? person({ id: 9, name: "Mio" }) : person());
  }
  if (url.pathname === "/api/people/7/appearances/split" && method === "POST") {
    return jsonResponse(person({ id: 12, name: "", provisional: true }), 201);
  }
  const split = /^\/api\/people\/7\/appearances\/(\d+)\/split$/.exec(url.pathname);
  if (split !== null && method === "POST") {
    server.appearances = server.appearances.filter(
      (item) => item.appearanceId !== Number(split[1]),
    );
    return jsonResponse(person({ id: 12, name: "", provisional: true }), 201);
  }
  const pair =
    /^\/api\/people\/7\/face-candidates\/(\d+)\/(confirm|dismiss|reopen)$/.exec(
      url.pathname,
    );
  if (pair !== null && method === "POST") {
    if (pair[2] === "reopen") return new Response(null, { status: 204 });
    server.faceCandidates = server.faceCandidates.filter(
      (item) => item.person.id !== Number(pair[1]),
    );
    if (pair[2] === "dismiss") return new Response(null, { status: 204 });
    return jsonResponse({ person: server.mergeTarget, undoActivityId: 31 });
  }
  if (url.pathname === "/api/activity/31/undo" && method === "POST") {
    return jsonResponse({ id: 32 });
  }
  if (url.pathname === "/api/people/7/merge" && method === "POST") {
    return jsonResponse({ person: server.person, notFoundIds: [] });
  }
  const target = server.mergeTarget;
  if (target !== null && url.pathname === `/api/people/${String(target.id)}`) {
    return jsonResponse(target);
  }
  return null;
}

function respond(method: string, url: URL, body: unknown): Response {
  if (url.pathname === "/api/videos") {
    // The person performs in every served video and has no other role.
    const role = url.searchParams.get("personRole");
    const ids = Array.from({ length: server.videoCount }, (_, index) => index + 1);
    return role === null || role === "performer"
      ? jsonResponse({ items: ids.map((id) => video(id)), total: ids.length })
      : jsonResponse({ items: [], total: 0 });
  }
  if (url.pathname === "/api/settings/face-recognition") {
    return jsonResponse({ enabled: server.faceRecognition });
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
  const face = faceRoute(method, url);
  if (face !== null) return face;
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
    server.faceCandidates = [];
    server.appearances = [];
    server.laterAppearances = [];
    server.mergeTarget = null;
    server.faceRecognition = true;
    server.videoCount = 1;
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

function VideoScreen() {
  const location = useLocation();
  const autoplay = (location.state as { autoplay?: boolean } | null)?.autoplay === true;
  return (
    <p data-testid="video-screen">
      {`${location.pathname}${location.search}${autoplay ? " autoplay" : ""}`}
    </p>
  );
}

export function renderPage(path = "/people/7") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/people/:id" element={<PersonPage />} />
            <Route path="/people/:id/edit" element={<PersonEditPage />} />
            <Route path="/people/:id/faces" element={<PersonFacesPage />} />
            <Route path="/videos/:id" element={<VideoScreen />} />
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
