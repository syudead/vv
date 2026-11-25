import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "../../../api/credits";
import { jsonResponse } from "../../../testing/personPage";
import { TooltipProvider } from "../../../ui/shadcn/tooltip";
import OrganizationEditPage from "./OrganizationEditPage";

// Opened by its URL; the leaving hook and the discard check are the person edit page's, shared.

const studio: Organization = {
  id: 5,
  name: "Studio",
  names: [{ name: "Studio" }, { name: "スタジオ", language: "ja" }],
  externalIds: [{ source: "db", value: "9" }],
  profile: { foundedDate: "2001", location: "Tokyo" },
  parent: { id: 2, name: "Group", provisional: false },
  favorite: false,
  videoCount: 3,
  createdAt: "2026-01-01T00:00:00Z",
};

const candidates: Organization[] = [
  { ...studio, id: 2, name: "Group", names: [{ name: "Group" }], parent: undefined },
  { ...studio, id: 8, name: "Label", names: [{ name: "Label" }], parent: undefined },
];

const server = {
  organization: (): Response => jsonResponse(studio),
  patch: vi.fn<(body: unknown) => Response>(),
};

function Where() {
  return <p data-testid="where">{useLocation().pathname}</p>;
}

function renderEditor() {
  render(
    <TooltipProvider>
      <MemoryRouter
        initialEntries={["/organizations", "/organizations/5/edit"]}
        initialIndex={1}
      >
        <Where />
        <Routes>
          <Route path="/organizations/:id/edit" element={<OrganizationEditPage />} />
          <Route path="/organizations/:id" element={<p>organization page</p>} />
          <Route path="/organizations" element={<p>organizations</p>} />
        </Routes>
      </MemoryRouter>
    </TooltipProvider>,
  );
}

/** Opens the page and returns the first name row's input. */
async function opened() {
  renderEditor();
  const [first] = await screen.findAllByRole<HTMLInputElement>("textbox", {
    name: "Name",
  });
  return first!;
}

function where() {
  return screen.getByTestId("where").textContent;
}

async function chooseParent(name: string) {
  const user = userEvent.setup();
  const parent = screen.getByRole<HTMLButtonElement>("combobox", {
    name: "Parent organization",
  });
  await waitFor(() => expect(parent.disabled).toBe(false));
  await user.click(parent);
  await user.click(await screen.findByRole("option", { name }));
}

describe("OrganizationEditPage", () => {
  beforeEach(() => {
    server.organization = () => jsonResponse(studio);
    server.patch.mockReset();
    server.patch.mockImplementation(() => jsonResponse(studio));
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/organizations/5" && init?.method === "PATCH")
          return Promise.resolve(server.patch(JSON.parse(String(init.body))));
        if (url.pathname === "/api/organizations/5")
          return Promise.resolve(server.organization());
        if (url.pathname === "/api/organizations")
          return Promise.resolve(
            jsonResponse({ items: [studio, ...candidates], total: 3, totalAll: 3 }),
          );
        return Promise.resolve(jsonResponse({ code: "not_found" }, 404));
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("名前に着いて、1 回の PATCH で変えた property だけを送り、団体ページへ戻る", async () => {
    const name = await opened();
    expect(document.activeElement).toBe(name);
    const user = userEvent.setup();
    await user.type(name, " One");
    await user.clear(screen.getByRole("textbox", { name: "Location" }));
    await chooseParent("Label");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("organization page")).toBeDefined();
    expect(where()).toBe("/organizations/5");
    expect(server.patch).toHaveBeenCalledTimes(1);
    expect(server.patch.mock.calls[0]![0]).toEqual({
      names: [{ name: "Studio One" }, { name: "スタジオ", language: "ja" }],
      parentId: 8,
      organizationProfile: { location: null },
    });
  });

  it("変えたまま Cancel で離れると Discard changes? を確かめてから離れる", async () => {
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Location" }), "-to");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    expect(where()).toBe("/organizations/5/edit");
    await user.click(within(dialog).getByRole("button", { name: "Discard" }));
    expect(await screen.findByText("organization page")).toBeDefined();
    expect(server.patch).not.toHaveBeenCalled();
  });

  describe("親を拒む答えは Parent organization の下に出し、ほかの入力と編集ページを残す", () => {
    it.each([
      {
        answer: "循環",
        status: 400,
        body: {
          code: "invalid_request",
          message: "x",
          reason: "organization_parent_cycle",
        },
        shown: "The parent can't be the organization itself or one inside it.",
      },
      {
        answer: "field: parentId の 404",
        status: 404,
        body: {
          code: "not_found",
          message: "x",
          reason: "organization_not_found",
          field: "parentId",
        },
        // The reason's text, read here as the parent's.
        shown: "This organization no longer exists.",
      },
    ])("$answer", async ({ status, body, shown }) => {
      server.patch.mockImplementation(() => jsonResponse(body, status));
      await opened();
      const user = userEvent.setup();
      await user.type(screen.getByRole("textbox", { name: "Location" }), " West");
      await chooseParent("Label");
      await user.click(screen.getByRole("button", { name: "Save" }));

      // The field's own error only: no band failure line and no gone state.
      const alert = await screen.findByRole("alert");
      const field = screen
        .getByRole("combobox", { name: "Parent organization" })
        .closest("[data-slot=field]");
      expect(alert.closest("[data-slot=field]")).toBe(field);
      expect(alert.textContent).toBe(shown);
      expect(
        screen.getByRole<HTMLInputElement>("textbox", { name: "Location" }).value,
      ).toBe("Tokyo West");
      expect(screen.queryByText("This organization no longer exists")).toBeNull();
      expect(where()).toBe("/organizations/5/edit");
    });
  });

  it("団体がもうなければ、団体ページの消えた状態を出す", async () => {
    server.organization = () =>
      jsonResponse(
        { code: "not_found", message: "x", reason: "organization_not_found" },
        404,
      );
    renderEditor();
    expect(await screen.findByText("This organization no longer exists")).toBeDefined();
    expect(screen.getByRole("link", { name: "Back to organizations" })).toBeDefined();
  });
});
