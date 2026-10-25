/** Provide default section reads; routes overrides them and unmatched requests pass to fetchMock. */
import { render } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import type { MediaFolder, TranscodingSettings } from "../api/client";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import { ScanProvider } from "../shell/ScanProvider";
import SettingsPage from "../settings/SettingsPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";

import { OwnerAudience } from "./audience";

export function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function folder(id: number, path: string, version = 1): MediaFolder {
  return {
    id,
    path,
    version,
    createdAt: "2026-09-21T00:00:00Z",
    updatedAt: "2026-09-21T00:00:00Z",
  };
}

const transcodingSettings: TranscodingSettings = {
  videoEncoder: "software",
  effectiveEncoder: "software",
  checking: false,
  encoders: [
    { encoder: "nvenc", state: "unavailable", reason: "encoder_missing" },
    { encoder: "qsv", state: "unavailable", reason: "encoder_missing" },
    { encoder: "vaapi", state: "unavailable", reason: "encoder_missing" },
    { encoder: "videotoolbox", state: "unavailable", reason: "unsupported_os" },
  ],
};

/**
 * Keep section defaults minimal: section tests cover behavior; network defaults to non-desktop 404
 * with no proxy.
 */
function sectionResponses(): Record<string, () => Response> {
  return {
    "/api/settings/transcoding": () => json(transcodingSettings),
    "/api/api-tokens": () => json({ items: [] }),
    "/api/settings/network": () => json({ code: "not_found", message: "not found" }, 404),
    "/api/settings/plugin-proxy": () => json({ url: "" }),
    "/api/settings/auto-import": () => json({ enabled: false, watch: { state: "off" } }),
    "/api/settings/ollama": () =>
      json({
        endpoint: "http://127.0.0.1:11434",
        model: "clef-flash",
        generationModel: "",
        saved: false,
      }),
    "/api/settings/auto-tagging": () =>
      json({
        enabled: false,
        afterImport: false,
        threshold: 0.8,
        tentativeThreshold: 0.5,
        queue: { queued: 0, running: 0, done: 0, failed: 0 },
      }),
    "/api/settings/face-recognition": () =>
      json({
        enabled: false,
        recognizer: "sface_int8",
        recognizers: [],
        runtime: { state: "absent" },
        counts: { queued: 0, running: 0, done: 0, failed: 0 },
      }),
    "/api/settings/version-judging": () => json({ enabled: false }),
    "/api/settings/comfyui": () => json({ apiKeySet: false, model: "flux2_klein_4b" }),
    "/api/work-info-plugins": () => json({ plugins: [] }),
  };
}

/**
 * Record default-read URLs to assert page-local loading; route promises can remain pending to test
 * loading states.
 */
export function installSettingsServer() {
  const server = {
    fetchMock: vi.fn<typeof fetch>(),
    routes: {} as Record<string, () => Response | Promise<Response>>,
    requested: [] as string[],
  };

  beforeEach(() => {
    server.fetchMock = vi.fn<typeof fetch>();
    server.routes = {};
    server.requested = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const respond = server.routes[url] ?? sectionResponses()[url];
      if (respond !== undefined && (init?.method ?? "GET") === "GET") {
        server.requested.push(url);
        return Promise.resolve(respond());
      }
      return server.fetchMock(input, init);
    });
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  return server;
}

/** Expose location through an attribute, avoiding literal text in pseudo-locale checks. */
function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <button
      type="button"
      hidden
      data-testid="history-back"
      data-location={`${location.pathname}${location.hash}`}
      onClick={() => void navigate(-1)}
    />
  );
}

export function currentLocation(): string {
  return (
    document
      .querySelector("[data-testid='history-back']")
      ?.getAttribute("data-location") ?? ""
  );
}

export function historyBack(): void {
  document.querySelector<HTMLButtonElement>("[data-testid='history-back']")?.click();
}

export function renderSettings(path = "/settings") {
  const page = (
    <AppShell>
      <SettingsPage />
    </AppShell>
  );
  return render(
    <MemoryRouter initialEntries={[path]}>
      <TooltipProvider>
        <ToastProvider>
          <OwnerAudience>
            <ScanProvider>
              <ScanNoticeProvider>
                {/* Match App's nested settings route beneath the outer splat. */}
                <Routes>
                  <Route
                    path="*"
                    element={
                      <Routes>
                        <Route path="/settings" element={page} />
                        <Route path="/settings/*" element={page} />
                      </Routes>
                    }
                  />
                </Routes>
                <LocationProbe />
              </ScanNoticeProvider>
            </ScanProvider>
          </OwnerAudience>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}
