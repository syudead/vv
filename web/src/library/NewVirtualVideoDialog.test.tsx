import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import NewVirtualVideoDialog from "./NewVirtualVideoDialog";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function VideoPageProbe() {
  const state = useLocation().state as { from?: string } | null;
  return <p>video page from {state?.from}</p>;
}

describe("NewVirtualVideoDialog", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let created: unknown[] = [];

  beforeEach(() => {
    created = [];
    fetchMock.mockImplementation((_input, init) => {
      const body = JSON.parse(String(init?.body)) as { url?: string };
      created.push(body);
      if (body.url === "https://bad.test/")
        return Promise.resolve(json({ code: "invalid_request", message: "x" }, 400));
      return Promise.resolve(json({ id: 7, virtual: true, title: body.url }, 201));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    fetchMock.mockReset();
    vi.unstubAllGlobals();
  });

  it("リンクだけでも作れ、http でないリンクとサーバーが断ったリンクはリンクの欄に理由を出す", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route
            path="/"
            element={<NewVirtualVideoDialog backTo="/" onClose={() => undefined} />}
          />
          <Route path="/videos/7" element={<VideoPageProbe />} />
        </Routes>
      </MemoryRouter>,
    );
    const dialog = await screen.findByRole("dialog", { name: "New virtual video" });
    const link = within(dialog).getByRole("textbox", { name: "Link" });
    await user.type(link, "example.com/watch");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(
      within(dialog).getByText("Enter a link that starts with http:// or https://."),
    ).toBeDefined();
    expect(created).toHaveLength(0);

    await user.clear(link);
    await user.type(link, "https://bad.test/{Enter}");
    expect(
      await within(dialog).findByText(
        "Enter a link that starts with http:// or https://.",
      ),
    ).toBeDefined();

    await user.clear(link);
    await user.type(link, " https://example.com/watch?v=1 {Enter}");
    await waitFor(() =>
      expect(created).toEqual([
        { url: "https://bad.test/" },
        { url: "https://example.com/watch?v=1" },
      ]),
    );
    expect(await screen.findByText("video page from /")).toBeDefined();
  });
});
