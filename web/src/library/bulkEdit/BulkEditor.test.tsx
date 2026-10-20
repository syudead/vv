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
};

beforeEach(() => {
  __resetTagsForTest();
  // A Response body is read once, so build one per call.
  server.summary.mockReset().mockImplementation(() => json(fullSummary));
  server.apply.mockReset().mockImplementation(() => json({ applied: 3, tags: [] }));
  // jsdom lacks the pointer capture API that Radix calls.
  Element.prototype.hasPointerCapture ??= () => false;
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
  const onApplied = vi.fn<(removed: readonly number[]) => void>();
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
    expect(onApplied).toHaveBeenCalledWith([1]);
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

    await user.click(screen.getByRole("button", { name: "Close" }));
    const confirm = await screen.findByRole("alertdialog", {
      name: "Discard these changes?",
    });
    expect(onClose).not.toHaveBeenCalled();
    await user.click(within(confirm).getByRole("button", { name: "Discard" }));
    expect(onClose).toHaveBeenCalled();
  });
});
