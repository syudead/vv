import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  currentLocation,
  folder,
  historyBack,
  installSettingsServer,
  json,
  renderSettings,
} from "../testing/settingsPage";

// 設定の 8 つのページ、その一覧とアドレス。
// アドレスの決め方そのものは settingsPages.test.ts が確かめる。

/** sectionHeadings はページの本文にある節の見出しである（一覧の見出しは含まない）。 */
function sectionHeadings(): string[] {
  const body = document.querySelector("[data-slot='settings-page-body']");
  if (body === null) return [];
  return within(body as HTMLElement)
    .queryAllByRole("heading", { level: 2 })
    .map((heading) => heading.textContent ?? "");
}

function pageList(): HTMLElement {
  return screen.getByRole("navigation", { name: "Settings" });
}

const pages: [string, string, string[]][] = [
  ["/settings/scan", "Scan", ["Scan status", "Auto-import"]],
  ["/settings/library", "Library", ["Media folders", "Auto-import", "Duplicate judging"]],
  ["/settings/auto-tagging", "Auto-tagging", ["Progress", "Classifier"]],
  ["/settings/connections", "Connections", ["Ollama", "ComfyUI"]],
  ["/settings/plugins", "Plugins", ["Work-info plugins"]],
  ["/settings/conversion", "Video conversion", ["Encoder"]],
  ["/settings/network", "Network", ["Local network"]],
  ["/settings/api", "API", ["API tokens"]],
];

describe("SettingsPage: pages", () => {
  const server = installSettingsServer();

  beforeEach(() => {
    server.fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([]));
      return Promise.resolve(json({}, 404));
    });
  });

  it.each(pages)("%s renders only its own sections", async (path, title, headings) => {
    renderSettings(path);

    expect(await screen.findByRole("heading", { level: 1, name: title })).toBeDefined();
    await waitFor(() => expect(sectionHeadings()).toEqual(headings));
    expect(document.title).toBe(title);
  });

  it("reads only the resources of the page it shows", async () => {
    renderSettings("/settings/connections");

    await waitFor(() => expect(sectionHeadings()).toEqual(["Ollama", "ComfyUI"]));
    expect(new Set(server.requested)).toEqual(
      new Set(["/api/settings/ollama", "/api/settings/comfyui"]),
    );
  });

  it("marks the current page in the list, and Scan on /settings", async () => {
    const view = renderSettings("/settings/connections");
    const current = await within(pageList()).findByRole("link", { name: "Connections" });
    expect(current.getAttribute("aria-current")).toBe("page");
    const links = within(pageList()).getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual(
      pages.map(([, title]) => title),
    );
    expect(links.filter((link) => link.hasAttribute("aria-current"))).toHaveLength(1);
    view.unmount();

    renderSettings("/settings");
    expect(await screen.findByRole("heading", { level: 1, name: "Scan" })).toBeDefined();
    expect(
      within(pageList()).getByRole("link", { name: "Scan" }).getAttribute("aria-current"),
    ).toBe("page");
    expect(
      document.querySelector("[data-slot='settings-page']")?.getAttribute("data-view"),
    ).toBe("list");
  });

  it("replaces an unknown page with /settings/scan", async () => {
    renderSettings("/settings/nowhere");

    expect(await screen.findByRole("heading", { level: 1, name: "Scan" })).toBeDefined();
    expect(currentLocation()).toBe("/settings/scan");
  });

  it("forwards /settings#comfyui to the Connections page and focuses ComfyUI", async () => {
    renderSettings("/settings#comfyui");

    const heading = await screen.findByRole("heading", { level: 2, name: "ComfyUI" });
    expect(currentLocation()).toBe("/settings/connections#comfyui");
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it("opens /settings/scan#scan-status with Scan status focused", async () => {
    renderSettings("/settings/scan#scan-status");

    const heading = await screen.findByRole("heading", { level: 2, name: "Scan status" });
    expect(screen.getByRole("heading", { level: 1, name: "Scan" })).toBeDefined();
    expect(currentLocation()).toBe("/settings/scan#scan-status");
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it("goes to another page from the list and back to the previous one", async () => {
    const user = userEvent.setup();
    renderSettings("/settings/library");
    await screen.findByRole("heading", { level: 1, name: "Library" });

    await user.click(within(pageList()).getByRole("link", { name: "Plugins" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Plugins" }),
    ).toBeDefined();
    expect(currentLocation()).toBe("/settings/plugins");

    act(() => historyBack());
    expect(
      await screen.findByRole("heading", { level: 1, name: "Library" }),
    ).toBeDefined();
    expect(currentLocation()).toBe("/settings/library");
  });

  it("opens the next page at its top when an entry is pressed", async () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    const user = userEvent.setup();
    renderSettings("/settings/scan");
    await screen.findByRole("heading", { level: 1, name: "Scan" });
    expect(scrollTo).not.toHaveBeenCalled();

    await user.click(within(pageList()).getByRole("link", { name: "Plugins" }));
    await screen.findByRole("heading", { level: 1, name: "Plugins" });
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
    scrollTo.mockRestore();
  });

  it.each(["/settings", "/settings/library"])(
    "marks Settings current in the sidebar on %s",
    async (path) => {
      renderSettings(path);
      await screen.findByRole("heading", { level: 1 });
      const entry = document.querySelector(
        "[data-sidebar='menu-button'][data-active='true']",
      );
      expect(entry?.textContent).toBe("Settings");
    },
  );

  it("says LAN access is in the desktop app when the read answers 404, with the proxy row", async () => {
    renderSettings("/settings/network");

    expect(
      await screen.findByText(
        "LAN access is set in the Windows desktop app. This VVMDM isn't running as the desktop app, so there's no switch here.",
      ),
    ).toBeDefined();
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.getByLabelText("Proxy for plugin requests")).toBeDefined();
  });

  it("enables Scan library on the Scan page after a folder is added on Library", async () => {
    const created = folder(7, "/");
    let createdOnServer = false;
    server.fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders" && init?.method === "POST") {
        createdOnServer = true;
        return Promise.resolve(json(created, 201));
      }
      if (url === "/api/media-folders")
        return Promise.resolve(json(createdOnServer ? [created] : []));
      if (url === "/api/directories")
        return Promise.resolve(
          json({ parentPath: null, directories: [{ name: "/", path: "/" }] }),
        );
      if (url === "/api/directories?path=%2F")
        return Promise.resolve(
          json({ currentPath: "/", parentPath: null, directories: [] }),
        );
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    renderSettings("/settings/scan");

    const blocked = await screen.findByRole("button", { name: "Scan library" });
    await waitFor(() => expect(blocked.hasAttribute("disabled")).toBe(true));
    const reason = await waitFor(() => {
      const element = document.getElementById("scan-start-blocked");
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });
    expect(reason.textContent).toBe("Add a media folder in Library to scan.");
    expect(
      within(reason).getByRole("link", { name: "Library" }).getAttribute("href"),
    ).toBe("/settings/library");

    await user.click(within(reason).getByRole("link", { name: "Library" }));
    await user.click(await screen.findByRole("button", { name: "Add folder" }));
    const dialog = await screen.findByRole("dialog", { name: "Add media folder" });
    await user.click(await within(dialog).findByRole("button", { name: "/" }));
    const add = within(dialog).getByRole("button", { name: "Add this folder" });
    await waitFor(() => expect(add.hasAttribute("disabled")).toBe(false));
    await user.click(add);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(within(pageList()).getByRole("link", { name: "Scan" }));
    const start = await screen.findByRole("button", { name: "Scan library" });
    await waitFor(() => expect(start.hasAttribute("disabled")).toBe(false));
  });
});
