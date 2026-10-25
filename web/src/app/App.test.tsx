import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Link } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FakeEventSource, installFakeEventSource } from "../api/fakeEventSource";
import type { UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import App from "./App";

vi.mock("../library/LibraryPage", () => ({
  default: function MockLibraryPage() {
    const toast = useToast();
    return (
      <div>
        <Link to="/folders">フォルダへ</Link>
        <Link to="/settings">設定へ</Link>
        <Link to="/videos/1">動画へ</Link>
        <button type="button" onClick={() => toast("route toast" as UiText)}>
          通知する
        </button>
      </div>
    );
  },
}));

vi.mock("../folders/FolderPage", () => ({
  default: () => (
    <div>
      <Link to="/settings">設定へ</Link>
      <Link to="/videos/1">動画へ</Link>
    </div>
  ),
}));

vi.mock("../settings/SettingsPage", () => ({
  default: () => (
    <div>
      <Link to="/">ライブラリへ</Link>
      <Link to="/videos/1">動画へ</Link>
    </div>
  ),
}));

vi.mock("../player/VideoPage", () => ({
  default: () => <Link to="/">ライブラリへ</Link>,
}));

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("App", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    window.sessionStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url.startsWith("/api/activity?")) {
        return Promise.resolve(
          json({
            items: [],
            counts: { running: 0, attention: 0, done: 0 },
            features: {},
          }),
        );
      }
      if (url === "/api/scans/current") {
        return Promise.resolve(
          json({
            id: 188,
            status: "running",
            videos: { total: 10, settled: 4 },
            issues: { failed: 0, substituted: 0, revision: 0 },
            state: "running",
          }),
        );
      }
      return Promise.resolve(json({}));
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it("keeps one indicator instance and shared progress across library, settings, and playback routes", async () => {
    render(<App />);
    const user = userEvent.setup();
    const indicator = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    const home = await screen.findByRole("link", { name: "VVMDM home" });
    expect(home.getAttribute("href")).toBe("/");
    expect(home.querySelectorAll('img[alt=""]')).toHaveLength(2);
    expect(indicator.closest(".fixed")?.classList.contains("bottom-4")).toBe(true);

    await user.click(screen.getByRole("link", { name: "フォルダへ" }));
    // These routes load lazily, so wait for their content.
    await user.click(await screen.findByRole("link", { name: "設定へ" }));
    expect(await screen.findByRole("link", { name: "動画へ" })).toBeDefined();
    expect(screen.getByRole("button", { name: /^Scanning 4 of 10 videos done\./ })).toBe(
      indicator,
    );

    await user.click(screen.getByRole("link", { name: "動画へ" }));
    expect(await screen.findByRole("link", { name: "ライブラリへ" })).toBeDefined();
    const playbackIndicator = screen.getByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    expect(playbackIndicator).toBe(indicator);
    const playbackPlacement = playbackIndicator.closest(".fixed")?.classList;
    expect(playbackPlacement?.contains("bottom-4")).toBe(true);
    expect(playbackPlacement?.contains("sm:bottom-5")).toBe(true);
    expect(playbackPlacement?.contains("sm:top-2")).toBe(false);
    expect(playbackIndicator.getAttribute("aria-label")).toBe(
      "Scanning 4 of 10 videos done. Open the scan status",
    );
  });

  it("keeps root filter URLs on the integrated home", async () => {
    window.history.replaceState({}, "", "/?tag=3");
    render(<App />);

    expect(await screen.findByRole("button", { name: "通知する" })).toBeDefined();
    expect(`${window.location.pathname}${window.location.search}`).toBe("/?tag=3");
  });

  it("redirects former library bookmarks with their query and hash intact", async () => {
    window.history.replaceState({}, "", "/library?tag=3#results");
    render(<App />);
    expect(await screen.findByRole("button", { name: "通知する" })).toBeDefined();
    expect(
      `${window.location.pathname}${window.location.search}${window.location.hash}`,
    ).toBe("/?tag=3#results");
  });

  it("keeps an active toast while a real link changes to playback placement", async () => {
    render(<App />);
    const user = userEvent.setup();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });

    await user.click(await screen.findByRole("button", { name: "通知する" }));
    const toast = await screen.findByText("route toast");
    const toaster = () =>
      document.querySelector<HTMLElement>("[data-sonner-toaster]")?.style;
    expect(toaster()?.getPropertyValue("--offset-top")).toBe(
      "calc(var(--spacing-navbar) + 0.5rem)",
    );
    expect(
      document.querySelector("[data-sonner-toaster]")?.getAttribute("data-x-position"),
    ).toBe("right");

    await user.click(screen.getByRole("link", { name: "動画へ" }));
    expect(await screen.findByRole("link", { name: "ライブラリへ" })).toBeDefined();
    expect(screen.getByText("route toast").textContent).toBe(toast.textContent);
    expect(toaster()?.getPropertyValue("--offset-top")).toBe("0.375rem");
    expect(
      document.querySelector("[data-sonner-toaster]")?.getAttribute("data-x-position"),
    ).toBe("center");
  });

  it("初回設定画面をシェルとプロバイダの外に描く", async () => {
    window.history.replaceState({}, "", "/videos/1");
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/auth/session") {
        return Promise.resolve(json({ state: "setupRequired" }));
      }
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    });
    render(<App />);

    expect(
      await screen.findByRole("heading", { level: 1, name: "Create an account" }),
    ).toBeDefined();
    expect(window.location.pathname).toBe("/setup");
    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
    // Avoid provider requests such as import status.
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("ゲストの設定画面はログイン画面へ置き換え、シェルを描かない", async () => {
    window.history.replaceState({}, "", "/settings");
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/auth/session") {
        return Promise.resolve(json({ state: "guest" }));
      }
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    });
    render(<App />);

    expect(
      await screen.findByRole("heading", { level: 1, name: "Sign in" }),
    ).toBeDefined();
    expect(`${window.location.pathname}${window.location.search}`).toBe(
      "/login?next=%2Fsettings",
    );
    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
    expect(fetchMock).toHaveBeenCalledOnce();
  });
  it("ゲストには取り込みの button・進捗・所有者だけのナビを出さず、所有者だけの API と /api/events を開かない", async () => {
    installFakeEventSource();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/auth/session" ? json({ state: "guest" }) : json({}),
      ),
    );
    render(<App />);
    await screen.findByRole("link", { name: "フォルダへ" });

    expect(screen.queryByRole("button", { name: /Scan library|Scanning/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /scan/i })).toBeNull();
    const main = screen.getByRole("complementary", { name: "Main navigation" });
    const names = Array.from(main.querySelectorAll("a, button")).map((node) =>
      node.textContent?.trim(),
    );
    expect(names).toEqual(["Home", "Folders", "Sign in"]);

    await new Promise((resolve) => setTimeout(resolve, 0));
    // The pairing state is guest-capable: it decides the guest's peer notice.
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/auth/session",
      "/api/peer",
    ]);
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it("段階が done でない所有者の設定画面は、シェルの外のオンボードへ置き換える", async () => {
    window.history.replaceState({}, "", "/settings");
    const original = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) =>
      String(input) === "/api/onboarding"
        ? Promise.resolve(json({ step: "scan" }))
        : original!(input, init),
    );
    render(<App />);

    expect(
      await screen.findByRole("heading", { level: 1, name: "Scan your library" }),
    ).toBeDefined();
    expect(window.location.pathname).toBe("/welcome");
    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
  });

  it("所有者にはサイドバーの全項目を出し、トップバーに取り込みの button を出さない", async () => {
    render(<App />);
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    const main = await screen.findByRole("complementary", { name: "Main navigation" });
    const names = Array.from(main.querySelectorAll("a, button")).map((node) =>
      node.textContent?.trim(),
    );
    expect(names).toEqual([
      "Home",
      "Folders",
      "History",
      "Playlists",
      "Tags",
      "People",
      "Organizations",
      "Duplicates",
      "Activity",
      "New virtual video",
      "Settings",
      "Sign out",
    ]);
    const topBar = screen.getByRole("banner");
    expect(within(topBar).queryByRole("button", { name: /scan|refresh/i })).toBeNull();
  });
});
