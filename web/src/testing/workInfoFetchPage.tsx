/** Fake server-side plugin execution; each test file owns its player mock. */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import type {
  WorkInfoFetch,
  WorkInfoFetchApplied,
  WorkInfoPlugin,
} from "../api/workInfoPlugins";
import { type Audience, AudienceProvider } from "../auth/audience";
import VideoPage from "../player/VideoPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";

export const video: Video = {
  id: 7,
  title: "テスト動画",
  public: false,
  sizeBytes: 1_000,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  fileModifiedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  thumbnailUrl: "/api/videos/7/thumbnail",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/テスト動画.mp4", openable: false },
  tags: [
    {
      kind: null,
      id: 1,
      name: "既存",
      manual: true,
      fromFolder: false,
      tentative: false,
    },
  ],
};

export const works: WorkInfoPlugin = {
  id: 3,
  kind: "work",
  sources: [],
  domains: [],
  name: "works",
  fileName: "works.wasm",
  operations: ["search", "fetch"],
  digest: "d",
  state: "approved",
  settings: [
    { key: "token", label: "API token", secret: true, required: true, set: true },
  ],
  network: true,
  updatedAt: 0,
};

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function failure(status: number, body: Record<string, unknown>): Response {
  return json({ message: "failed", ...body }, status);
}

export function fetchResult(overrides: Partial<WorkInfoFetch> = {}): WorkInfoFetch {
  return {
    fetchId: "F1",
    expiresAt: 0,
    tags: [],
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    covers: [],
    ...overrides,
  };
}

export function appliedResult(
  applied: Video,
  overrides: Partial<WorkInfoFetchApplied> = {},
): WorkInfoFetchApplied {
  return { video: applied, skippedTags: [], skipped: [], linkedPeople: [], ...overrides };
}

export const workId = "W-1";

/** Display-only candidate URL; never submitted for fetching. */
export const workUrl = "https://works.example/w/1";

export const server = {
  video,
  plugins: [works] as WorkInfoPlugin[],
  search: vi.fn<(body: unknown) => Response>(),
  fetch: vi.fn<(body: unknown) => Response>(),
  /** A promise holds the apply's answer back. */
  apply: vi.fn<(body: unknown) => Response | Promise<Response>>(),
  upload: vi.fn<(body: unknown) => Response>(),
  related: { items: [] } as unknown,
  requests: [] as string[],
};

function renderPage(audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/videos/7"]}>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

export async function openPage(audience: Audience = "owner") {
  renderPage(audience);
  await screen.findByRole("heading", { level: 1, name: "テスト動画" });
}

/** The Work tab is listed once the plugin list arrives, even for a video without work information. */
export async function openWorkTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("tab", { name: "Work" }));
}

export function entry() {
  return screen.findByRole("button", { name: /^Fetch( again)?$/ });
}

export async function openDialog() {
  await openPage();
  const user = userEvent.setup();
  await openWorkTab(user);
  await user.click(await entry());
  const dialog = await screen.findByRole("dialog", { name: "Fetch work info" });
  return { user, dialog };
}

export async function fetchById() {
  const opened = await openDialog();
  await opened.user.click(within(opened.dialog).getByRole("radio", { name: "ID" }));
  const query = within(opened.dialog).getByRole("textbox", { name: "The work's ID" });
  await opened.user.clear(query);
  await opened.user.type(query, workId);
  await opened.user.click(within(opened.dialog).getByRole("button", { name: "Fetch" }));
  // Every comparison has Back, whatever fields the result carries.
  await within(opened.dialog).findByRole("button", { name: "Back" });
  return opened;
}

export function heading() {
  // The modal hides the background page from accessibility queries.
  return screen.getByRole("heading", { level: 1, hidden: true });
}

function respond(url: string, body: unknown): Response | Promise<Response> {
  if (url === "/api/auth/session") return json({ state: "owner" });
  if (url === "/api/work-info-plugins") return json({ plugins: server.plugins });
  if (url === "/api/work-info-plugins/3/search") return server.search(body);
  if (url === "/api/videos/7/work-info-fetches") return server.fetch(body);
  if (url === "/api/videos/7/work-info-fetches/F1/apply") return server.apply(body);
  if (url === "/api/videos/7/thumbnail-image") return server.upload(body);
  if (url === "/api/thumbnail-edits/readiness") return json({ connection: true });
  if (url === "/api/videos/7") return json(server.video);
  if (url === "/api/videos/7/related") return json(server.related);
  if (url === "/api/videos/7/subtitles") return json({ subtitles: [] });
  if (url === "/api/tags") return json([]);
  return json({});
}

/** Call within describe to install default fetch responses per test and restore afterward. */
export function useFetchServer() {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    server.video = video;
    server.plugins = [works];
    server.search.mockReset();
    server.fetch.mockReset();
    server.apply.mockReset();
    server.upload.mockReset();
    server.fetch.mockImplementation(() =>
      json(fetchResult({ title: "取得した題名", tags: ["既存", "新しい"] }), 201),
    );
    server.related = { items: [] };
    server.requests = [];
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      server.requests.push(`${method} ${url.split("?")[0]!}`);
      const body: unknown =
        typeof init?.body === "string" ? JSON.parse(init.body) : init?.body;
      return Promise.resolve(respond(url, body));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
}
