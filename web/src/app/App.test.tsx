import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Link } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FakeEventSource, installFakeEventSource } from "../api/fakeEventSource";
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
        <button type="button" onClick={() => toast("route toast")}>
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
      if (url === "/api/scans/current") {
        return Promise.resolve(
          json({ id: 188, state: "running", total: 10, completed: 4, failed: 0 }),
        );
      }
      return Promise.resolve(json({}));
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it("keeps one indicator instance and shared progress across library, settings, and playback routes", async () => {
    render(<App />);
    const user = userEvent.setup();
    const indicator = await screen.findByRole("button", { name: /取り込み中 40%/ });
    expect(indicator.closest(".fixed")?.classList.contains("bottom-4")).toBe(true);

    await user.click(screen.getByRole("link", { name: "フォルダへ" }));
    await user.click(screen.getByRole("link", { name: "設定へ" }));
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "動画へ" })).toBeDefined(),
    );
    expect(screen.getByRole("button", { name: /取り込み中 40%/ })).toBe(indicator);

    await user.click(screen.getByRole("link", { name: "動画へ" }));
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "ライブラリへ" })).toBeDefined(),
    );
    const playbackIndicator = screen.getByRole("button", { name: /取り込み中 40%/ });
    expect(playbackIndicator).toBe(indicator);
    const playbackPlacement = playbackIndicator.closest(".fixed")?.classList;
    // 再生画面でも右下に置く。右上には閉じる × がある。
    expect(playbackPlacement?.contains("bottom-4")).toBe(true);
    expect(playbackPlacement?.contains("sm:bottom-5")).toBe(true);
    expect(playbackPlacement?.contains("sm:top-2")).toBe(false);
    expect(playbackIndicator.getAttribute("aria-label")).toBe(
      "取り込み中 40%。取り込み状況を開く",
    );
  });

  it("keeps an active toast while a real link changes to playback placement", async () => {
    render(<App />);
    const user = userEvent.setup();
    await screen.findByRole("button", { name: /取り込み中 40%/ });

    await user.click(screen.getByRole("button", { name: "通知する" }));
    const toast = screen.getByText("route toast");
    const defaultPlacement = toast.parentElement?.classList;
    expect(defaultPlacement?.contains("top-16")).toBe(true);
    expect(defaultPlacement?.contains("lg:bottom-20")).toBe(true);

    await user.click(screen.getByRole("link", { name: "動画へ" }));
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "ライブラリへ" })).toBeDefined(),
    );
    expect(screen.getByText("route toast")).toBe(toast);
    const playbackPlacement = toast.parentElement?.classList;
    expect(playbackPlacement?.contains("top-1.5")).toBe(true);
    // どの幅でも上端の中央に出し、右上の閉じる × を覆わない。
    expect(playbackPlacement?.contains("items-center")).toBe(true);
    expect(playbackPlacement?.contains("items-end")).toBe(false);
    expect(playbackPlacement?.contains("lg:bottom-20")).toBe(false);
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
      await screen.findByRole("heading", { level: 1, name: "アカウントを作成" }),
    ).toBeDefined();
    expect(window.location.pathname).toBe("/setup");
    expect(
      screen.queryByRole("complementary", { name: "メインナビゲーション" }),
    ).toBeNull();
    // 取り込みの状態などプロバイダの要求を送らない。
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
      await screen.findByRole("heading", { level: 1, name: "ログイン" }),
    ).toBeDefined();
    expect(`${window.location.pathname}${window.location.search}`).toBe(
      "/login?next=%2Fsettings",
    );
    expect(
      screen.queryByRole("complementary", { name: "メインナビゲーション" }),
    ).toBeNull();
    expect(fetchMock).toHaveBeenCalledOnce();
  });
  it("ゲストには更新・取り込みの進捗・所有者だけのナビを出さず、所有者だけの API と /api/events を開かない", async () => {
    installFakeEventSource();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/auth/session" ? json({ state: "guest" }) : json({}),
      ),
    );
    render(<App />);
    await screen.findByRole("link", { name: "フォルダへ" });

    expect(
      screen.queryByRole("button", {
        name: /ライブラリを更新|取り込み中|メディアフォルダを設定/,
      }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: /取り込み/ })).toBeNull();
    const main = screen.getByRole("complementary", { name: "メインナビゲーション" });
    const names = Array.from(main.querySelectorAll("a, button")).map((node) =>
      node.textContent?.trim(),
    );
    expect(names).toEqual(["ライブラリ", "フォルダ", "ログイン"]);

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/auth/session",
    ]);
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it("所有者にはサイドバーの全項目と更新を出す", async () => {
    render(<App />);
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    const main = screen.getByRole("complementary", { name: "メインナビゲーション" });
    const names = Array.from(main.querySelectorAll("a, button")).map((node) =>
      node.textContent?.trim(),
    );
    expect(names).toEqual([
      "ライブラリ",
      "フォルダ",
      "タグ",
      "最近追加",
      "視聴途中",
      "設定",
      "ログアウト",
    ]);
    expect(screen.getByRole("button", { name: "取り込み中" })).toBeDefined();
  });
});
