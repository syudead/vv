/**
 * settingsPage は設定のページ（src/settings/SettingsPage.tsx）のテストが共有する偽のサーバーと
 * 描き方である。テストは話題ごとのファイル（src/settings/SettingsPage*.test.tsx）に分けてある。
 *
 * 各節の読み込みには既定の応答を返し、どれにも当たらない要求（メディアフォルダ、取り込み、
 * フォルダの選択など）は `fetchMock` へ通す。`routes` に URL を足すと、既定の応答を差し替える。
 */
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
 * sectionResponses は各節の読み込みの既定の応答である。節の振る舞いはそれぞれの節のテストが
 * 確かめるので、ここでは節が描けるだけの値を返す。Network はデスクトップ版でない（404）で、
 * プラグインのプロキシは何も保存されていない。
 */
function sectionResponses(): Record<string, () => Response> {
  return {
    "/api/settings/transcoding": () => json(transcodingSettings),
    "/api/api-tokens": () => json({ items: [] }),
    "/api/settings/network": () => json({ code: "not_found", message: "not found" }, 404),
    "/api/settings/plugin-proxy": () => json({ url: "" }),
    "/api/settings/auto-import": () => json({ enabled: false, watch: { state: "off" } }),
    "/api/settings/ollama": () =>
      json({ endpoint: "http://127.0.0.1:11434", model: "clef-flash", saved: false }),
    "/api/settings/auto-tagging": () =>
      json({
        enabled: false,
        afterImport: false,
        threshold: 0.8,
        tentativeThreshold: 0.5,
        queue: { queued: 0, running: 0, done: 0, failed: 0 },
      }),
    "/api/settings/version-judging": () => json({ enabled: false }),
    "/api/settings/comfyui": () => json({ apiKeySet: false, model: "flux2_klein_4b" }),
    "/api/work-info-plugins": () => json({ plugins: [] }),
    // 「Home」の節は HomeSection.test.tsx が確かめる。ここでは整理役が走っていない。
    "/api/home": () => json({ shelves: [], organizer: { running: false } }),
  };
}

/**
 * installSettingsServer は、各テストの前に偽のサーバーを fetch に差し込む。`requested` は
 * 既定の応答を返した要求の URL の記録で、ページが自分の読み込みだけをするかを確かめる。`routes` の
 * 応答は Promise でもよく、読み込みの途中を見るテストはそれを解かずにおく。
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

/**
 * LocationProbe は今のアドレス（パスとハッシュ）を `data-location` に置き、ブラウザの戻るの
 * 代わりのボタンを置く。疑似ロケールの検査に掛からないよう、文は持たない。
 */
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

/** currentLocation は今のアドレス（パスとハッシュ）である。 */
export function currentLocation(): string {
  return (
    document
      .querySelector("[data-testid='history-back']")
      ?.getAttribute("data-location") ?? ""
  );
}

/** historyBack はブラウザの戻るの代わりに、ルーターの履歴を 1 つ戻る。 */
export function historyBack(): void {
  document.querySelector<HTMLButtonElement>("[data-testid='history-back']")?.click();
}

/** renderSettings は App と同じ入れ子のルートで、`path` を開いた設定をシェルの中に描く。 */
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
                {/* App と同じく、設定のルートを外側の `*` のルートの下に置く。 */}
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
