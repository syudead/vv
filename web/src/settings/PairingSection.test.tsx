import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { PeerState } from "../api/peer";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import PairingSection from "./PairingSection";

const NODE = { id: "a1", name: "den-pc", version: "1.4.0" };
const PEER = {
  name: "studio-pc",
  address: "http://192.168.1.20:8080",
  version: "1.4.0",
  pinned: false,
};

const unpaired: PeerState = { state: "unpaired", node: NODE };
const connected: PeerState = { state: "connected", peer: PEER, node: NODE };

function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function conflict(reason: string, extra: object = {}): Response {
  return json({ code: "conflict", message: "", reason, ...extra }, 409);
}

interface Sent {
  method: string;
  url: string;
  body: unknown;
}

type Route = (body: unknown) => Response;

/** GET /api/peer answers `initial`; other routes answer from `routes` and record what was sent. */
function serve(initial: PeerState, routes: Record<string, Route | Route[]> = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (method === "GET" && url === "/api/peer") return Promise.resolve(json(initial));
    const body: unknown =
      init?.body === undefined ? undefined : JSON.parse(String(init.body));
    sent.push({ method, url, body });
    const route = routes[`${method} ${url}`];
    const next = Array.isArray(route) ? route.shift() : route;
    if (next === undefined) throw new Error(`unexpected request: ${method} ${url}`);
    return Promise.resolve(next(body));
  });
  return sent;
}

function renderSection() {
  return render(
    <ToastProvider>
      <TooltipProvider>
        <PairingSection />
      </TooltipProvider>
    </ToastProvider>,
  );
}

async function fillForm(user: ReturnType<typeof userEvent.setup>, code = "abcd efgh") {
  await user.type(
    await screen.findByRole("textbox", { name: "Address of the other VVMDM" }),
    PEER.address,
  );
  await user.type(screen.getByRole("textbox", { name: "Its pairing code" }), code);
}

const pairButton = () => screen.getByRole("button", { name: "Pair" });

beforeEach(() => installFakeEventSource());
afterEach(() => vi.unstubAllGlobals());

