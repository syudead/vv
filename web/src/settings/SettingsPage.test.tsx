import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { MediaFolder } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import { ScanProvider } from "../shell/ScanProvider";
import { OwnerAudience } from "../testing/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import SettingsPage from "./SettingsPage";

function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function folder(id: number, path: string, version = 1): MediaFolder {
  return {
    id,
    path,
    version,
    createdAt: "2026-09-21T00:00:00Z",
    updatedAt: "2026-09-21T00:00:00Z",
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/settings"]}>
      <TooltipProvider>
        <ToastProvider>
          <OwnerAudience>
            <ScanProvider>
              <ScanNoticeProvider>
                <AppShell>
                  <SettingsPage />
                </AppShell>
              </ScanNoticeProvider>
            </ScanProvider>
          </OwnerAudience>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("SettingsPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it("0件を明示し、path入力とbulk保存を置かず、取り込みを無効にする", async () => {
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([]));
      return Promise.resolve(json({}, 404));
    });

    renderPage();

    expect(
      await screen.findByRole("heading", { level: 1, name: "Settings" }),
    ).toBeDefined();
    expect(await screen.findByText("No media folders yet")).toBeDefined();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("button", { name: /保存/ })).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Add a media folder in Settings first" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("pickerを親子移動して1件追加し、自動取り込みを始めない", async () => {
    const created = folder(7, "/srv/media");
    let createdOnServer = false;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders" && init?.method === "POST") {
        createdOnServer = true;
        return Promise.resolve(json(created, 201));
      }
      if (url === "/api/media-folders") {
        return Promise.resolve(json(createdOnServer ? [created] : []));
      }
      if (url === "/api/directories") {
        return Promise.resolve(
          json({ parentPath: null, directories: [{ name: "/", path: "/" }] }),
        );
      }
      if (url === "/api/directories?path=%2F") {
        return Promise.resolve(
          json({
            currentPath: "/",
            parentPath: null,
            directories: [{ name: "srv", path: "/srv" }],
          }),
        );
      }
      if (url === "/api/directories?path=%2Fsrv") {
        return Promise.resolve(
          json({
            currentPath: "/srv",
            parentPath: "/",
            directories: [{ name: "media", path: "/srv/media" }],
          }),
        );
      }
      if (url === "/api/directories?path=%2Fsrv%2Fmedia") {
        return Promise.resolve(
          json({ currentPath: "/srv/media", parentPath: "/srv", directories: [] }),
        );
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Add folder" }));
    const dialog = await screen.findByRole("dialog", { name: "Add media folder" });
    expect(
      within(dialog)
        .getByRole("button", { name: "Add this folder" })
        .hasAttribute("disabled"),
    ).toBe(true);
    const root = await within(dialog).findByRole("button", { name: "/" });
    root.focus();
    await user.keyboard("{Enter}");
    const srv = await within(dialog).findByRole("button", { name: "srv" });
    await waitFor(() => expect(document.activeElement).toBe(srv));
    expect(
      within(dialog)
        .getByRole("button", { name: "Add this folder" })
        .hasAttribute("disabled"),
    ).toBe(false);
    await user.keyboard("{ArrowRight}");
    const media = await within(dialog).findByRole("button", { name: "media" });
    await waitFor(() => expect(document.activeElement).toBe(media));
    await user.click(media);
    await user.click(within(dialog).getByRole("button", { name: "Add this folder" }));

    expect(await screen.findByText("/srv/media")).toBeDefined();
    const createCall = fetchMock.mock.calls.find(
      ([input, init]) =>
        String(input) === "/api/media-folders" && init?.method === "POST",
    );
    expect(createCall?.[1]?.body).toBe(JSON.stringify({ path: "/srv/media" }));
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) => String(input) === "/api/scans" && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("複数folderを表示し、変更と削除の影響を確認してcancelできる", async () => {
    const folders = [
      folder(1, "/media/one"),
      folder(2, "/very/long/日本語/動画保管場所"),
    ];
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json(folders));
      if (url === "/api/directories?path=%2Fmedia%2Fone") {
        return Promise.resolve(
          json({
            currentPath: "/media/one",
            parentPath: "/media",
            directories: [{ name: "replacement", path: "/media/one/replacement" }],
          }),
        );
      }
      if (url === "/api/directories?path=%2Fmedia%2Fone%2Freplacement") {
        return Promise.resolve(
          json({
            currentPath: "/media/one/replacement",
            parentPath: "/media/one",
            directories: [],
          }),
        );
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("/very/long/日本語/動画保管場所")).toBeDefined();
    await user.click(screen.getAllByRole("button", { name: "Change folder" })[0]!);
    let dialog = await screen.findByRole("dialog", { name: "Change media folder" });
    await user.click(await within(dialog).findByRole("button", { name: "replacement" }));
    await user.click(
      within(dialog).getByRole("button", { name: "Change to this folder" }),
    );
    dialog = await screen.findByRole("dialog", { name: "Change this folder?" });
    expect(
      within(dialog).getByText(/Playback positions and watched status are kept/),
    ).toBeDefined();
    const back = within(dialog).getByRole("button", { name: "Back" });
    await waitFor(() => expect(document.activeElement).toBe(back));
    await user.click(back);
    expect(
      await screen.findByRole("dialog", { name: "Change media folder" }),
    ).toBeDefined();
    expect(
      Array.from(document.body.children).some((element) => element.hasAttribute("inert")),
    ).toBe(true);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getAllByRole("button", { name: "Remove folder" })[1]!);
    dialog = await screen.findByRole("dialog", { name: "Remove this folder?" });
    expect(
      within(dialog).getByText(/Videos that are also in another media folder stay/),
    ).toBeDefined();
    const cancel = within(dialog).getByRole("button", { name: "Cancel" });
    await waitFor(() => expect(document.activeElement).toBe(cancel));
    await user.click(cancel);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("/very/long/日本語/動画保管場所")).toBeDefined();
  });

  it("一覧取得失敗を0件扱いせず再試行できる", async () => {
    let attempts = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") {
        attempts += 1;
        return Promise.resolve(
          attempts === 1
            ? json({ code: "internal", message: "DBを読めません" }, 500)
            : json([]),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    renderPage();

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Something went wrong on the server.",
    );
    expect(screen.queryByText("No media folders yet")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Add folder" }).hasAttribute("disabled"),
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("No media folders yet")).toBeDefined();
  });

  it("削除は対象id/versionだけへ送り、残る行へfocusを移す", async () => {
    const folders = [folder(1, "/media/one", 3), folder(2, "/media/two", 5)];
    let deleted = false;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") {
        return Promise.resolve(json(deleted ? [folders[1]] : folders));
      }
      if (url === "/api/media-folders/1?version=3" && init?.method === "DELETE") {
        deleted = true;
        return Promise.resolve(json({}, 204));
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("/media/two");
    await user.click(screen.getAllByRole("button", { name: "Remove folder" })[0]!);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }),
    );

    await waitFor(() => expect(screen.queryByText("/media/one")).toBeNull());
    expect(screen.getByText("/media/two")).toBeDefined();
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          String(input) === "/api/media-folders/1?version=3" && init?.method === "DELETE",
      ),
    ).toBe(true);
    await waitFor(() =>
      expect(
        (document.activeElement as HTMLElement | null)?.getAttribute("aria-label"),
      ).toBe("Change folder"),
    );
  });

  it("削除後に一覧を再取得して別タブで追加されたfolder数を反映する", async () => {
    const original = folder(1, "/media/original", 3);
    const concurrent = folder(2, "/media/concurrent", 1);
    let deleted = false;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") {
        return Promise.resolve(json(deleted ? [concurrent] : [original]));
      }
      if (url === "/api/media-folders/1?version=3" && init?.method === "DELETE") {
        deleted = true;
        return Promise.resolve(json({}, 204));
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("/media/original");
    await user.click(screen.getByRole("button", { name: "Remove folder" }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }),
    );

    expect(await screen.findByText("/media/concurrent")).toBeDefined();
    expect(screen.queryByText("/media/original")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Refresh library" }).hasAttribute("disabled"),
    ).toBe(false);
  });

  it("削除後の一覧再取得に失敗しても削除済み行とdialogを戻さない", async () => {
    const original = folder(1, "/media/original", 3);
    let deleted = false;
    let deleteRequests = 0;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") {
        return Promise.resolve(
          deleted
            ? json({ code: "internal", message: "一覧を再取得できません" }, 500)
            : json([original]),
        );
      }
      if (url === "/api/media-folders/1?version=3" && init?.method === "DELETE") {
        deleted = true;
        deleteRequests += 1;
        return Promise.resolve(json({}, 204));
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("/media/original");
    await user.click(screen.getByRole("button", { name: "Remove folder" }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }),
    );

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Something went wrong on the server.",
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText("/media/original")).toBeNull();
    expect(deleteRequests).toBe(1);
  });

  it("別画面で削除済みなら一覧を再取得して古い行を除く", async () => {
    const stale = folder(1, "/media/stale", 3);
    const remaining = folder(2, "/media/remaining", 5);
    let listRequests = 0;
    let deletedElsewhere = false;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") {
        listRequests += 1;
        return Promise.resolve(json(deletedElsewhere ? [remaining] : [stale, remaining]));
      }
      if (url === "/api/media-folders/1?version=3" && init?.method === "DELETE") {
        deletedElsewhere = true;
        return Promise.resolve(
          json(
            { code: "media_folder_not_found", message: "フォルダが見つかりません" },
            404,
          ),
        );
      }
      throw new Error(`unexpected request: ${url}`);
    });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("/media/remaining");
    await user.click(screen.getAllByRole("button", { name: "Remove folder" })[0]!);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }),
    );

    await waitFor(() => expect(screen.queryByText("/media/stale")).toBeNull());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("/media/remaining")).toBeDefined();
    expect(listRequests).toBeGreaterThan(1);
    await waitFor(() =>
      expect(
        (document.activeElement as HTMLElement | null)?.getAttribute("aria-label"),
      ).toBe("Change folder"),
    );
  });
  it("存在しないフォルダを開くと、API エラーの英語の説明を出す", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url === "/api/directories") {
        return Promise.resolve(
          json(
            {
              code: "not_found",
              reason: "directory_not_found",
              message: "Directory not found.",
            },
            404,
          ),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Add folder" }));
    const dialog = await screen.findByRole("dialog", { name: "Add media folder" });
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "That folder wasn't found.",
    );
    expect(within(dialog).getByRole("button", { name: "Go to the top" })).toBeDefined();
  });

  it("重なるメディアフォルダの追加は、API エラーの英語の説明を出す", async () => {
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders" && init?.method === "POST") {
        return Promise.resolve(
          json(
            {
              code: "overlapping_media_directories",
              message: "Media folders overlap.",
            },
            409,
          ),
        );
      }
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url.startsWith("/api/directories")) {
        return Promise.resolve(
          json({ currentPath: "/srv", parentPath: "/", directories: [] }),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Add folder" }));
    const dialog = await screen.findByRole("dialog", { name: "Add media folder" });
    const add = await within(dialog).findByRole("button", { name: "Add this folder" });
    await waitFor(() => expect(add.hasAttribute("disabled")).toBe(false));
    await user.click(add);
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "That folder overlaps a media folder that's already added.",
    );
  });

  it("別の操作でメディアフォルダが変わったら、reason の英語の説明を出す", async () => {
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders" && init?.method === "POST") {
        return Promise.resolve(
          json(
            {
              code: "conflict",
              reason: "media_folders_changed",
              message: "Media folders changed.",
            },
            409,
          ),
        );
      }
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url.startsWith("/api/directories")) {
        return Promise.resolve(
          json({ currentPath: "/srv", parentPath: "/", directories: [] }),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Add folder" }));
    const dialog = await screen.findByRole("dialog", { name: "Add media folder" });
    const add = await within(dialog).findByRole("button", { name: "Add this folder" });
    await waitFor(() => expect(add.hasAttribute("disabled")).toBe(false));
    await user.click(add);
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "The media folders were changed elsewhere. Reload and try again.",
    );
  });

  describe("疑似ロケール", () => {
    const paths = ["/srv/media", "/very/long/日本語/動画保管場所", "/other", "/", "srv"];

    function serve(folders: MediaFolder[], listFails = false) {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        if (url === "/api/media-folders") {
          return Promise.resolve(
            listFails
              ? json({ code: "internal", message: "Internal error." }, 500)
              : json(folders),
          );
        }
        if (url === "/api/directories") {
          return Promise.resolve(
            json({ parentPath: null, directories: [{ name: "srv", path: "/srv" }] }),
          );
        }
        if (url.startsWith("/api/directories")) {
          return Promise.resolve(
            json({ currentPath: "/other", parentPath: "/", directories: [] }),
          );
        }
        return Promise.resolve(json({}, 404));
      });
    }

    it("空の状態と上部の枠・サイドバーはカタログの文言だけを描く", async () => {
      enablePseudoLocale();
      serve([]);
      const { container } = renderPage();
      await screen.findByText(/No media folders yet/);
      expectCatalogTextOnly(container, paths);
    });

    it("一覧の取得の失敗はカタログの文言だけを描く", async () => {
      enablePseudoLocale();
      serve([], true);
      const { container } = renderPage();
      await screen.findByText(/Couldn't load the media folders/);
      expectCatalogTextOnly(container, paths);
    });

    it("一覧・フォルダ選択・変更の確認・削除の確認はカタログの文言だけを描く", async () => {
      enablePseudoLocale();
      serve([folder(1, "/srv/media"), folder(2, "/very/long/日本語/動画保管場所")]);
      const user = userEvent.setup();
      const { container } = renderPage();
      await screen.findByText("/srv/media");
      expectCatalogTextOnly(container, paths);

      await user.click(screen.getByRole("button", { name: /Add folder/ }));
      const picker = await screen.findByRole("dialog");
      await within(picker).findByRole("button", { name: "srv" });
      expectCatalogTextOnly(document.body, paths);
      await user.click(within(picker).getByRole("button", { name: /Cancel/ }));
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

      await user.click(screen.getAllByRole("button", { name: /Change folder/ })[1]!);
      const change = await screen.findByRole("dialog");
      const submit = await within(change).findByRole("button", {
        name: /Change to this folder/,
      });
      await waitFor(() => expect(submit.hasAttribute("disabled")).toBe(false));
      await user.click(submit);
      await within(screen.getByRole("dialog")).findByText(/Before/);
      expectCatalogTextOnly(document.body, paths);
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", { name: /Back/ }),
      );
      await user.click(
        within(await screen.findByRole("dialog")).getByRole("button", { name: /Cancel/ }),
      );
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

      await user.click(screen.getAllByRole("button", { name: /Remove folder/ })[0]!);
      await screen.findByRole("dialog");
      expectCatalogTextOnly(document.body, paths);
    });
  });
});
