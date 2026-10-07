import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AutoImportSettings } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { ScanProvider } from "../shell/ScanProvider";
import { OwnerAudience } from "../testing/audience";
import AutoImportSection from "./AutoImportSection";

const URL = "/api/settings/auto-import";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings(
  enabled: boolean,
  watch: AutoImportSettings["watch"] = { state: enabled ? "active" : "off" },
): AutoImportSettings {
  return { enabled, watch };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function autoImportSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Pick up changes as they happen" });
}

describe("AutoImportSection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: AutoImportSettings;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === "/api/scans/current")
      return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
    if (url === "/api/media-folders") return Promise.resolve(json([]));
    if (url === URL && init?.method === "PUT") {
      const body = JSON.parse(String(init.body)) as { enabled: boolean };
      current = settings(
        body.enabled,
        body.enabled ? { state: "starting" } : { state: "off" },
      );
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  function renderSection(reloadToken = 0) {
    const view = render(
      <OwnerAudience>
        <ScanProvider>
          <AutoImportSection reloadToken={reloadToken} />
        </ScanProvider>
      </OwnerAudience>,
    );
    return {
      ...view,
      rerenderToken: (token: number) =>
        view.rerender(
          <OwnerAudience>
            <ScanProvider>
              <AutoImportSection reloadToken={token} />
            </ScanProvider>
          </OwnerAudience>,
        ),
    };
  }

  const getsOfSettings = () =>
    fetchMock.mock.calls.filter(
      ([input, init]) => String(input) === URL && init?.method !== "PUT",
    ).length;

  beforeEach(() => {
    current = settings(true);
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("shows the section between the words of each state", async () => {
    current = settings(false);
    const view = renderSection();

    expect(
      await screen.findByRole("heading", { level: 2, name: "Auto-import" }),
    ).toBeDefined();
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("false");
    expect(
      screen.getByText("Off. Changes are picked up by the next scan."),
    ).toBeDefined();
    view.unmount();

    current = settings(true);
    renderSection();
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("says to add a media folder when it is on and there is none", async () => {
    current = settings(true, { state: "off" });
    renderSection();

    expect(
      await screen.findByText("Add a media folder below to watch it."),
    ).toBeDefined();
  });

  it("shows the problem with its path for a limited watch, and no button", async () => {
    current = settings(true, {
      state: "limited",
      problem: "watch_limit",
      path: "/media/very/deep",
    });
    renderSection();

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
    current = settings(true, { state: "limited", problem: "events_lost" });
    renderSection();

    expect(await screen.findByText("Some changes were lost")).toBeDefined();
    expect(
      screen.getByText(/Start a scan under Scan status to pick them up/),
    ).toBeDefined();
  });

  it("saves the choice, shows Starting, and polls every 2 s until it settles", async () => {
    current = settings(false);
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Off. Changes are picked up by the next scan.");

    const put = deferred<Response>();
    fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === URL ? put.promise : route(input, init),
    );
    await user.click(autoImportSwitch());

    expect(fetchMock).toHaveBeenCalledWith(
      URL,
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ enabled: true }),
      }),
    );
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
    expect(autoImportSwitch().getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Saving…")).toBeDefined();
    // 送信中にもう一度押しても送らない。
    await user.click(autoImportSwitch());
    expect(
      fetchMock.mock.calls.filter(([, init]) => init?.method === "PUT"),
    ).toHaveLength(1);

    vi.useFakeTimers({ shouldAdvanceTime: true });
    current = settings(true, { state: "starting" });
    await act(async () => put.resolve(json(current)));
    expect(await screen.findByText("Starting to watch the media folders…")).toBeDefined();
    expect(screen.queryByText("Saving…")).toBeNull();

    // 2 秒ごとに読み直し、落ち着けば止める。
    current = settings(true, { state: "active" });
    const before = getsOfSettings();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    expect(getsOfSettings()).toBeGreaterThan(before);
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
    const settled = getsOfSettings();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(getsOfSettings()).toBe(settled);
  });

  it("reverts the switch and says why when saving fails", async () => {
    current = settings(false);
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Off. Changes are picked up by the next scan.");
    fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === URL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : route(input, init),
    );

    await user.click(autoImportSwitch());

    const alert = await screen.findByText(/Couldn't change the setting:/);
    expect(alert.closest("[data-slot='alert']")?.getAttribute("role")).toBe("alert");
    await waitFor(() =>
      expect(autoImportSwitch().getAttribute("aria-checked")).toBe("false"),
    );
    expect(
      screen.getByText("Off. Changes are picked up by the next scan."),
    ).toBeDefined();
  });

  it("offers a retry when the first read fails", async () => {
    fetchMock.mockImplementation((input, init) =>
      String(input) === URL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : route(input, init),
    );
    const user = userEvent.setup();
    renderSection();

    expect(
      await screen.findByText(/Couldn't load the auto-import settings:/),
    ).toBeDefined();
    expect(screen.queryByRole("switch")).toBeNull();

    fetchMock.mockImplementation(route);
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Watching the media folders.")).toBeDefined();
  });

  it("reads again when the media folders change and when the window regains focus", async () => {
    const view = renderSection();
    await screen.findByText("Watching the media folders.");
    const initial = getsOfSettings();

    current = settings(true, {
      state: "limited",
      problem: "folder_unreachable",
      path: "/gone",
    });
    view.rerenderToken(1);
    expect(await screen.findByText("A media folder can't be reached")).toBeDefined();
    expect(getsOfSettings()).toBeGreaterThan(initial);

    current = settings(true);
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    await waitFor(() =>
      expect(screen.queryByText("A media folder can't be reached")).toBeNull(),
    );
  });
});
