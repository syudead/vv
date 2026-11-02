import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { PeerLinkState, PeerState } from "../api/peer";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import PairingSection from "./PairingSection";

const NODE = { id: "a1", name: "den-pc", version: "1.4.0" };
const ADDRESS = "http://192.168.1.20:8080";
const LINK: PeerLinkState = {
  slot: 1,
  nodeId: "n1",
  state: "connected",
  name: "studio-pc",
  address: ADDRESS,
  version: "1.4.0",
  pinned: false,
};
const OTHER: PeerLinkState = {
  ...LINK,
  slot: 2,
  nodeId: "n2",
  name: "attic-nas",
  address: "http://192.168.1.21:8080",
};
const GRANT = { name: "laptop", pairedAt: "2026-10-20T09:30:00Z" };
const NETWORK_DETAIL =
  "Its videos, groups and smart folders are hidden until it answers. Check that it is running and that the address below is right.";

const unpaired: PeerState = { links: [], node: NODE };
const connected: PeerState = { links: [LINK], node: NODE };

/** The state with the one link changed by link, and the rest by state. */
function linked(link: Partial<PeerLinkState>, state: Partial<PeerState> = {}): PeerState {
  return { links: [{ ...LINK, ...link }], node: NODE, ...state };
}

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
    ADDRESS,
  );
  await user.type(screen.getByRole("textbox", { name: "Its pairing code" }), code);
}

const pairButton = () => screen.getByRole("button", { name: "Pair" });
const readSection = () => screen.getByRole("region", { name: "Read another VVMDM" });
const linkSection = (name: string) => screen.getByRole("region", { name });
const readersSection = () =>
  screen.getByRole("region", { name: "VVMDMs that read this one" });

beforeEach(() => installFakeEventSource());
afterEach(() => vi.unstubAllGlobals());

