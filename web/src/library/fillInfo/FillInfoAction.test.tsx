import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkInfoPlugin } from "../../api/workInfoPlugins";
import { works } from "../../testing/workInfoFetchPage";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import FillInfoAction from "./FillInfoAction";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

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

function install(plugins: WorkInfoPlugin[]) {
  const requests: { url: string; body: unknown }[] = [];
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const body: unknown =
      init?.body === undefined ? undefined : JSON.parse(String(init.body));
    if (url === "/api/work-info-plugins") {
      return Promise.resolve(jsonResponse({ plugins }));
    }
    requests.push({ url, body });
    if (url === "/api/work-info-fill-targets") {
      return Promise.resolve(
        jsonResponse({
          targets: [
            { videoId: 1, title: "Episode", query: "ABC-1" },
            { videoId: 2, title: "Summer trip", query: "Summer trip" },
          ],
        }),
      );
    }
    if (url.endsWith("/search")) {
      return Promise.resolve(jsonResponse(searches[(body as { query: string }).query]));
    }
    if (url.endsWith("/fills")) {
      return Promise.resolve(jsonResponse({ queued: 2 }, 202));
    }
    throw new Error(`想定しない要求: ${init?.method ?? "GET"} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return requests;
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
  it("検索結果を確かめ、一致しなかった動画の候補を選んでから補完を始める", async () => {
    const user = userEvent.setup();
    const requests = install([
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
    const matched = await within(list).findByRole("combobox", {
      name: "Work for Episode",
    });
    expect(matched.textContent).toBe("Work one");
    const unmatched = await within(list).findByRole("combobox", {
      name: "Work for Summer trip",
    });
    expect(unmatched.textContent).toBe("Don't fill");
    await user.click(unmatched);
    await user.click(await screen.findByRole("option", { name: "Trip" }));
    await user.click(screen.getByRole("button", { name: "Fill 2 videos" }));

    await screen.findByText("Filling 2 videos in the background");
    expect(requests).toEqual([
      {
        url: "/api/work-info-fill-targets",
        body: { videoIds: [1, 2], smartFolderIds: [9] },
      },
      {
        url: `/api/work-info-plugins/${String(works.id)}/search`,
        body: { query: "ABC-1" },
      },
      {
        url: `/api/work-info-plugins/${String(works.id)}/search`,
        body: { query: "Summer trip" },
      },
      {
        url: `/api/work-info-plugins/${String(works.id)}/fills`,
        body: {
          items: [
            { videoId: 1, workId: "abc-1" },
            { videoId: 2, workId: "s-2" },
          ],
        },
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
