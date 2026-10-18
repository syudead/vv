import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NetworkSettings } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import NetworkSection from "./NetworkSection";

const NETWORK = "/api/settings/network";
const PROXY = "/api/settings/plugin-proxy";
const ADDRESS = "http://192.168.1.20:47880/";
const PROXY_LABEL = "Proxy for plugin requests";
const REFUSED =
  "Enter a URL starting with http://, https:// or socks5://, with a host and no path.";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const notFound = () => json({ code: "not_found", message: "not found" }, 404);
const serverError = () => json({ code: "internal", message: "x" }, 500);

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

function proxyInput(): Promise<HTMLInputElement> {
  return screen.findByRole("textbox", { name: PROXY_LABEL });
}

type Handler = (init?: RequestInit) => Response | Promise<Response>;

describe("NetworkSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  function serve(network: Handler, proxy: Handler = () => json({ url: "" })) {
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === NETWORK) return Promise.resolve(network(init));
      if (url === PROXY) return Promise.resolve(proxy(init));
      throw new Error(`unexpected request: ${url}`);
    });
  }

  function proxyServer(initial = "") {
    const server = { stored: initial, puts: [] as string[] };
    const handler: Handler = (init) => {
      if (init?.method === "PUT") {
        const { url } = JSON.parse(String(init.body)) as { url: string };
        server.puts.push(url);
        server.stored = url;
      }
      return json({ url: server.stored });
    };
    return { server, handler };
  }

  const putCalls = (url: string) =>
    fetchMock.mock.calls.filter(
      ([input, init]) => String(input) === url && init?.method === "PUT",
    );

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("renders nothing until a request answers", async () => {
    const network = deferred<Response>();
    const proxy = deferred<Response>();
    serve(
      () => network.promise,
      () => proxy.promise,
    );
    const { container } = render(<NetworkSection />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(container.innerHTML).toBe("");
    network.resolve(notFound());
    await new Promise((done) => setTimeout(done, 0));
    expect(container.innerHTML).toBe("");
    proxy.resolve(json({ url: "" }));
    expect(await proxyInput()).toBeDefined();
  });

  it("says LAN access is in the desktop app outside it (404), then the proxy row alone", async () => {
    serve(notFound);
    render(<NetworkSection />);

    expect(
      await screen.findByRole("heading", { level: 2, name: "Local network" }),
    ).toBeDefined();
    expect(
      screen.getByText("Connections to VVMDM, and from its plugins to other sites."),
    ).toBeDefined();
    const line = screen.getByText(/LAN access is set in the Windows desktop app\./);
    const input = await proxyInput();
    expect(
      line.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(input.value).toBe("");
    expect(input.getAttribute("placeholder")).toBe("http://proxy.example:3128");
    expect(input.getAttribute("type")).toBe("url");
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByText("Before you turn this on")).toBeNull();
  });

  it("shows the LAN switch and then the proxy row on the desktop app", async () => {
    serve(() => json(settings()));
    render(<NetworkSection />);

    const input = await proxyInput();
    const toggle = lanSwitch();
    expect(
      toggle.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByText(/By default only this PC can open VVMDM/)).toBeDefined();
    expect(input.getAttribute("aria-describedby")).toBe(
      "network-plugin-proxy-description",
    );
    expect(
      document.getElementById("network-plugin-proxy-description")?.textContent,
    ).toMatch(/^Plugin requests to other sites go through this proxy/);
  });

  it("saves a changed URL on blur and shows it after a reload", async () => {
    const { server, handler } = proxyServer();
    serve(notFound, handler);
    const user = userEvent.setup();
    const { unmount } = render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "  http://user:pass@proxy.example:3128 ");
    await user.tab();

    expect(await screen.findByText("Saved.")).toBeDefined();
    expect(server.puts).toEqual(["http://user:pass@proxy.example:3128"]);
    expect(input.value).toBe("http://user:pass@proxy.example:3128");
    await user.click(input);
    await user.tab();
    expect(server.puts).toHaveLength(1);

    unmount();
    render(<NetworkSection />);
    expect((await proxyInput()).value).toBe("http://user:pass@proxy.example:3128");
    expect(screen.queryByText("Saved.")).toBeNull();
  });

  it("saves on Enter and saves an emptied field as no proxy", async () => {
    const { server, handler } = proxyServer("http://proxy.example:3128");
    serve(notFound, handler);
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    expect(input.value).toBe("http://proxy.example:3128");
    await user.clear(input);
    await user.keyboard("{Enter}");

    expect(await screen.findByText("Saved.")).toBeDefined();
    expect(server.puts).toEqual([""]);
    await user.type(input, "h");
    expect(screen.queryByText("Saved.")).toBeNull();
  });

  it("does not send a refused URL and shows the error until the value changes", async () => {
    const { server, handler } = proxyServer();
    serve(notFound, handler);
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://proxy.example:3128/path");
    await user.tab();

    expect(await screen.findByText(REFUSED)).toBeDefined();
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input.getAttribute("aria-describedby")).toBe(
      "network-plugin-proxy-description network-plugin-proxy-error",
    );
    expect(server.puts).toHaveLength(0);

    await user.type(input, "x");
    expect(screen.queryByText(REFUSED)).toBeNull();
    expect(input.getAttribute("aria-invalid")).toBeNull();
  });

  it("shows the same error when the server refuses the URL", async () => {
    serve(notFound, (init) =>
      init?.method === "PUT"
        ? json({ code: "invalid_request", message: "x" }, 400)
        : json({ url: "" }),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://proxy.example:99");
    await user.tab();

    expect(await screen.findByText(REFUSED)).toBeDefined();
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input.value).toBe("http://proxy.example:99");
    expect(screen.queryByText("Saved.")).toBeNull();
  });

  it("keeps the typed value and shows the failure when the save fails", async () => {
    serve(notFound, (init) =>
      init?.method === "PUT" ? serverError() : json({ url: "" }),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://proxy.example");
    await user.tab();

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "Couldn't save the proxy: Something went wrong on the server.",
    );
    expect(input.value).toBe("http://proxy.example");
  });

  it("drops a failure once the value it was for has been edited", async () => {
    const first = deferred<Response>();
    serve(notFound, (init) =>
      init?.method === "PUT" ? first.promise : json({ url: "" }),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://one.example");
    await user.tab();
    expect(await screen.findByText("Saving…")).toBeDefined();

    await user.type(input, "x");
    first.resolve(serverError());
    await waitFor(() => expect(screen.queryByText("Saving…")).toBeNull());
    expect(screen.queryByRole("alert")).toBeNull();
    expect(input.value).toBe("http://one.examplex");
  });

  it("clears a shown failure when the value is edited", async () => {
    serve(notFound, (init) =>
      init?.method === "PUT" ? serverError() : json({ url: "" }),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://proxy.example");
    await user.tab();
    expect(await screen.findByRole("alert")).toBeDefined();

    await user.clear(input);
    await user.tab();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("runs one save at a time and keeps an edit made while saving", async () => {
    const first = deferred<Response>();
    const puts: string[] = [];
    serve(notFound, (init) => {
      if (init?.method !== "PUT") return json({ url: "" });
      const { url } = JSON.parse(String(init.body)) as { url: string };
      puts.push(url);
      return puts.length === 1 ? first.promise : json({ url });
    });
    const user = userEvent.setup();
    render(<NetworkSection />);

    const input = await proxyInput();
    await user.type(input, "http://one.example");
    await user.tab();
    expect(await screen.findByText("Saving…")).toBeDefined();

    await user.click(input);
    await user.clear(input);
    await user.type(input, "http://two.example");
    await user.tab();
    expect(puts).toEqual(["http://one.example"]);

    first.resolve(json({ url: "http://one.example" }));
    expect(await screen.findByText("Saved.")).toBeDefined();
    expect(puts).toEqual(["http://one.example", "http://two.example"]);
    expect(input.value).toBe("http://two.example");
    expect(putCalls(PROXY)).toHaveLength(2);
  });

  it("shows the caution and no addresses while LAN access is off", async () => {
    serve(() => json(settings()));
    render(<NetworkSection />);

    expect(
      await screen.findByRole("heading", { level: 2, name: "Local network" }),
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
    serve((init) => (init?.method === "PUT" ? put.promise : json(settings())));
    const user = userEvent.setup();
    render(<NetworkSection />);

    await screen.findByRole("switch");
    await user.click(lanSwitch());

    expect(fetchMock).toHaveBeenCalledWith(
      NETWORK,
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ lanAccess: true }),
      }),
    );
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
    expect(lanSwitch().getAttribute("aria-disabled")).toBe("true");
    await user.click(lanSwitch());
    expect(putCalls(NETWORK)).toHaveLength(1);

    put.resolve(json(settings({ lanAccess: true, addresses: [ADDRESS] })));
    const list = await screen.findByRole("list", { name: /Open one of these addresses/ });
    expect(list.textContent).toBe(ADDRESS);
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
    expect(lanSwitch().getAttribute("aria-disabled")).toBeNull();
    expect(screen.queryByText("Before you turn this on")).toBeNull();
  });

  it("lists the addresses when LAN access is already on", async () => {
    serve(() =>
      json(settings({ lanAccess: true, addresses: [ADDRESS, "http://10.0.0.5:47880/"] })),
    );
    render(<NetworkSection />);

    const list = await screen.findByRole("list", { name: /Open one of these addresses/ });
    expect(list.querySelectorAll("li")).toHaveLength(2);
    expect(lanSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("says so when LAN access is on but there is no address", async () => {
    serve(() => json(settings({ lanAccess: true })));
    render(<NetworkSection />);

    expect(
      await screen.findByText(/has no local network address right now/),
    ).toBeDefined();
  });

  it("shows the error and puts the switch back on 409 listen_failed", async () => {
    serve((init) =>
      init?.method === "PUT"
        ? json({ code: "conflict", reason: "listen_failed", message: "x" }, 409)
        : json(settings()),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    await screen.findByRole("switch");
    await user.click(lanSwitch());

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "Couldn't change the setting: vv couldn't change who can connect, so the previous setting is kept.",
    );
    expect(lanSwitch().getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText("Before you turn this on")).toBeDefined();
  });

  it("shows one retryable error when nothing can be loaded", async () => {
    let failing = true;
    serve(
      () => (failing ? serverError() : json(settings())),
      () => (failing ? serverError() : json({ url: "" })),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load the network settings: /);
    expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(1);
    failing = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("switch")).toBeDefined();
    expect(await proxyInput()).toBeDefined();
  });

  it("shows the error in the proxy row's place when only the proxy fails", async () => {
    let failing = true;
    serve(
      () => json(settings()),
      () => (failing ? serverError() : json({ url: "http://proxy.example" })),
    );
    const user = userEvent.setup();
    render(<NetworkSection />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load the network settings: /);
    expect(lanSwitch()).toBeDefined();
    expect(screen.queryByRole("textbox", { name: PROXY_LABEL })).toBeNull();
    failing = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect((await proxyInput()).value).toBe("http://proxy.example");
  });

  it("shows a retryable error when the proxy fails outside the desktop app", async () => {
    serve(notFound, serverError);
    render(<NetworkSection />);

    expect((await screen.findByRole("alert")).textContent).toMatch(
      /^Couldn't load the network settings: /,
    );
    expect(screen.getByRole("button", { name: "Retry" })).toBeDefined();
  });

  it("renders only catalog text", async () => {
    enablePseudoLocale();
    serve(
      (init) =>
        init?.method === "PUT"
          ? json({ code: "conflict", reason: "listen_failed", message: "x" }, 409)
          : json(settings()),
      (init) => (init?.method === "PUT" ? serverError() : json({ url: "" })),
    );
    const user = userEvent.setup();
    const { container } = render(<NetworkSection />);

    const toggle = await screen.findByRole("switch");
    const input = await screen.findByRole("textbox");
    expectCatalogTextOnly(container);
    await user.click(toggle);
    await screen.findByRole("alert");
    expectCatalogTextOnly(container);
    await user.type(input, "nope");
    await user.tab();
    expectCatalogTextOnly(container);
  });
});
