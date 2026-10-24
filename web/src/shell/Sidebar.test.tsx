import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { SidebarProvider, SidebarTrigger } from "../ui/shadcn/sidebar";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import Sidebar from "./Sidebar";

vi.mock("../auth/pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/pageNavigation")>()),
  reloadPage: vi.fn(),
}));

type SidebarMode = "rail" | "drawer";

function LocationProbe() {
  const location = useLocation();
  const state = location.state as { from?: string } | null;
  return (
    <output aria-label="location">{`${location.pathname} from ${state?.from ?? ""}`}</output>
  );
}

function stubWidth(phone: boolean) {
  vi.stubGlobal(
    "matchMedia",
    (query: string) =>
      ({
        matches: phone && query.includes("639px"),
        media: query,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }) as unknown as MediaQueryList,
  );
}

function renderSidebar({
  audience = "owner",
  mode = "drawer",
  path = "/",
  probe = false,
}: {
  audience?: Audience;
  mode?: SidebarMode;
  path?: string;
  probe?: boolean;
} = {}) {
  stubWidth(mode === "drawer");
  const result = render(
    <MemoryRouter initialEntries={[path]}>
      <AudienceProvider audience={audience}>
        <ToastProvider>
          <TooltipProvider>
            <SidebarProvider open={false} onOpenChange={() => undefined}>
              <SidebarTrigger />
              <Sidebar />
            </SidebarProvider>
          </TooltipProvider>
        </ToastProvider>
      </AudienceProvider>
      {probe && <LocationProbe />}
    </MemoryRouter>,
  );
  if (mode === "drawer") {
    act(() => screen.getByRole("button", { name: /Toggle sidebar/ }).click());
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
    window.history.replaceState({}, "", "/");
    vi.mocked(reloadPage).mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("閉じたドロワーは描かず、☰ で開く", () => {
    stubWidth(true);
    render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <ToastProvider>
            <TooltipProvider>
              <SidebarProvider>
                <SidebarTrigger />
                <Sidebar />
              </SidebarProvider>
            </TooltipProvider>
          </ToastProvider>
        </AudienceProvider>
      </MemoryRouter>,
    );

    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
    act(() => screen.getByRole("button", { name: "Toggle sidebar" }).click());
    expect(screen.getByRole("dialog").textContent).toContain("Home");
  });

  it.each([true, false])(
    "keyboardShortcut=%s のときだけ Ctrl+B を奪う",
    (keyboardShortcut) => {
      stubWidth(false);
      render(
        <SidebarProvider open={false} keyboardShortcut={keyboardShortcut}>
          <div />
        </SidebarProvider>,
      );
      const event = new KeyboardEvent("keydown", {
        key: "b",
        ctrlKey: true,
        cancelable: true,
      });
      act(() => {
        window.dispatchEvent(event);
      });
      expect(event.defaultPrevented).toBe(keyboardShortcut);
    },
  );

  it("ドロワーとその背面の幕はトップバーの下から始める", () => {
    renderSidebar();

    // Use Sheet's data-[side=left] selector to override its top position.
    const drawer = screen.getByRole("dialog");
    expect(drawer.getAttribute("data-side")).toBe("left");
    expect(drawer.className).toContain("data-[side=left]:top-navbar");
    expect(drawer.className).toContain("data-[side=left]:h-auto");
    expect(drawer.className).not.toContain("data-[side=left]:h-full");
    const overlay = document.querySelector('[data-slot="sheet-overlay"]');
    expect(overlay?.className.split(" ")).toContain("top-navbar");
  });

  it("設定をサイドバー末尾の実リンクとして表示してdrawerを閉じる", async () => {
    const user = userEvent.setup();
    renderSidebar();

    const sidebar = screen.getByRole("complementary", {
      name: "Main navigation",
    });
    const settings = within(sidebar).getByRole("link", { name: "Settings" });

    await user.click(settings);

    expect(settings.getAttribute("href")).toBe("/settings");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it.each<SidebarMode>(["rail", "drawer"])(
    "所有者の下段は「設定」→「ログアウト」の順で、ログインを置かない（%s）",
    (mode) => {
      renderSidebar({ mode });
      const nav = accountNav();
      const names = Array.from(nav.querySelectorAll("a, button")).map(
        (element) => element.textContent,
      );
      expect(nav.closest("aside")?.getAttribute("aria-label")).toBe("Main navigation");
      expect(names).toEqual(["Settings", "Sign out"]);
    },
  );

  it.each<SidebarMode>(["rail", "drawer"])(
    "ゲストの下段は今の URL へ戻るログインだけにする（%s）",
    (mode) => {
      renderSidebar({ audience: "guest", mode, path: "/folders/3/A%20B?query=x" });
      const nav = accountNav();
      const login = within(nav).getByRole("link", { name: "Sign in" });
      expect(login.getAttribute("href")).toBe(
        `/login?next=${encodeURIComponent("/folders/3/A%20B?query=x")}`,
      );
      expect(within(nav).queryByRole("link", { name: "Settings" })).toBeNull();
      expect(within(nav).queryByRole("button", { name: "Sign out" })).toBeNull();
    },
  );

  it("所有者だけが上段の下端の「New virtual video」から題名を入れて作り、閉じたドロワーの元のページを戻り先に開く", async () => {
    const user = userEvent.setup();
    const created: unknown[] = [];
    fetchMock.mockImplementation((input, init) => {
      if (String(input) !== "/api/virtual-videos") return new Promise(() => undefined);
      created.push(JSON.parse(String(init?.body)));
      return Promise.resolve(
        new Response(JSON.stringify({ id: 7, virtual: true, title: "京都の記録" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
    });
    renderSidebar({ path: "/folders/3?query=x", probe: true });
    const main = screen.getByRole("complementary", { name: "Main navigation" });
    const entry = within(main).getByRole("button", { name: "New virtual video" });
    expect(entry.closest("nav")).toBeNull();
    expect(
      entry.compareDocumentPosition(
        within(main).getByRole("link", { name: "Activity" }),
      ) & Node.DOCUMENT_POSITION_PRECEDING,
    ).toBeTruthy();
    expect(
      entry.compareDocumentPosition(accountNav()) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    await user.click(entry);
    expect(screen.queryByRole("complementary", { name: "Main navigation" })).toBeNull();
    const dialog = await screen.findByRole("dialog", { name: "New virtual video" });
    const field = within(dialog).getByRole("textbox", { name: "Title" });
    await user.type(field, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Enter a title.")).toBeDefined();
    expect(created).toHaveLength(0);

    await user.clear(field);
    await user.type(field, " 京都の記録 {Enter}");
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "location" }).textContent).toBe(
        "/videos/7 from /folders/3?query=x",
      ),
    );
    expect(created).toEqual([{ title: "京都の記録" }]);
  });

  it.each<SidebarMode>(["rail", "drawer"])(
    "ゲストには「New virtual video」を出さない（%s）",
    (mode) => {
      renderSidebar({ audience: "guest", mode });
      expect(screen.queryByRole("button", { name: "New virtual video" })).toBeNull();
    },
  );

  it("ログアウトは送信中に押せず、204 で今の URL をページごと読み直す", async () => {
    const user = userEvent.setup();
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    renderSidebar();

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
    renderSidebar();

    await user.click(screen.getByRole("button", { name: "Sign out" }));

    expect(await screen.findByText("Couldn't sign out")).toBeDefined();
    const entry = screen.getByRole("button", { name: "Sign out" });
    expect((entry as HTMLButtonElement).disabled).toBe(false);
    expect(reloadPage).not.toHaveBeenCalled();
  });
  it("所有者の上段は「Folders」の後に「History」と「Playlists」、「Tags」の後に「People」「Organizations」「Duplicates」を置き、ゲストには出さない", () => {
    const { unmount } = renderSidebar({ mode: "rail" });
    const main = screen.getByRole("complementary", { name: "Main navigation" });
    const names = within(main)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(names.slice(0, 8)).toEqual([
      "Home",
      "Folders",
      "History",
      "Playlists",
      "Tags",
      "People",
      "Organizations",
      "Duplicates",
    ]);
    expect(within(main).getByRole("link", { name: "Home" }).getAttribute("href")).toBe(
      "/",
    );
    expect(within(main).queryByRole("link", { name: "Library" })).toBeNull();
    expect(within(main).getByRole("link", { name: "People" }).getAttribute("href")).toBe(
      "/people",
    );
    expect(
      within(main).getByRole("link", { name: "Organizations" }).getAttribute("href"),
    ).toBe("/organizations");
    expect(
      within(main).getByRole("link", { name: "Playlists" }).getAttribute("href"),
    ).toBe("/playlists");
    expect(within(main).getByRole("link", { name: "History" }).getAttribute("href")).toBe(
      "/history",
    );
    expect(
      within(main).getByRole("link", { name: "Duplicates" }).getAttribute("href"),
    ).toBe("/duplicates");
    unmount();

    renderSidebar({ audience: "guest", mode: "rail" });
    const guestMain = screen.getByRole("complementary", { name: "Main navigation" });
    expect(
      within(guestMain)
        .getAllByRole("link")
        .map((link) => link.textContent)
        .slice(0, 2),
    ).toEqual(["Home", "Folders"]);
    expect(screen.queryByRole("link", { name: "History" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Playlists" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Duplicates" })).toBeNull();
  });

  it("所有者の上段の最後に「Activity」を置き、要対応の数をレールに出して activity の知らせで読み直す", async () => {
    installFakeEventSource();
    let attention = 2;
    fetchMock.mockImplementation((input) => {
      expect(String(input)).toMatch(/^\/api\/activity\?limit=1$/);
      return Promise.resolve(
        new Response(
          JSON.stringify({
            items: [],
            counts: { running: 4, attention, done: 9 },
            features: {},
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    });
    renderSidebar({ mode: "rail" });
    const main = screen.getByRole("complementary", { name: "Main navigation" });
    const [screens] = within(main).getAllByRole("navigation");
    const links = within(screens!).getAllByRole("link");
    expect(links.at(-1)?.textContent).toBe("Activity");
    expect(links.at(-1)?.getAttribute("href")).toBe("/activity");
    expect(await within(main).findByText("2 need attention")).toBeDefined();

    attention = 120;
    await emitServerEvent("activity");
    expect(await within(main).findByText("120 need attention")).toBeDefined();
    expect(within(main).getByText("99+")).toBeDefined();

    attention = 0;
    await emitServerEvent("activity");
    await waitFor(() => expect(within(main).queryByText("99+")).toBeNull());
  });

  it("ゲストには「Activity」を出さず、件数も読まない", () => {
    installFakeEventSource();
    renderSidebar({ audience: "guest", mode: "rail" });
    expect(screen.queryByRole("link", { name: "Activity" })).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each<Audience>(["owner", "guest"])(
    "疑似ロケールで %s のサイドバーはカタログの文言だけを描く",
    (audience) => {
      enablePseudoLocale();
      renderSidebar({ audience });
      expectCatalogTextOnly(document.body);
    },
  );

  it("疑似ロケールでログアウトの送信中と失敗はカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    let fail: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (fail = resolve)));
    renderSidebar();
    await user.click(screen.getByRole("button", { name: /Sign out/ }));
    await screen.findByRole("button", { name: /Signing out/ });
    expectCatalogTextOnly(document.body);
    fail(new Response(null, { status: 500 }));
    await screen.findByText(/Couldn't sign out/);
    expectCatalogTextOnly(document.body);
  });
});
