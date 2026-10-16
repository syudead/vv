import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  autoImportSettings as settings,
  autoImportURL,
  deferred,
  installAutoImportServer,
  json,
  renderWithScan,
} from "../testing/autoImportServer";
import AutoImportSection from "./AutoImportSection";

// 状態の言葉と問題は AutoImportState.test.tsx、保存の振る舞いは AutoImportSwitch.test.tsx が
// 確かめる。ここでは 2 つを 1 つの区画に並べたときのつながりを確かめる。

function autoImportSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Pick up changes as they happen" });
}

describe("AutoImportSection", () => {
  const server = installAutoImportServer();

  it("puts the state line under the switch in one section", async () => {
    server.current = settings(false);
    renderWithScan(<AutoImportSection reloadToken={0} />);

    const heading = await screen.findByRole("heading", { level: 2, name: "Auto-import" });
    const line = await screen.findByText("Off. Changes are picked up by the next scan.");
    const row = autoImportSwitch().closest("[data-slot='form-row']");
    expect(row?.contains(line)).toBe(true);
    expect(heading.closest("section")?.contains(row ?? null)).toBe(true);
  });

  it("shows the saved Starting state at once and polls until it settles", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<AutoImportSection reloadToken={0} />);
    await screen.findByText("Off. Changes are picked up by the next scan.");

    const put = deferred<Response>();
    server.fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === autoImportURL
        ? put.promise
        : server.route(input, init),
    );
    await user.click(autoImportSwitch());

    vi.useFakeTimers({ shouldAdvanceTime: true });
    server.current = settings(true, { state: "starting" });
    await act(async () => put.resolve(json(server.current)));
    expect(await screen.findByText("Starting to watch the media folders…")).toBeDefined();

    server.current = settings(true, { state: "active" });
    const before = server.gets();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    expect(server.gets()).toBeGreaterThan(before);
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
  });

  it("keeps a saved switch when the window regains focus while the save is pending", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<AutoImportSection reloadToken={0} />);
    await screen.findByText("Off. Changes are picked up by the next scan.");

    const save = deferred<Response>();
    server.fetchMock.mockImplementation((input, init) =>
      String(input) === autoImportURL && init?.method === "PUT"
        ? save.promise
        : server.route(input, init),
    );
    await user.click(autoImportSwitch());
    const before = server.gets();
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    // 保存の最中は読み直さない。
    expect(server.gets()).toBe(before);

    await act(async () => save.resolve(json(settings(true, { state: "active" }))));
    await screen.findByText("Watching the media folders.");
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("puts the failed save's state back in the state line", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<AutoImportSection reloadToken={0} />);
    await screen.findByText("Off. Changes are picked up by the next scan.");
    server.fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === autoImportURL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : server.route(input, init),
    );

    await user.click(autoImportSwitch());

    await screen.findByText(/Couldn't change the setting:/);
    expect(
      screen.getByText("Off. Changes are picked up by the next scan."),
    ).toBeDefined();
  });
});
