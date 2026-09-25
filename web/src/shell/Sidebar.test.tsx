import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { ToastProvider } from "../ui/Toast";
import Sidebar from "./Sidebar";
import type { SidebarMode } from "./useSidebar";

vi.mock("../auth/pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/pageNavigation")>()),
  reloadPage: vi.fn(),
}));

function renderSidebar({
  audience = "owner",
  mode = "drawer",
  path = "/",
  onClose = vi.fn(),
}: {
  audience?: Audience;
  mode?: SidebarMode;
  path?: string;
  onClose?: () => void;
} = {}) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AudienceProvider audience={audience}>
        <ToastProvider>
          <Sidebar mode={mode} open onClose={onClose} />
        </ToastProvider>
      </AudienceProvider>
    </MemoryRouter>,
  );
}

function accountNav() {
  return screen.getByRole("navigation", { name: "アカウントと設定" });
}

describe("Sidebar", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(reloadPage).mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("閉じたドロワーをフォーカス順と支援技術から外す", () => {
    render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <ToastProvider>
            <Sidebar mode="drawer" open={false} onClose={vi.fn()} />
          </ToastProvider>
        </AudienceProvider>
      </MemoryRouter>,
    );

    const sidebar = document.querySelector("aside");
    expect(sidebar?.hasAttribute("inert")).toBe(true);
    expect(sidebar?.getAttribute("aria-hidden")).toBe("true");
  });

  it("設定をサイドバー末尾の実リンクとして表示してdrawerを閉じる", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderSidebar({ onClose });

    const sidebar = screen.getByRole("complementary", {
      name: "メインナビゲーション",
    });
    const settings = within(sidebar).getByRole("link", { name: "設定" });

    await user.click(settings);

    expect(settings.getAttribute("href")).toBe("/settings");
    expect(onClose).toHaveBeenCalledOnce();
    expect(settings.getAttribute("aria-current")).toBe("page");
  });

  it.each<SidebarMode>(["expanded", "rail", "drawer"])(
    "所有者の下段は「設定」→「ログアウト」の順で、ログインを置かない（%s）",
    (mode) => {
      renderSidebar({ mode });
      const nav = accountNav();
      const names = Array.from(nav.querySelectorAll("a, button")).map(
        (element) => element.textContent,
      );
      expect(names).toEqual(["設定", "ログアウト"]);
    },
  );

  it.each<SidebarMode>(["expanded", "rail", "drawer"])(
    "ゲストの下段は今の URL へ戻るログインだけにする（%s）",
    (mode) => {
      renderSidebar({ audience: "guest", mode, path: "/folders/3/A%20B?query=x" });
      const nav = accountNav();
      const login = within(nav).getByRole("link", { name: "ログイン" });
      expect(login.getAttribute("href")).toBe(
        `/login?next=${encodeURIComponent("/folders/3/A%20B?query=x")}`,
      );
      expect(within(nav).queryByRole("link", { name: "設定" })).toBeNull();
      expect(within(nav).queryByRole("button", { name: "ログアウト" })).toBeNull();
    },
  );

  it("ログアウトは送信中に押せず、204 で今の URL をページごと読み直す", async () => {
    const user = userEvent.setup();
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    renderSidebar();

    await user.click(screen.getByRole("button", { name: "ログアウト" }));
    const pending = screen.getByRole("button", { name: "ログアウト中…" });
    expect((pending as HTMLButtonElement).disabled).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/logout", { method: "POST" });

    finish(new Response(null, { status: 204 }));
    await waitFor(() => expect(reloadPage).toHaveBeenCalledOnce());
  });

  it("ログアウトに失敗したらトーストを出して項目を戻す", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(new Response(null, { status: 500 }));
    renderSidebar();

    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    expect(await screen.findByText("ログアウトできませんでした")).toBeDefined();
    const entry = screen.getByRole("button", { name: "ログアウト" });
    expect((entry as HTMLButtonElement).disabled).toBe(false);
    expect(reloadPage).not.toHaveBeenCalled();
  });
});
