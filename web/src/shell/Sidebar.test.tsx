import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { ToastProvider } from "../ui/legacy/Toast";
import { SidebarProvider, SidebarTrigger } from "../ui/sidebar";
import { TooltipProvider } from "../ui/tooltip";
import Sidebar from "./Sidebar";

vi.mock("../auth/pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/pageNavigation")>()),
  reloadPage: vi.fn(),
}));

type SidebarMode = "expanded" | "rail" | "drawer";

/** stubWidth は、ドロワーの幅（639px 以下）かどうかを matchMedia で決める。 */
function stubWidth(phone: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: phone && query.includes("639px"),
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

async function renderSidebar({
  audience = "owner",
  mode = "drawer",
  path = "/",
}: {
  audience?: Audience;
  mode?: SidebarMode;
  path?: string;
} = {}) {
  stubWidth(mode === "drawer");
  const result = render(
    <MemoryRouter initialEntries={[path]}>
      <AudienceProvider audience={audience}>
        <ToastProvider>
          <TooltipProvider>
            <SidebarProvider defaultOpen={mode !== "rail"}>
              <SidebarTrigger />
              <Sidebar />
            </SidebarProvider>
          </TooltipProvider>
        </ToastProvider>
      </AudienceProvider>
    </MemoryRouter>,
  );
  if (mode === "drawer") {
    await userEvent.click(screen.getByRole("button", { name: /Toggle sidebar/ }));
  }
  return result;
}

function accountNav() {
  return screen.getByRole("navigation", { name: "Account and settings" });
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

  it("閉じたドロワーは描かず、フォーカス順と支援技術に残さない", () => {
    stubWidth(true);
    render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <ToastProvider>
            <SidebarProvider>
              <Sidebar />
            </SidebarProvider>
          </ToastProvider>
        </AudienceProvider>
      </MemoryRouter>,
    );

    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
  });

  it("設定をサイドバー末尾の実リンクとして表示してdrawerを閉じる", async () => {
    const user = userEvent.setup();
    await renderSidebar();

    const sidebar = screen.getByRole("complementary", {
      name: "Main navigation",
    });
    const settings = within(sidebar).getByRole("link", { name: "Settings" });
    expect(settings.getAttribute("href")).toBe("/settings");

    await user.click(settings);

    await waitFor(() =>
      expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull(),
    );
  });

  it("今いる画面の項目を選択中にする", async () => {
    await renderSidebar({ mode: "expanded", path: "/settings" });
    const settings = screen.getByRole("link", { name: "Settings" });
    expect(settings.getAttribute("aria-current")).toBe("page");
    expect(settings.getAttribute("data-active")).toBe("true");
  });

  it.each<SidebarMode>(["expanded", "rail", "drawer"])(
    "所有者の下段は「設定」→「ログアウト」の順で、ログインを置かない（%s）",
    async (mode) => {
      await renderSidebar({ mode });
      const nav = accountNav();
      const names = Array.from(nav.querySelectorAll("a, button")).map(
        (element) => element.textContent,
      );
      expect(names).toEqual(["Settings", "Sign out"]);
    },
  );

  it.each<SidebarMode>(["expanded", "rail", "drawer"])(
    "ゲストの下段は今の URL へ戻るログインだけにする（%s）",
    async (mode) => {
      await renderSidebar({ audience: "guest", mode, path: "/folders/3/A%20B?query=x" });
      const nav = accountNav();
      const login = within(nav).getByRole("link", { name: "Sign in" });
      expect(login.getAttribute("href")).toBe(
        `/login?next=${encodeURIComponent("/folders/3/A%20B?query=x")}`,
      );
      expect(within(nav).queryByRole("link", { name: "Settings" })).toBeNull();
      expect(within(nav).queryByRole("button", { name: "Sign out" })).toBeNull();
    },
  );

  it("ログアウトは送信中に押せず、204 で今の URL をページごと読み直す", async () => {
    const user = userEvent.setup();
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    await renderSidebar();

    await user.click(screen.getByRole("button", { name: "Sign out" }));
    const pending = screen.getByRole("button", { name: "Signing out…" });
    expect((pending as HTMLButtonElement).disabled).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/logout", { method: "POST" });

    finish(new Response(null, { status: 204 }));
    await waitFor(() => expect(reloadPage).toHaveBeenCalledOnce());
  });

  it("ログアウトに失敗したらトーストを出して項目を戻す", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(new Response(null, { status: 500 }));
    await renderSidebar();

    await user.click(screen.getByRole("button", { name: "Sign out" }));

    expect(await screen.findByText("Couldn't sign out")).toBeDefined();
    const entry = screen.getByRole("button", { name: "Sign out" });
    expect((entry as HTMLButtonElement).disabled).toBe(false);
    expect(reloadPage).not.toHaveBeenCalled();
  });
  it("所有者の上段は「タグ」の直後に「Duplicates」を置き、ゲストには出さない", async () => {
    const { unmount } = await renderSidebar({ mode: "expanded" });
    const main = screen.getByRole("complementary", { name: "Main navigation" });
    const names = within(main)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(names.slice(0, 4)).toEqual(["Library", "Folders", "Tags", "Duplicates"]);
    expect(
      within(main).getByRole("link", { name: "Duplicates" }).getAttribute("href"),
    ).toBe("/duplicates");
    unmount();

    await renderSidebar({ audience: "guest", mode: "expanded" });
    expect(screen.queryByRole("link", { name: "Duplicates" })).toBeNull();
  });

  it.each<Audience>(["owner", "guest"])(
    "疑似ロケールで %s のサイドバーはカタログの文言だけを描く",
    async (audience) => {
      enablePseudoLocale();
      await renderSidebar({ audience });
      expectCatalogTextOnly(screen.getByRole("complementary"));
    },
  );

  it("疑似ロケールでログアウトの送信中と失敗はカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    let fail: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (fail = resolve)));
    await renderSidebar();
    await user.click(screen.getByRole("button", { name: /Sign out/ }));
    await screen.findByRole("button", { name: /Signing out/ });
    expectCatalogTextOnly(document.body);
    fail(new Response(null, { status: 500 }));
    await screen.findByText(/Couldn't sign out/);
    expectCatalogTextOnly(document.body);
  });
});