describe("PairingSection, reading another VVMDM", () => {
  it("shows both sections, with the form and the code, while nothing is paired", async () => {
    serve(unpaired);
    renderSection();

    await screen.findByRole("heading", { name: "Read another VVMDM" });
    expect(within(readSection()).getByRole("button", { name: "Pair" })).toBeDefined();
    expect(
      within(readersSection()).getByRole("button", { name: "Show pairing code" }),
    ).toBeDefined();
    expect(screen.queryByRole("textbox", { name: "Address of this VVMDM" })).toBeNull();
  });

  it("pairs another link after the first one and clears the form", async () => {
    const sent = serve(connected, {
      "POST /api/peer/pair": () => json({ ...connected, links: [LINK, OTHER] }, 201),
    });
    const user = userEvent.setup();
    renderSection();

    await fillForm(user);
    await user.click(pairButton());

    const added = await screen.findByRole("region", { name: OTHER.name });
    expect(within(added).getByText("Connected")).toBeDefined();
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((heading) => heading.textContent),
    ).toEqual(["Read another VVMDM", LINK.name, OTHER.name, "VVMDMs that read this one"]);
    expect(
      (
        screen.getByRole("textbox", {
          name: "Address of the other VVMDM",
        }) as HTMLInputElement
      ).value,
    ).toBe("");
    expect(sent).toEqual([
      {
        method: "POST",
        url: "/api/peer/pair",
        body: { address: ADDRESS, code: "ABCDEFGH" },
      },
    ]);
  });

  it.each([
    [
      "already_paired",
      conflict("already_paired"),
      "This VVMDM already reads the VVMDM at this address, or another VVMDM already reads it. Remove that pairing on its Pairing page first.",
    ],
    [
      "peer_limit",
      conflict("peer_limit"),
      "This VVMDM already reads 8 VVMDMs, the most it can. Unpair one first.",
    ],
    [
      "peer_unreachable",
      json(
        {
          code: "conflict",
          reason: "peer_unreachable",
          message: "Could not reach the other VVMDM at http://192.168.1.20:8080.",
        },
        409,
      ),
      "Couldn't complete the pairing: Could not reach the other VVMDM at http://192.168.1.20:8080.",
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
    ).toBe(ADDRESS);
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

    expect(await screen.findByRole("region", { name: LINK.name })).toBeDefined();
    expect(sent[1]?.body).toEqual({
      address: ADDRESS,
      code: "ABCDEFGH",
      fingerprint: "c2hhMjU2LXBpbg==",
    });
  });

  it("shows each link in its own section with its own state", async () => {
    serve({
      links: [LINK, { ...OTHER, state: "unreachable", reason: "network" }],
      node: NODE,
    });
    renderSection();

    const first = within(await screen.findByRole("region", { name: LINK.name }));
    expect(first.getByText("Connected")).toBeDefined();
    expect(first.queryByText(NETWORK_DETAIL)).toBeNull();
    const second = within(linkSection(OTHER.name ?? ""));
    expect(second.getByText("Not connected")).toBeDefined();
    expect(second.getByText(NETWORK_DETAIL)).toBeDefined();
    expect(second.getByRole("textbox", { name: "Address" })).toHaveProperty(
      "value",
      OTHER.address,
    );
  });

  it("follows the peer event between the faces and the states", async () => {
    serve(unpaired);
    renderSection();
    await screen.findByRole("heading", { name: "Read another VVMDM" });

    await emitServerEvent("peer", connected);
    expect(await screen.findByText("Connected")).toBeDefined();

    await emitServerEvent("peer", linked({ state: "unreachable", reason: "network" }));
    expect(await screen.findByText("Not connected")).toBeDefined();
    expect(screen.queryByText("Connected")).toBeNull();
  });

  it("puts the limit line in place of the form at 8 links, and the form back after an unpair", async () => {
    const links = Array.from({ length: 8 }, (_, index) => ({
      ...LINK,
      slot: index + 1,
      nodeId: `n${String(index + 1)}`,
      name: `pc-${String(index + 1)}`,
    }));
    serve({ links, node: NODE }, { "DELETE /api/peer/links/n8": () => json(null, 204) });
    const user = userEvent.setup();
    renderSection();

    expect(
      await within(
        await screen.findByRole("region", { name: "Read another VVMDM" }),
      ).findByText(
        "This VVMDM reads 8 VVMDMs, the most it can. Unpair one to read another.",
      ),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: "Pair" })).toBeNull();

    await user.click(within(linkSection("pc-8")).getByRole("button", { name: "Unpair" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Stop reading pc-8?" });
    await user.click(within(dialog).getByRole("button", { name: "Unpair" }));

    expect(
      await within(readSection()).findByRole("button", { name: "Pair" }),
    ).toBeDefined();
  });

  it("sends a changed address on Enter and says Saved", async () => {
    const changed = "http://192.168.1.30:8080";
    const sent = serve(linked({ state: "unreachable", reason: "network" }), {
      "PUT /api/peer/links/n1/address": () => json(linked({ address: changed })),
    });
    const user = userEvent.setup();
    renderSection();

    const address = await screen.findByRole("textbox", { name: "Address" });
    await user.clear(address);
    await user.type(address, `${changed}{Enter}`);

    expect(await screen.findByText("Saved.")).toBeDefined();
    expect(sent).toEqual([
      { method: "PUT", url: "/api/peer/links/n1/address", body: { address: changed } },
    ]);
    // The state waits for the next peer event.
    expect(screen.getByText("Not connected")).toBeDefined();
  });

  it("asks for a first trust, not a changed certificate, when nothing is pinned", async () => {
    serve(linked({ state: "unreachable", reason: "certificate", fingerprint: "b2xk" }));
    renderSection();

    expect(
      await screen.findByText("Confirm its certificate below to connect."),
    ).toBeDefined();
    expect(screen.getByText("Certificate not trusted")).toBeDefined();
    expect(screen.queryByText("Confirm the new certificate")).toBeNull();
  });

  it("trusts the fingerprint it shows, and the new one after fingerprint_changed", async () => {
    const sent = serve(
      linked({
        pinned: true,
        state: "unreachable",
        reason: "certificate",
        fingerprint: "b2xk",
      }),
      {
        "POST /api/peer/links/n1/certificate": [
          () => conflict("fingerprint_changed", { fingerprint: "bmV3" }),
          () => json(linked({ state: "unreachable", reason: "certificate" })),
        ],
      },
    );
    const user = userEvent.setup();
    renderSection();

    expect(
      await screen.findByText(
        "Its certificate is not the one you trusted. Confirm the new one below to reconnect.",
      ),
    ).toBeDefined();
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

  it("unpairs one link after the confirmation and keeps the other and the grant", async () => {
    const sent = serve(
      { links: [LINK, OTHER], node: NODE, grant: GRANT },
      { "DELETE /api/peer/links/n1": () => json(null, 204) },
    );
    const user = userEvent.setup();
    renderSection();

    await user.click(
      within(await screen.findByRole("region", { name: LINK.name })).getByRole("button", {
        name: "Unpair",
      }),
    );
    const dialog = await screen.findByRole("alertdialog", {
      name: "Stop reading studio-pc?",
    });
    await user.click(within(dialog).getByRole("button", { name: "Unpair" }));

    await waitFor(() =>
      expect(screen.queryByRole("region", { name: LINK.name })).toBeNull(),
    );
    expect(within(linkSection(OTHER.name ?? "")).getByText("Connected")).toBeDefined();
    expect(
      within(readersSection()).getByText("laptop can read and organize this VVMDM"),
    ).toBeDefined();
    expect(sent).toEqual([
      { method: "DELETE", url: "/api/peer/links/n1", body: undefined },
    ]);
  });

  it("removes a link the other side revoked, without an address row", async () => {
    const sent = serve(linked({ state: "revoked" }), {
      "DELETE /api/peer/links/n1": () => json(null, 204),
    });
    const user = userEvent.setup();
    renderSection();

    expect(await screen.findByText("Access removed")).toBeDefined();
    expect(screen.queryByRole("textbox", { name: "Address" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Remove pairing" }));

    await waitFor(() =>
      expect(screen.queryByRole("region", { name: LINK.name })).toBeNull(),
    );
    expect(sent).toEqual([
      { method: "DELETE", url: "/api/peer/links/n1", body: undefined },
    ]);
  });
});

describe("PairingSection, VVMDMs that read this one", () => {
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

  it("shows the reader in place of the code and removes its access after the confirmation", async () => {
    const sent = serve(
      { ...unpaired, grant: GRANT },
      { "DELETE /api/peer/grant": () => json(null, 204) },
    );
    const user = userEvent.setup();
    renderSection();

    const readers = within(
      await screen.findByRole("region", { name: "VVMDMs that read this one" }),
    );
    expect(readers.getByText("laptop can read and organize this VVMDM")).toBeDefined();
    expect(readers.getByText(/^Paired /)).toBeDefined();
    expect(readers.queryByRole("button", { name: "Show pairing code" })).toBeNull();
    await user.click(readers.getByRole("button", { name: "Remove access" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Remove laptop's access?",
    });
    await user.click(within(dialog).getByRole("button", { name: "Remove access" }));

    expect(
      await within(readersSection()).findByRole("button", { name: "Show pairing code" }),
    ).toBeDefined();
    expect(sent).toEqual([{ method: "DELETE", url: "/api/peer/grant", body: undefined }]);
  });

  it("renders only catalog text besides the peers' data", async () => {
    enablePseudoLocale();
    serve(
      linked(
        { state: "unreachable", reason: "certificate", fingerprint: "b2xk" },
        { grant: GRANT },
      ),
    );
    const { container } = renderSection();
    await screen.findByText("b2xk");

    expectCatalogTextOnly(container, [
      LINK.name ?? "",
      ADDRESS,
      LINK.version ?? "",
      "b2xk",
      GRANT.name,
    ]);
  });
});
