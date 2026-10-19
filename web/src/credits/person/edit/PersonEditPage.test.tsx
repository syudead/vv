import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useParams } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Person } from "../../../api/credits";
import { TooltipProvider } from "../../../ui/shadcn/tooltip";
import { useHandedPerson } from "../personHandoff";
import PersonEditPage from "./PersonEditPage";

// Opened by its URL; the person page's Edit link and Back along the history reuse the video editor's leaving hook.

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const aoi: Person = {
  id: 7,
  name: "Aoi",
  provisional: false,
  aliases: [{ name: "あおい", language: "ja" }],
  externalIds: [{ source: "fanza", value: "123" }],
  profile: { birthDate: "1990-05-01", birthplace: "Tokyo", cup: "C", heightCm: 158 },
  favorite: false,
  videoCount: 12,
  createdAt: "2026-01-01T00:00:00Z",
};

const server = {
  person: (): Response => json(aoi),
  patch: vi.fn<(body: unknown) => Response>(),
};

/** Stands in for the person page: shows the person the editor handed back, if any. */
function PersonPageStub() {
  const id = Number(useParams().id);
  const handed = useHandedPerson(id);
  return <p>person page {handed?.name ?? "read again"}</p>;
}

function Where() {
  return <p data-testid="where">{useLocation().pathname}</p>;
}

function renderEditor() {
  render(
    <TooltipProvider>
      <MemoryRouter initialEntries={["/people", "/people/7/edit"]} initialIndex={1}>
        <Where />
        <Routes>
          <Route path="/people/:id/edit" element={<PersonEditPage />} />
          <Route path="/people/:id" element={<PersonPageStub />} />
          <Route path="/people" element={<p>people</p>} />
        </Routes>
      </MemoryRouter>
    </TooltipProvider>,
  );
}

async function opened() {
  renderEditor();
  return screen.findByRole<HTMLInputElement>("textbox", { name: "Name" });
}

function where() {
  return screen.getByTestId("where").textContent;
}

describe("PersonEditPage", () => {
  beforeEach(() => {
    server.person = () => json(aoi);
    server.patch.mockReset();
    server.patch.mockImplementation(() => json({ ...aoi, name: "Aoi Sora" }));
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = String(input);
        if (url === "/api/people/7" && init?.method === "PATCH")
          return Promise.resolve(server.patch(JSON.parse(String(init.body))));
        if (url === "/api/people/7") return Promise.resolve(server.person());
        return Promise.resolve(json({}));
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("1 回の PATCH で変えた property だけを送り、応答を持って人物ページへ戻る", async () => {
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Name" }), " Sora");
    await user.clear(screen.getByRole("textbox", { name: "Birthplace" }));
    await user.type(screen.getByRole("textbox", { name: "Waist (cm)" }), "5x8");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("person page Aoi Sora")).toBeDefined();
    expect(where()).toBe("/people/7");
    expect(server.patch).toHaveBeenCalledTimes(1);
    expect(server.patch.mock.calls[0]![0]).toEqual({
      name: "Aoi Sora",
      profile: { birthplace: null, waistCm: 58 },
    });
  });

  it("項目のある理由はその項目の下に出し、ほかの入力を残す", async () => {
    server.patch.mockImplementation(() =>
      json(
        {
          code: "invalid_request",
          message: "x",
          reason: "work_info_out_of_range",
          field: "profile.deathDate",
        },
        400,
      ),
    );
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Bust (cm)" }), "85");
    await user.type(screen.getByRole("textbox", { name: "Death date" }), "1980");
    await user.click(screen.getByRole("button", { name: "Save" }));

    const death = screen.getByRole<HTMLInputElement>("textbox", { name: "Death date" });
    await waitFor(() => expect(death.getAttribute("aria-invalid")).toBe("true"));
    expect(
      document.getElementById(death.getAttribute("aria-describedby")!)?.textContent,
    ).toBe("This value is outside the allowed range.");
    // Only the field's own error, no band failure line.
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(
      screen.getByRole<HTMLInputElement>("textbox", { name: "Bust (cm)" }).value,
    ).toBe("85");
    expect(where()).toBe("/people/7/edit");
  });

  it("変えたまま Cancel で離れると Discard changes? を確かめ、Discard で送らずに戻る", async () => {
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Cup" }), "D");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    expect(
      within(dialog).getByText('Your changes to "Aoi" aren\'t saved.'),
    ).toBeDefined();
    expect(where()).toBe("/people/7/edit");
    await user.click(within(dialog).getByRole("button", { name: "Discard" }));
    expect(await screen.findByText("person page read again")).toBeDefined();
    expect(server.patch).not.toHaveBeenCalled();
  });

  describe("仮の人物", () => {
    const unnamed: Person = { ...aoi, name: "", provisional: true, aliases: [] };

    beforeEach(() => {
      server.person = () => json(unnamed);
      server.patch.mockImplementation(() =>
        json({ ...unnamed, name: "Mio", provisional: false }),
      );
    });

    it("名前を空のまま placeholder で出し、空のまま Save しても name を送らない", async () => {
      const name = await opened();
      expect(name.value).toBe("");
      expect(name.getAttribute("placeholder")).toBe("Unnamed person");
      expect(document.activeElement).toBe(name);

      await userEvent.setup().click(screen.getByRole("button", { name: "Save" }));

      expect(await screen.findByText(/^person page/)).toBeDefined();
      expect(server.patch).not.toHaveBeenCalled();
    });

    it("入力した名前を PATCH の name で送る", async () => {
      const name = await opened();
      const user = userEvent.setup();
      await user.type(name, "Mio");
      await user.click(screen.getByRole("button", { name: "Save" }));

      expect(await screen.findByText("person page Mio")).toBeDefined();
      expect(server.patch.mock.calls[0]![0]).toEqual({ name: "Mio" });
    });
  });

  it("URL で開いて人物がいなければ、人物ページの消えた状態を出す", async () => {
    server.person = () =>
      json({ code: "not_found", message: "x", reason: "person_not_found" }, 404);
    renderEditor();
    expect(await screen.findByText("This person no longer exists")).toBeDefined();
    expect(screen.getByRole("link", { name: "Back to people" })).toBeDefined();
  });
});