describe("PairingSection, unpaired", () => {
  it("shows a pairing code in two groups with its countdown", async () => {
    const sent = serve(unpaired, {
      "POST /api/peer/code": () =>
        json(
          { code: "ABCDEFGH", expiresAt: new Date(Date.now() + 600_000).toISOString() },
          201,
        ),
    });
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Show pairing code" }));

    expect(await screen.findByText("ABCD EFGH")).toBeDefined();
    expect(screen.getByRole("status").textContent).toBe("Expires in 10:00");
    expect(screen.getByRole("button", { name: "New code" })).toBeDefined();
    expect(sent).toEqual([{ method: "POST", url: "/api/peer/code", body: undefined }]);
  });

  it("pairs with both addresses and the typed code, then shows Connected", async () => {
    const sent = serve(unpaired, { "POST /api/peer/pair": () => json(connected, 201) });
    const user = userEvent.setup();
    renderSection();

    const own = await screen.findByRole("textbox", { name: "Address of this VVMDM" });
    expect((own as HTMLInputElement).value).toBe(window.location.origin);
    await fillForm(user);
    await user.click(pairButton());

    expect(await screen.findByText("Connected")).toBeDefined();
    expect(screen.getByRole("heading", { name: "Paired VVMDM" })).toBeDefined();
    expect(sent).toEqual([
      {
        method: "POST",
        url: "/api/peer/pair",
        body: {
          address: PEER.address,
          code: "ABCDEFGH",
          ownAddress: window.location.origin,
        },
      },
    ]);
  });

  it.each([
    [
      "already_paired",
      conflict("already_paired"),
      "One of the two VVMDMs is already paired. Unpair it on its Pairing page first.",
    ],
    [
      "peer_unreachable",
      json(
        {
          code: "conflict",
          reason: "peer_unreachable",
          message: "the other node could not call this node back",
        },
        409,
      ),
      "Couldn't complete the pairing: the other node could not call this node back",
    ],
  ])("names %s and keeps the form filled", async (_reason, response, shown) => {
    serve(unpaired, { "POST /api/peer/pair": () => response });
    const user = userEvent.setup();
    renderSection();

    await fillForm(user);
    await user.click(pairButton());

    expect(await screen.findByText(shown)).toBeDefined();
    expect(
      (
        screen.getByRole("textbox", {
          name: "Address of the other VVMDM",
        }) as HTMLInputElement
      ).value,
    ).toBe(PEER.address);
    expect(
      (screen.getByRole("textbox", { name: "Its pairing code" }) as HTMLInputElement)
        .value,
    ).toBe("ABCD EFGH");
    expect(pairButton().hasAttribute("disabled")).toBe(false);
  });

  it("puts a refused code under the code field", async () => {
    serve(unpaired, {
      "POST /api/peer/pair": () =>
        json({ code: "invalid_request", message: "", reason: "invalid_code" }, 400),
    });
    const user = userEvent.setup();
    renderSection();

    await fillForm(user);
    await user.click(pairButton());

    const code = screen.getByRole("textbox", { name: "Its pairing code" });
    await waitFor(() => expect(code.getAttribute("aria-invalid")).toBe("true"));
    expect(code.getAttribute("aria-describedby")).toContain(
      within(code.parentElement as HTMLElement).getByRole("alert").id,
    );
  });

  it("pairs again with the fingerprint it showed after Trust and pair", async () => {
    const sent = serve(unpaired, {
      "POST /api/peer/pair": [
        () => conflict("certificate_untrusted", { fingerprint: "c2hhMjU2LXBpbg==" }),
        () => json(connected, 201),
      ],
    });
    const user = userEvent.setup();
    renderSection();

    await fillForm(user);
    await user.click(pairButton());
    expect(await screen.findByText("c2hhMjU2LXBpbg==")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Trust and pair" }));

    expect(await screen.findByText("Connected")).toBeDefined();
    expect(sent[1]?.body).toEqual({
      address: PEER.address,
      code: "ABCDEFGH",
      ownAddress: window.location.origin,
      fingerprint: "c2hhMjU2LXBpbg==",
    });
  });

  it("confirms this VVMDM's own certificate, then the other's, and sends both", async () => {
    const sent = serve(unpaired, {
      "POST /api/peer/pair": [
        () => conflict("own_certificate_unverified", { fingerprint: "b3du" }),
        () => conflict("certificate_untrusted", { fingerprint: "cGVlcg==" }),
        () => json(connected, 201),
      ],
    });
    const user = userEvent.setup();
    renderSection();

    await fillForm(user);
    await user.click(pairButton());
    expect(
      await screen.findByText("This VVMDM's certificate can't be verified"),
    ).toBeDefined();
    expect(
      screen.getByText(
        /compare this fingerprint with the certificate this VVMDM serves/i,
      ),
    ).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Trust and pair" }));
    expect(await screen.findByText("Certificate not trusted")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Trust and pair" }));

    expect(await screen.findByText("Connected")).toBeDefined();
    expect(sent.map((request) => request.body)).toEqual([
      { address: PEER.address, code: "ABCDEFGH", ownAddress: window.location.origin },
      {
        address: PEER.address,
        code: "ABCDEFGH",
        ownAddress: window.location.origin,
        ownFingerprint: "b3du",
      },
      {
        address: PEER.address,
        code: "ABCDEFGH",
        ownAddress: window.location.origin,
        ownFingerprint: "b3du",
        fingerprint: "cGVlcg==",
      },
    ]);
  });
});

describe("PairingSection, paired", () => {
  it("follows the peer event between the faces and the states", async () => {
    serve(unpaired);
    renderSection();
    await screen.findByRole("heading", { name: "Pair with another VVMDM" });

    await emitServerEvent("peer", connected);
    expect(await screen.findByText("Connected")).toBeDefined();

    await emitServerEvent("peer", {
      ...connected,
      state: "unreachable",
      reason: "network",
    });
    expect(await screen.findByText("Can't reach it")).toBeDefined();
    expect(screen.queryByText("Connected")).toBeNull();
  });

  it("sends a changed address on Enter and says Saved", async () => {
    const changed = "http://192.168.1.30:8080";
    const sent = serve(
      { ...connected, state: "unreachable", reason: "network" },
      {
        "PUT /api/peer/address": () =>
          json({ ...connected, peer: { ...PEER, address: changed } }),
      },
    );
    const user = userEvent.setup();
    renderSection();

    const address = await screen.findByRole("textbox", { name: "Address" });
    await user.clear(address);
    await user.type(address, `${changed}{Enter}`);

    expect(await screen.findByText("Saved.")).toBeDefined();
    expect(sent).toEqual([
      { method: "PUT", url: "/api/peer/address", body: { address: changed } },
    ]);
    // The state line waits for the next peer event.
    expect(screen.getByText("Can't reach it")).toBeDefined();
  });

  it("asks for a first trust, not a changed certificate, when nothing is pinned", async () => {
    serve({
      ...connected,
      state: "unreachable",
      reason: "certificate",
      fingerprint: "b2xk",
    });
    renderSection();

    expect(await screen.findByText("Can't verify its certificate")).toBeDefined();
    expect(screen.getByText("Certificate not trusted")).toBeDefined();
    expect(screen.queryByText("Certificate changed")).toBeNull();
    expect(screen.queryByText("Confirm the new certificate")).toBeNull();
  });

  it("trusts the fingerprint it shows, and the new one after fingerprint_changed", async () => {
    const sent = serve(
      {
        ...connected,
        peer: { ...PEER, pinned: true },
        state: "unreachable",
        reason: "certificate",
        fingerprint: "b2xk",
      },
      {
        "POST /api/peer/certificate": [
          () => conflict("fingerprint_changed", { fingerprint: "bmV3" }),
          () => json({ ...connected, state: "unreachable", reason: "certificate" }),
        ],
      },
    );
    const user = userEvent.setup();
    renderSection();

    expect(await screen.findByText("Certificate changed")).toBeDefined();
    expect(screen.getByText("Confirm the new certificate")).toBeDefined();
    await user.click(await screen.findByRole("button", { name: "Trust and reconnect" }));
    expect(
      await screen.findByText(
        "The certificate changed again. Compare the new fingerprint.",
      ),
    ).toBeDefined();
    expect(screen.getByText("bmV3")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Trust and reconnect" }));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent.map((request) => request.body)).toEqual([
      { fingerprint: "b2xk" },
      { fingerprint: "bmV3" },
    ]);
  });

  it("unpairs after the confirmation and returns to the unpaired face", async () => {
    const sent = serve(connected, { "DELETE /api/peer": () => json(null, 204) });
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Unpair" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Unpair from studio-pc?",
    });
    await user.click(within(dialog).getByRole("button", { name: "Unpair" }));

    expect(
      await screen.findByRole("heading", { name: "Pair with another VVMDM" }),
    ).toBeDefined();
    expect(sent).toEqual([{ method: "DELETE", url: "/api/peer", body: undefined }]);
  });

  it("removes a pairing the other side ended, without an address row", async () => {
    const sent = serve(
      { state: "revoked", peer: PEER, node: NODE },
      { "DELETE /api/peer": () => json(null, 204) },
    );
    const user = userEvent.setup();
    renderSection();

    expect(await screen.findByText("Unpaired by the other side")).toBeDefined();
    expect(screen.queryByRole("textbox", { name: "Address" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Remove pairing" }));

    expect(
      await screen.findByRole("heading", { name: "Pair with another VVMDM" }),
    ).toBeDefined();
    expect(sent).toEqual([{ method: "DELETE", url: "/api/peer", body: undefined }]);
  });

  it("renders only catalog text besides the peer's data", async () => {
    enablePseudoLocale();
    serve({
      ...connected,
      state: "unreachable",
      reason: "certificate",
      fingerprint: "b2xk",
    });
    const { container } = renderSection();
    await screen.findByText("b2xk");

    expectCatalogTextOnly(container, [PEER.name, PEER.address, PEER.version, "b2xk"]);
  });
});
