import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagsForTest } from "../../api/tags";
import type { VideoEditSummary } from "../../api/videoEdits";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import type { SelectionTarget } from "../selection";
import BulkEditor from "./BulkEditor";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const fullSummary: VideoEditSummary = {
  total: 3,
  public: 0,
  tags: [
    {
      tag: { id: 1, name: "Travel", tentative: false, kind: null },
      count: 2,
      manualCount: 2,
    },
  ],
  people: [
    { role: "performer", person: { id: 7, name: "Alice", provisional: false }, count: 3 },
  ],
  organizations: [],
  dates: [],
};

const server = {
  summary: vi.fn<(body: unknown) => Response>(),
  apply: vi.fn<(body: unknown) => Response>(),
  matches: vi.fn<(body: unknown) => Response>(),
  undo: vi.fn<(url: string) => Response>(),
};

beforeEach(() => {
  __resetTagsForTest();
  // A Response body is read once, so build one per call.
  server.summary.mockReset().mockImplementation(() => json(fullSummary));
  server.apply.mockReset().mockImplementation(() => json({ applied: 3, tags: [] }));
  server.matches.mockReset().mockImplementation(() => json({ items: [] }));
  server.undo.mockReset().mockImplementation(() => json({}));
  // jsdom lacks the pointer capture API that Radix calls.
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = String(input);
      const body: unknown =
        init?.body === undefined ? undefined : JSON.parse(String(init.body));
      if (url === "/api/tags") return Promise.resolve(json({ items: [] }));
      if (url === "/api/video-edits/summary")
        return Promise.resolve(server.summary(body));
      if (url === "/api/video-edits") return Promise.resolve(server.apply(body));
      if (url === "/api/video-edits/provisional-matches")
        return Promise.resolve(server.matches(body));
      if (url.startsWith("/api/activity/")) return Promise.resolve(server.undo(url));
      return Promise.resolve(json({ code: "not_found", message: "not found" }, 404));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const targets: SelectionTarget[] = [
  { key: "v1", kind: "video", id: 1, label: "One", videoIds: [1] },
  { key: "g5", kind: "group", id: 5, label: "Series", videoIds: [2, 3] },
  { key: "s9", kind: "smartFolder", id: 9, label: "Favs", videoIds: [] },
];

function renderEditor() {
  const onClose = vi.fn<() => void>();
  const onApplied = vi.fn<(removed: readonly number[], partial: unknown) => void>();
  render(
    <ToastProvider>
      <TooltipProvider>
        <BulkEditor targets={targets} onClose={onClose} onApplied={onApplied} />
      </TooltipProvider>
    </ToastProvider>,
  );
  return { onClose, onApplied };
}

describe("BulkEditor", () => {
  it("shows what the selection carries, and pressing values builds one request that Apply sends", async () => {
    const user = userEvent.setup();
    const { onClose, onApplied } = renderEditor();

    const tags = await screen.findByRole("list", { name: "Tags" });
    const travel = within(tags).getByRole("button", {
      name: /^Travel, on 2 of 3 videos/,
    });
    await user.click(travel); // add to every video
    await user.click(travel); // remove from every video
    const performers = screen.getByRole("list", { name: "Performers" });
    await user.click(
      within(performers).getByRole("button", { name: /^Alice, on 3 of 3 videos/ }),
    );
    await user.click(screen.getByRole("radio", { name: "Public" }));

    const changes = screen.getByRole("list", { name: "Changes" });
    expect(
      within(changes)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Tags: remove Travel from 2 videos",
      "Performers: remove Alice from 3 videos",
      "Visibility: make 3 videos public",
    ]);
    await user.click(screen.getByRole("button", { name: "Apply to 3 videos" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(server.summary).toHaveBeenCalledWith({
      videoIds: [1, 2, 3],
      smartFolderIds: [9],
    });
    expect(server.apply).toHaveBeenCalledWith({
      videoIds: [1, 2, 3],
      smartFolderIds: [9],
      tags: { remove: [1] },
      credits: [{ kind: "person", role: "performer", remove: [7] }],
      public: true,
    });
    expect(onApplied).toHaveBeenCalledWith([1], null);
    expect(await screen.findByText("Updated 3 videos")).toBeTruthy();
  });

  it("leaves a pressed card out of both the summary and the request", async () => {
    const user = userEvent.setup();
    renderEditor();
    await screen.findByRole("list", { name: "Tags" });

    await user.click(screen.getByRole("button", { name: "Series, edited" }));
    expect(screen.getByRole("button", { name: "Series, left out" })).toBeTruthy();
    await waitFor(() =>
      expect(server.summary).toHaveBeenLastCalledWith({
        videoIds: [1],
        smartFolderIds: [9],
      }),
    );
    await user.click(
      within(await screen.findByRole("list", { name: "Tags" })).getByRole("button", {
        name: /^Travel/,
      }),
    );
    await user.click(screen.getByRole("button", { name: /^Apply to/ }));
    await waitFor(() => expect(server.apply).toHaveBeenCalled());
    expect(server.apply.mock.calls[0]?.[0]).toMatchObject({
      videoIds: [1],
      smartFolderIds: [9],
    });
  });

  it("draws only the most common values and reaches the rest through Show all or the filter", async () => {
    const many = Array.from({ length: 30 }, (_, index) => ({
      tag: {
        id: index + 1,
        name: `Tag ${String(index + 1)}`,
        tentative: false,
        kind: null,
      },
      count: index === 29 ? 3 : 1,
      manualCount: index === 29 ? 3 : 1,
    }));
    server.summary.mockImplementation(() => json({ ...fullSummary, tags: many }));
    const user = userEvent.setup();
    renderEditor();

    const tags = await screen.findByRole("list", { name: "Tags" });
    const chips = () => within(tags).getAllByRole("button", { name: /^Tag \d+, on / });
    // The 24 most common, in name order: Tag 30 is on every video, so Tag 24 is the one left out.
    const names = () =>
      chips().map((chip) => chip.getAttribute("aria-label")?.split(",")[0]);
    expect(names()).toHaveLength(24);
    expect(names()).toContain("Tag 30");
    expect(names()).not.toContain("Tag 24");
    expect(names()[0]).toBe("Tag 1");

    await user.type(screen.getByRole("searchbox", { name: "Filter tags" }), "Tag 28");
    expect(chips().map((chip) => chip.getAttribute("aria-label"))).toEqual([
      expect.stringMatching(/^Tag 28, /),
    ]);
    await user.clear(screen.getByRole("searchbox", { name: "Filter tags" }));
    await user.click(screen.getByRole("button", { name: "Show all 30" }));
    expect(chips()).toHaveLength(30);
    await user.click(screen.getByRole("button", { name: "Show fewer" }));
    expect(chips()).toHaveLength(24);
  });

  it("keeps the editor and its changes after a failure, and asks before discarding them", async () => {
    server.apply.mockReturnValue(
      json({ code: "not_found", reason: "person_not_found", message: "gone" }, 404),
    );
    const user = userEvent.setup();
    const { onClose } = renderEditor();
    const tags = await screen.findByRole("list", { name: "Tags" });
    await user.click(within(tags).getByRole("button", { name: /^Travel/ }));
    await user.click(screen.getByRole("button", { name: "Apply to 3 videos" }));

    expect(await screen.findByText(/^Couldn't apply the changes:/)).toBeTruthy();
    expect(screen.getByText("Tags: add Travel to 1 video")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    const confirm = await screen.findByRole("alertdialog", {
      name: "Discard these changes?",
    });
    expect(onClose).not.toHaveBeenCalled();
    await user.click(within(confirm).getByRole("button", { name: "Discard" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("suggests merging an unnamed performer into the added one, sends the merge and offers its undo", async () => {
    server.summary.mockImplementation(() =>
      json({
        ...fullSummary,
        people: [
          {
            role: "performer",
            person: { id: 7, name: "Alice", provisional: false },
            count: 1,
          },
          {
            role: "performer",
            person: { id: 21, name: "", provisional: true },
            count: 2,
          },
          {
            role: "performer",
            person: { id: 22, name: "", provisional: true },
            count: 1,
          },
        ],
      }),
    );
    server.matches.mockImplementation(() =>
      json({
        items: [
          { personId: 21, alone: true, coPerformers: [], similarTo: [] },
          { personId: 22, alone: false, coPerformers: [], similarTo: [] },
        ],
      }),
    );
    server.apply.mockImplementation(() =>
      json({
        applied: 3,
        tags: [],
        merged: [{ personId: 21, into: 7, undoActivityId: 40 }],
      }),
    );
    // The mock onClose keeps the modal editor mounted, which blocks pointer events on the toast.
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderEditor();

    const performers = await screen.findByRole("list", { name: "Performers" });
    await user.click(
      within(performers).getByRole("button", { name: /^Alice, on 1 of 3/ }),
    );
    const unnamed = await screen.findByRole("list", { name: "Unnamed performers" });
    expect(
      within(unnamed)
        .getAllByRole("button")
        .map((tile) => tile.getAttribute("aria-label")),
    ).toEqual([
      "#21, on 2 of 3 videos, will be merged into Alice",
      "#22, on 1 of 3 videos, kept apart",
    ]);
    // The unnamed performers are compared too, so one face can lead to another's target.
    expect(server.matches).toHaveBeenCalledWith({
      videoIds: [1, 2, 3],
      smartFolderIds: [9],
      personIds: [7, 21, 22],
    });
    expect(
      screen.getByText("Performers: merge #21 into Alice, on 2 videos here"),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Apply to 3 videos" }));
    await waitFor(() => expect(server.apply).toHaveBeenCalled());
    expect(server.apply.mock.calls[0]?.[0]).toMatchObject({
      credits: [{ kind: "person", role: "performer", add: [7] }],
      merges: [{ into: 7, sourceIds: [21] }],
    });
    expect(
      await screen.findByText("Updated 3 videos and merged 1 unnamed person."),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo merges" }));
    await waitFor(() =>
      expect(server.undo).toHaveBeenCalledWith("/api/activity/40/undo"),
    );
  });

  it("names an unnamed person picked by face on Apply and merges the same faces into them", async () => {
    server.summary.mockImplementation(() =>
      json({
        ...fullSummary,
        people: [21, 22].map((id) => ({
          role: "performer",
          person: { id, name: "", provisional: true },
          count: 1,
        })),
      }),
    );
    server.matches.mockImplementation(() =>
      json({
        items: [
          { personId: 21, alone: true, coPerformers: [], similarTo: [] },
          { personId: 22, alone: true, coPerformers: [], similarTo: [21] },
        ],
      }),
    );
    const sent: string[] = [];
    const base = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = String(input);
        sent.push(`${init?.method ?? "GET"} ${url.split("?")[0] ?? url}`);
        if (url.startsWith("/api/people?"))
          return Promise.resolve(
            json({
              items: url.includes("provisional=true")
                ? [{ id: 21, name: "", provisional: true, videoCount: 1 }]
                : [],
            }),
          );
        if (url === "/api/people/21" && init?.method === "PATCH")
          return Promise.resolve(json({ id: 21, name: "Mio", provisional: false }));
        return base(input, init);
      }),
    );
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderEditor();

    await user.click(await screen.findByPlaceholderText("Add a performer"));
    await user.keyboard("Mio");
    await user.click(await screen.findByRole("option", { name: /^#21, .*Add as Mio/ }));

    expect(await screen.findByText("Performers: add Mio to 2 videos")).toBeTruthy();
    const unnamed = await screen.findByRole("list", { name: "Unnamed performers" });
    expect(
      within(unnamed)
        .getAllByRole("button")
        .map((tile) => tile.getAttribute("aria-label")),
    ).toEqual(["#22, on 1 of 3 videos, will be merged into Mio"]);

    await user.click(screen.getByRole("button", { name: "Apply to 3 videos" }));
    await waitFor(() => expect(server.apply).toHaveBeenCalled());
    // The person is named before the edit that merges into them.
    expect(sent.filter((line) => !line.startsWith("GET"))).toEqual(
      expect.arrayContaining(["PATCH /api/people/21"]),
    );
    expect(sent.indexOf("PATCH /api/people/21")).toBeLessThan(
      sent.indexOf("POST /api/video-edits"),
    );
    expect(server.apply.mock.calls[0]?.[0]).toMatchObject({
      credits: [{ kind: "person", role: "performer", add: [21] }],
      merges: [{ into: 21, sourceIds: [22] }],
    });
  });
});
