import { act, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  autoImportSettings as settings,
  installAutoImportServer,
  renderWithScan,
} from "../testing/autoImportServer";
import AutoImportState from "./AutoImportState";
import { useAutoImportSettings } from "./useAutoImportSettings";

/** 監視の状態を追う契機で読み、状態の 1 行と問題を出す。 */
function StateHarness({ reloadToken = 0 }: { reloadToken?: number }) {
  const source = useAutoImportSettings({ reloadToken, followWatch: true });
  return <AutoImportState source={source} />;
}

describe("AutoImportState", () => {
  const server = installAutoImportServer();

  it("says the words of each state", async () => {
    server.current = settings(false);
    const view = renderWithScan(<StateHarness />);
    const off = await screen.findByText(
      /^Off\. Changes are picked up by the next scan\./,
    );
    expect(off.textContent).toBe(
      "Off. Changes are picked up by the next scan. Turn it on in Library.",
    );
    view.unmount();

    server.current = settings(true);
    renderWithScan(<StateHarness />);
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
  });

  it("says to add a media folder when it is on and there is none", async () => {
    server.current = settings(true, { state: "off" });
    renderWithScan(<StateHarness />);

    const link = await screen.findByRole("link", { name: "Library" });
    expect(link.getAttribute("href")).toBe("/settings/library");
    expect(link.closest("p")?.textContent).toBe(
      "Add a media folder in Library to watch it.",
    );
  });

  it("shows the problem with its path for a limited watch, and no button", async () => {
    server.current = settings(true, {
      state: "limited",
      problem: "watch_limit",
      path: "/media/very/deep",
    });
    renderWithScan(<StateHarness />);

    expect(
      await screen.findByText("Watching, but some changes may be missed."),
    ).toBeDefined();
    expect(screen.getByText("Some folders aren't watched")).toBeDefined();
    const path = screen.getByText("/media/very/deep");
    expect(path.tagName).toBe("CODE");
    expect(path.closest("[data-slot='alert']")?.textContent).toContain(
      "fs.inotify.max_user_watches",
    );
    expect(path.closest("[data-slot='alert']")?.querySelector("button")).toBeNull();
  });

  it("shows the problem without a path for lost events", async () => {
    server.current = settings(true, { state: "limited", problem: "events_lost" });
    renderWithScan(<StateHarness />);

    expect(await screen.findByText("Some changes were lost")).toBeDefined();
    expect(
      screen.getByText(/Start a scan under Scan status to pick them up/),
    ).toBeDefined();
  });

  it("shows the lost-events problem without a path placeholder", async () => {
    server.current = settings(true, { state: "limited", problem: "events_lost" });
    renderWithScan(<StateHarness />);

    const alert = await screen.findByRole("note");
    expect(alert.textContent).toContain("pick them up.");
    expect(alert.textContent).not.toContain("a media folder");
  });

  it("polls every 2 s while starting and stops once it settles", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    server.current = settings(true, { state: "starting" });
    renderWithScan(<StateHarness />);
    expect(await screen.findByText("Starting to watch the media folders…")).toBeDefined();

    server.current = settings(true, { state: "active" });
    const before = server.gets();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    expect(server.gets()).toBeGreaterThan(before);
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
    const settled = server.gets();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(server.gets()).toBe(settled);
  });

  it("reads again when the media folders change and when the window regains focus", async () => {
    const view = renderWithScan(<StateHarness />);
    await screen.findByText("Watching the media folders.");
    const initial = server.gets();

    server.current = settings(true, {
      state: "limited",
      problem: "folder_unreachable",
      path: "/gone",
    });
    view.rerenderWith(<StateHarness reloadToken={1} />);
    expect(await screen.findByText("A media folder can't be reached")).toBeDefined();
    expect(server.gets()).toBeGreaterThan(initial);

    server.current = settings(true);
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    await waitFor(() =>
      expect(screen.queryByText("A media folder can't be reached")).toBeNull(),
    );
  });
});
