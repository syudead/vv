import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import AuthGate from "./AuthGate";
import { useAudience } from "./audience";
import LoginPage from "./LoginPage";
import SetupPage from "./SetupPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function Library() {
  const audience = useAudience();
  return <p>library as {audience}</p>;
}

let currentLocation = "";
function LocationProbe() {
  const location = useLocation();
  currentLocation = `${location.pathname}${location.search}`;
  return null;
}

function renderGate(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LocationProbe />
      <AuthGate>
        <Routes>
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<Library />} />
        </Routes>
      </AuthGate>
    </MemoryRouter>,
  );
}

describe("AuthGate", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    currentLocation = "";
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  function answer(body: unknown) {
    fetchMock.mockImplementation(() => Promise.resolve(json(body)));
  }

  it("確認が済むまで何も描かない", async () => {
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    const { container } = renderGate("/");

    expect(container.innerHTML).toBe("");
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/session", expect.anything());

    await act(async () => finish(json({ state: "owner" })));
    expect(await screen.findByText("library as owner")).toBeDefined();
  });

  it.each(["/", "/videos/1", "/settings", "/login?next=%2Ftags", "/folders/3/A"])(
    "setupRequired ではどの経路（%s）も初回設定画面にする",
    async (path) => {
      answer({ state: "setupRequired" });
      renderGate(path);

      expect(
        await screen.findByRole("heading", { level: 1, name: "アカウントを作成" }),
      ).toBeDefined();
      expect(currentLocation).toBe("/setup");
      expect(screen.queryByText(/library/)).toBeNull();
    },
  );

  it.each([
    ["/settings", "/login?next=%2Fsettings"],
    ["/tags", "/login?next=%2Ftags"],
    ["/settings?tab=a", "/login?next=%2Fsettings%3Ftab%3Da"],
    ["/setup", "/"],
  ])("guest で %s を開くと %s へ置き換える", async (path, expected) => {
    answer({ state: "guest" });
    renderGate(path);

    await waitFor(() => expect(currentLocation).toBe(expected));
    if (expected === "/") {
      expect(await screen.findByText("library as guest")).toBeDefined();
    } else {
      expect(
        await screen.findByRole("heading", { level: 1, name: "ログイン" }),
      ).toBeDefined();
    }
  });

  it("guest は同じ画面をゲストとして描く", async () => {
    answer({ state: "guest" });
    renderGate("/folders");
    expect(await screen.findByText("library as guest")).toBeDefined();
    expect(currentLocation).toBe("/folders");
  });

  it("owner で /login を開くと、next を送ってサーバーが返した redirectTo へ移る", async () => {
    answer({ state: "owner", redirectTo: "/videos/12?t=30" });
    renderGate("/login?next=%2Fvideos%2F12%3Ft%3D30");

    expect(await screen.findByText("library as owner")).toBeDefined();
    expect(currentLocation).toBe("/videos/12?t=30");
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "/api/auth/session?next=%2Fvideos%2F12%3Ft%3D30",
    );
  });

  it("owner で戻り先を画面で判定せず、サーバーの答えに従う", async () => {
    // サーバーが外部の next を捨てて / を返した場合。
    answer({ state: "owner", redirectTo: "/" });
    renderGate("/login?next=%2F%2Fevil.example");

    expect(await screen.findByText("library as owner")).toBeDefined();
    expect(currentLocation).toBe("/");
  });

  it("owner で /setup を開くと / へ移る", async () => {
    answer({ state: "owner" });
    renderGate("/setup");

    expect(await screen.findByText("library as owner")).toBeDefined();
    expect(currentLocation).toBe("/");
  });

  it.each([
    [
      "500",
      () =>
        Promise.resolve(json({ code: "internal", message: "DB が応答しません" }, 500)),
    ],
    ["通信の失敗", () => Promise.reject(new TypeError("Failed to fetch"))],
  ])("確認が %s なら、描かず・移らず・自動で確かめ直さない", async (_, respond) => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      fetchMock.mockImplementation(respond);
      renderGate("/settings");

      expect(
        await screen.findByRole("heading", { name: "サーバーに接続できません" }),
      ).toBeDefined();
      expect(screen.queryByText(/library/)).toBeNull();
      expect(currentLocation).toBe("/settings");

      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_000);
      });
      expect(fetchMock).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  it("「再試行」で確かめ直す", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(json({ code: "internal", message: "失敗" }, 500));
    fetchMock.mockResolvedValueOnce(json({ state: "owner" }));
    renderGate("/");

    await user.click(await screen.findByRole("button", { name: "再試行" }));

    expect(await screen.findByText("library as owner")).toBeDefined();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
