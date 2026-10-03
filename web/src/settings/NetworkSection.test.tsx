import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NetworkSettings } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import NetworkSection from "./NetworkSection";

const URL = "/api/settings/network";
const ADDRESS = "http://192.168.1.20:47880/";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings(values: Partial<NetworkSettings> = {}): NetworkSettings {
  return { lanAccess: false, port: 47880, addresses: [], ...values };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function lanSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Allow connections from the local network" });
}

describe("NetworkSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => vi.unstubAllGlobals());

  it("renders nothing outside the desktop app (404)", async () => {
    fetchMock.mockResolvedValue(json({ code: "not_found", message: "not found" }, 404));
    const { container } = render(<NetworkSection />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(URL, expect.anything()));
    await new Promise((done) => setTimeout(done, 0));
    expect(container.innerHTML).toBe("");
    expect(screen.queryByRole("heading", { name: "Network" })).toBeNull();
  });

  it("shows the caution and no addresses while LAN access is off", async () => {
    fetchMock.mockResolvedValue(json(settings()));
    render(<NetworkSection />);

    expect(
      await screen.findByRole("heading", { level: 2, name: "Network" }),
    ).toBeDefined();
    expect(lanSwitch().getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText("Before you turn this on")).toBeDefined();
    expect(
      screen.getByText(/Windows Firewall asks, allow VVMDM on private networks/),
    ).toBeDefined();
    expect(screen.getByText(/uses HTTP, so passwords travel unencrypted/)).toBeDefined();
    expect(
      screen.queryByRole("list", { name: /Open one of these addresses/ }),
    ).toBeNull();
  });

  it("sends PUT with the switch and lists the returned addresses", async () => {
    const put = deferred<Response>();
    fetchMock.mockImplementation((_input, init) =>
      init?.method === "PUT" ? put.promise : Promise.resolve(json(settings())),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    await screen.findByRole("heading", { name: "Network" });
    await user.click(lanSwitch());

    expect(fetchMock).toHaveBeenCalledWith(
      URL,
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ lanAccess: true }),
      }),
    );
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
    expect(lanSwitch().getAttribute("aria-disabled")).toBe("true");
    // 送信中にもう一度押しても送らない。
    await user.click(lanSwitch());
    expect(
      fetchMock.mock.calls.filter(([, init]) => init?.method === "PUT"),
    ).toHaveLength(1);

    put.resolve(json(settings({ lanAccess: true, addresses: [ADDRESS] })));
    const list = await screen.findByRole("list", { name: /Open one of these addresses/ });
    expect(list.textContent).toBe(ADDRESS);
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
    expect(lanSwitch().getAttribute("aria-disabled")).toBeNull();
    expect(screen.queryByText("Before you turn this on")).toBeNull();
  });

  it("lists the addresses when LAN access is already on", async () => {
    fetchMock.mockResolvedValue(
      json(settings({ lanAccess: true, addresses: [ADDRESS, "http://10.0.0.5:47880/"] })),
    );
    render(<NetworkSection />);

    const list = await screen.findByRole("list", { name: /Open one of these addresses/ });
    expect(list.querySelectorAll("li")).toHaveLength(2);
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("says so when LAN access is on but there is no address", async () => {
    fetchMock.mockResolvedValue(json(settings({ lanAccess: true })));
    render(<NetworkSection />);

    expect(
      await screen.findByText(/has no local network address right now/),
    ).toBeDefined();
  });

  it("shows the error and puts the switch back on 409 listen_failed", async () => {
    fetchMock.mockImplementation((_input, init) =>
      Promise.resolve(
        init?.method === "PUT"
          ? json({ code: "conflict", reason: "listen_failed", message: "x" }, 409)
          : json(settings()),
      ),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    await screen.findByRole("heading", { name: "Network" });
    await user.click(lanSwitch());

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "Couldn't change the setting: vv couldn't change who can connect, so the previous setting is kept.",
    );
    expect(lanSwitch().getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText("Before you turn this on")).toBeDefined();
  });

  it("shows a retryable error when the settings can't be loaded", async () => {
    fetchMock.mockResolvedValueOnce(json({ code: "internal", message: "x" }, 500));
    fetchMock.mockResolvedValueOnce(json(settings()));
    const user = userEvent.setup();
    render(<NetworkSection />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load the network settings: /);
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("switch")).toBeDefined();
  });

  it("renders only catalog text", async () => {
    enablePseudoLocale();
    fetchMock.mockImplementation((_input, init) =>
      Promise.resolve(
        init?.method === "PUT"
          ? json({ code: "conflict", reason: "listen_failed", message: "x" }, 409)
          : json(settings()),
      ),
    );
    const user = userEvent.setup();
    const { container } = render(<NetworkSection />);

    const toggle = await screen.findByRole("switch");
    expectCatalogTextOnly(container);
    await user.click(toggle);
    await screen.findByRole("alert");
    expectCatalogTextOnly(container);
  });
});
