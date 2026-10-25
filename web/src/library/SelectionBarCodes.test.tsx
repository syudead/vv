import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import SelectionBar from "./SelectionBar";
import type { SelectionTarget } from "./selection";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function video(id: number, fileTitle: string): Video {
  return {
    id,
    virtual: false,
    title: `Video ${String(id)}`,
    fileTitle,
    contentVersion: `v${String(id)}`,
    sizeBytes: 0,
    addedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    fileCreatedAt: "2026-10-01T00:00:00Z",
    fileModifiedAt: "2026-10-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    tags: [],
    public: false,
  };
}

/** Video 1 has a code, 2 has none, and 9 is a member only the smart folder brings. */
const rows = [
  { video: video(1, "old name"), code: "XYZ-1" },
  { video: video(2, "ABC-2") },
  { video: video(9, "DEF-9") },
];

const targets: SelectionTarget[] = [
  { key: "v1", kind: "video", id: 1, label: "Video 1", videoIds: [1] },
  { key: "v2", kind: "video", id: 2, label: "Video 2", videoIds: [2] },
  { key: "s5", kind: "smartFolder", id: 5, label: "Folder", videoIds: [] },
];

interface Server {
  summaries: unknown[];
  applies: unknown[];
}

function install({
  summary = () => Promise.resolve(json({ rows })),
  apply = () => Promise.resolve(json({ applied: 2 })),
}: {
  summary?: () => Promise<Response>;
  apply?: () => Promise<Response>;
} = {}): Server {
  const server: Server = { summaries: [], applies: [] };
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const body: unknown = JSON.parse(String(init?.body));
    if (url === "/api/video-code-edits/summary") {
      server.summaries.push(body);
      return summary();
    }
    if (url === "/api/video-code-edits") {
      server.applies.push(body);
      return apply();
    }
    throw new Error(`unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return server;
}

function renderBar(count = 2) {
  render(
    <TooltipProvider>
      <ToastProvider>
        <SelectionBar
          count={count}
          targets={targets}
          selectedIds={[1, 2]}
          favoriteVideoIds={[1, 2]}
          favoriteGroupIds={[]}
          smartFolderIds={[5]}
          onClear={vi.fn()}
          onTagRemoved={vi.fn()}
          onBundled={vi.fn()}
          onCollectionsDeleted={vi.fn()}
        />
      </ToastProvider>
    </TooltipProvider>,
  );
  return userEvent.setup();
}

function editCodes(): HTMLButtonElement {
  return screen.getByRole("button", { name: "Edit codes" });
}

async function openEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(editCodes());
  return screen.findByRole("dialog", { name: "Edit codes" });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Edit codes on the selection bar", () => {
  it("is disabled with the limit as its reason over 1,000 known videos", () => {
    install();
    renderBar(1001);

    expect(editCodes().disabled).toBe(true);
    const reason = document.getElementById(editCodes().getAttribute("aria-describedby")!);
    expect(reason?.textContent).toBe("Select between 1 and 1,000 videos.");
  });

  it("reads the summary for the selection, with the bar busy, then opens the summary's rows", async () => {
    let answer: (response: Response) => void = () => undefined;
    const server = install({
      summary: () => new Promise((resolve) => (answer = resolve)),
    });
    const user = renderBar();

    await user.click(editCodes());

    expect(server.summaries).toEqual([{ videoIds: [1, 2], smartFolderIds: [5] }]);
    expect(editCodes().getAttribute("aria-busy")).toBe("true");
    const bar = screen.getByRole("region", { name: "Selection actions" });
    expect(
      within(bar).getByRole<HTMLButtonElement>("button", { name: "Edit" }).disabled,
    ).toBe(true);
    expect(
      within(bar).getByRole<HTMLButtonElement>("button", {
        name: "Clear selection (Esc)",
      }).disabled,
    ).toBe(true);
    expect(screen.queryByRole("dialog")).toBeNull();

    answer(json({ rows }));
    const dialog = await screen.findByRole("dialog", { name: "Edit codes" });
    const code = (title: string) =>
      within(dialog).getByRole<HTMLInputElement>("textbox", { name: `Code of ${title}` })
        .value;
    expect([code("Video 1"), code("Video 2"), code("Video 9")]).toEqual([
      "XYZ-1",
      "ABC-2",
      "DEF-9",
    ]);
  });

  it.each([
    [
      json(
        { code: "invalid_request", reason: "too_many_videos", limit: 1000, message: "x" },
        400,
      ),
      "Select between 1 and 1,000 videos.",
    ],
    [
      json({ code: "internal", message: "x" }, 500),
      /^Couldn't read the selected videos: /,
    ],
  ])("toasts a failed summary and opens no dialog (%#)", async (response, text) => {
    install({ summary: () => Promise.resolve(response) });
    const user = renderBar();

    await user.click(editCodes());

    expect(await screen.findByText(text)).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(editCodes().hasAttribute("aria-busy")).toBe(false);
  });

  it("applies only the suggested and changed rows, then closes and counts them", async () => {
    const server = install();
    const user = renderBar();
    const dialog = await openEditor(user);

    await user.click(within(dialog).getByRole("button", { name: "Apply to 2 videos" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.applies).toEqual([
      {
        items: [
          { videoId: 2, contentVersion: "v2", code: "ABC-2" },
          { videoId: 9, contentVersion: "v9", code: "DEF-9" },
        ],
      },
    ]);
    expect(await screen.findByText("Set the code of 2 videos")).toBeDefined();
    expect(screen.getByText("2 videos and 1 smart folder selected")).toBeDefined();
  });

  it.each([
    [404, "not_found", "video_not_found", "items[0].videoId"],
    [409, "conflict", "content_changed", "items[0].contentVersion"],
  ])(
    "keeps the editor and its values when Apply answers %i",
    async (status, code, reason, field) => {
      install({
        apply: () => Promise.resolve(json({ code, reason, message: "x", field }, status)),
      });
      const user = renderBar();
      const dialog = await openEditor(user);
      const input = within(dialog).getByRole<HTMLInputElement>("textbox", {
        name: "Code of Video 2",
      });
      await user.type(input, "x");

      await user.click(within(dialog).getByRole("button", { name: "Apply to 2 videos" }));

      expect(
        await within(dialog).findByText(/^Couldn't apply the codes: /),
      ).toBeDefined();
      expect(screen.getByRole("dialog", { name: "Edit codes" })).toBe(dialog);
      expect(input.value).toBe("ABC-2x");
      expect(input.getAttribute("aria-invalid")).toBe("true");
    },
  );
});
