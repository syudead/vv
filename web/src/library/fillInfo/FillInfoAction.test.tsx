import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkInfoPlugin } from "../../api/workInfoPlugins";
import {
  appliedResult,
  fetchResult,
  json,
  video,
  works,
} from "../../testing/workInfoFetchPage";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import FillInfoAction from "./FillInfoAction";

const videos: Record<string, unknown> = {
  "1": { ...video, id: 1, title: "Episode", tags: [] },
  "2": { ...video, id: 2, title: "Summer trip", displayName: "Summer trip" },
};

const searches: Record<string, unknown> = {
  "ABC-1": {
    candidates: [
      { id: "abc-10", url: "https://e.test/10", title: "Other" },
      { id: "abc-1", url: "https://e.test/1", title: "Work one" },
    ],
    match: 1,
  },
  "Summer trip": {
    candidates: [
      { id: "s-1", url: "https://e.test/s1", title: "Summer" },
      { id: "s-2", url: "https://e.test/s2", title: "Trip" },
    ],
  },
};

const fetches: Record<string, unknown> = {
  "abc-1": fetchResult({
    fetchId: "F1",
    title: "Work one",
    tags: ["drama"],
    code: "ABC-1",
    dates: [{ kind: "release", date: "2024-05-01" }],
  }),
  "s-2": fetchResult({ fetchId: "F2", title: "Trip", description: "notes" }),
};

function install(plugins: WorkInfoPlugin[]) {
  const writes: { url: string; body: unknown }[] = [];
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const body = (
      init?.body === undefined ? {} : JSON.parse(String(init.body))
    ) as Record<string, string>;
    if (url === "/api/work-info-plugins") return Promise.resolve(json({ plugins }));
    if (url === "/api/work-info-fill-targets") {
      writes.push({ url, body });
      return Promise.resolve(
        json({
          targets: [
            { videoId: 1, title: "Episode", query: "ABC-1" },
            { videoId: 2, title: "Summer trip", query: "Summer trip" },
          ],
        }),
      );
    }
    if (url.endsWith("/search")) return Promise.resolve(json(searches[body.query ?? ""]));
    const fetchOf = /^\/api\/videos\/(\d+)\/work-info-fetches$/.exec(url);
    if (fetchOf !== null) return Promise.resolve(json(fetches[body.id ?? ""], 201));
    const videoOf = /^\/api\/videos\/(\d+)$/.exec(url);
    if (videoOf !== null) return Promise.resolve(json(videos[videoOf[1] ?? ""]));
    if (url.endsWith("/apply")) {
      writes.push({ url, body });
      return Promise.resolve(json(appliedResult(video)));
    }
    throw new Error(`想定しない要求: ${init?.method ?? "GET"} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return writes;
}

function renderAction() {
  render(
    <TooltipProvider>
      <ToastProvider>
        <FillInfoAction
          videoIds={[1, 2]}
          smartFolderIds={[9]}
          disabledReason={undefined}
        />
      </ToastProvider>
    </TooltipProvider>,
  );
}

beforeEach(() => {
  // jsdom lacks the pointer capture API that Radix calls.
  Element.prototype.hasPointerCapture ??= () => false;
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("FillInfoAction", () => {
  it("選んだ作品を品番と発売日で示し、確認した結果を反映する", async () => {
    const user = userEvent.setup();
    const writes = install([
      { ...works, id: 5, name: "fetch only", operations: ["fetch"] },
      { ...works, id: 6, name: "people", kind: "person" },
      works,
    ]);
    renderAction();
    await user.click(screen.getByRole("button", { name: "Fill info…" }));
    expect((await screen.findByRole("combobox", { name: "Plugin" })).textContent).toBe(
      works.name,
    );
    await user.click(screen.getByRole("button", { name: "Search" }));

    const list = await screen.findByRole("list", { name: "Videos to fill" });
    const [episode, trip] = within(list).getAllByRole("listitem");
    if (episode === undefined || trip === undefined) throw new Error("two rows expected");
    await within(episode).findByText("ABC-1 · 2024-05-01");
    expect(within(episode).queryByText("drama")).toBeNull();
    const unmatched = await within(trip).findByRole("combobox", {
      name: "Work for Summer trip",
    });
    expect(unmatched.textContent).toBe("Don't fill");
    await user.click(unmatched);
    await user.click(await screen.findByRole("option", { name: "Trip" }));
    await user.click(await screen.findByRole("button", { name: "Fill 2 videos" }));
    await screen.findByText("Filled 2 videos");
    expect(writes).toEqual([
      {
        url: "/api/work-info-fill-targets",
        body: { videoIds: [1, 2], smartFolderIds: [9] },
      },
      {
        url: "/api/videos/1/work-info-fetches/F1/apply",
        body: { fields: ["title", "code", "dates.release", "tags"], tags: ["drama"] },
      },
      {
        url: "/api/videos/2/work-info-fetches/F2/apply",
        body: { fields: ["description"] },
      },
    ]);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("検索と取得ができるプラグインがなければ理由を示し、始められない", async () => {
    install([{ ...works, state: "unapproved" }]);
    renderAction();
    await userEvent.click(screen.getByRole("button", { name: "Fill info…" }));
    await screen.findByText(
      "No approved plugin both searches and fetches works. Add or approve one in Settings.",
    );
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Search" }).disabled,
    ).toBe(true);
  });
});
