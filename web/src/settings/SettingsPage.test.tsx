import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { MediaFolder } from "../api/client";
import AppShell from "../shell/AppShell";
import { ScanProvider } from "../shell/ScanProvider";
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
          <ScanProvider>
            <AppShell>
              <SettingsPage />
            </AppShell>
          </ScanProvider>
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

    expect(await screen.findByRole("heading", { level: 1, name: "設定" })).toBeDefined();
    expect(await screen.findByText("メディアフォルダが設定されていません")).toBeDefined();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("button", { name: /保存/ })).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "メディアフォルダを設定してください" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("pickerを親子移動して1件追加し、自動取り込みを始めない", async () => {
    const created = folder(7, "/srv/media");
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders" && init?.method === "POST") {
        return Promise.resolve(json(created, 201));
      }
      if (url === "/api/media-folders") return Promise.resolve(json([]));
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

    await user.click(await screen.findByRole("button", { name: "フォルダを追加" }));
    const dialog = await screen.findByRole("dialog", { name: "メディアフォルダを追加" });
    expect(
      within(dialog)
        .getByRole("button", { name: "このフォルダを追加" })
        .hasAttribute("disabled"),
    ).toBe(true);
    const root = await within(dialog).findByRole("button", { name: "/" });
    root.focus();
    await user.keyboard("{Enter}");
    const srv = await within(dialog).findByRole("button", { name: "srv" });
    await waitFor(() => expect(document.activeElement).toBe(srv));
    expect(
      within(dialog)
        .getByRole("button", { name: "このフォルダを追加" })
        .hasAttribute("disabled"),
    ).toBe(false);
    await user.keyboard("{ArrowRight}");
    const media = await within(dialog).findByRole("button", { name: "media" });
    await waitFor(() => expect(document.activeElement).toBe(media));
    await user.click(media);
    await user.click(within(dialog).getByRole("button", { name: "このフォルダを追加" }));

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
    await user.click(screen.getAllByRole("button", { name: "フォルダを変更" })[0]!);
    let dialog = await screen.findByRole("dialog", { name: "メディアフォルダを変更" });
    await user.click(await within(dialog).findByRole("button", { name: "replacement" }));
    await user.click(within(dialog).getByRole("button", { name: "このフォルダに変更" }));
    dialog = await screen.findByRole("dialog", { name: "フォルダの変更を確認" });
    expect(within(dialog).getByText(/再生位置と視聴済み状態は残ります/)).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "戻る" }));
    expect(
      await screen.findByRole("dialog", { name: "メディアフォルダを変更" }),
    ).toBeDefined();
    expect(
      Array.from(document.body.children).some((element) => element.hasAttribute("inert")),
    ).toBe(true);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getAllByRole("button", { name: "フォルダを削除" })[1]!);
    dialog = await screen.findByRole("dialog", { name: "フォルダの削除を確認" });
    expect(
      within(dialog).getByText(/別の登録フォルダにもある動画は残ります/),
    ).toBeDefined();
    const cancel = within(dialog).getByRole("button", { name: "キャンセル" });
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

    expect((await screen.findByRole("alert")).textContent).toContain("DBを読めません");
    expect(screen.queryByText("メディアフォルダが設定されていません")).toBeNull();
    expect(
      screen.getByRole("button", { name: "フォルダを追加" }).hasAttribute("disabled"),
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: "再試行" }));
    expect(await screen.findByText("メディアフォルダが設定されていません")).toBeDefined();
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
    await user.click(screen.getAllByRole("button", { name: "フォルダを削除" })[0]!);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "削除する" }),
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
      ).toBe("フォルダを変更"),
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
    await user.click(screen.getByRole("button", { name: "フォルダを削除" }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "削除する" }),
    );

    expect(await screen.findByText("/media/concurrent")).toBeDefined();
    expect(screen.queryByText("/media/original")).toBeNull();
    expect(
      screen.getByRole("button", { name: "ライブラリを更新" }).hasAttribute("disabled"),
    ).toBe(false);
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
    await user.click(screen.getAllByRole("button", { name: "フォルダを削除" })[0]!);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "削除する" }),
    );

    await waitFor(() => expect(screen.queryByText("/media/stale")).toBeNull());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("/media/remaining")).toBeDefined();
    expect(listRequests).toBeGreaterThan(1);
    await waitFor(() =>
      expect(
        (document.activeElement as HTMLElement | null)?.getAttribute("aria-label"),
      ).toBe("フォルダを変更"),
    );
  });
});
