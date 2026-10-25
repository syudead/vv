import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { WorkInfoPlugin } from "../api/workInfoPlugins";
import { works } from "../testing/workInfoFetchPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import FillInfoAction from "./FillInfoAction";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function install(plugins: WorkInfoPlugin[]) {
  const fills: { url: string; body: unknown }[] = [];
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    if (url === "/api/work-info-plugins") {
      return Promise.resolve(jsonResponse({ plugins }));
    }
    if (url.endsWith("/fills") && init?.method === "POST") {
      fills.push({ url, body: JSON.parse(String(init.body)) });
      return Promise.resolve(jsonResponse({ queued: 2 }, 202));
    }
    throw new Error(`想定しない要求: ${init?.method ?? "GET"} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fills;
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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("FillInfoAction", () => {
  it("検索と取得ができる承認済みプラグインで選択の補完を始め、件数を知らせる", async () => {
    const fills = install([
      { ...works, id: 5, name: "search only", operations: ["search"] },
      { ...works, id: 6, name: "people", kind: "person" },
      { ...works, id: 7, name: "vv.link", operations: ["fetch"], builtin: true },
      { ...works, id: 8, name: "unapproved", state: "unapproved" },
      works,
    ]);
    renderAction();
    await userEvent.click(screen.getByRole("button", { name: "Fill info…" }));
    expect((await screen.findByRole("combobox", { name: "Plugin" })).textContent).toBe(
      works.name,
    );
    await userEvent.click(screen.getByRole("button", { name: "Start" }));

    await screen.findByText("Filling 2 videos in the background");
    expect(fills).toEqual([
      {
        url: `/api/work-info-plugins/${String(works.id)}/fills`,
        body: { videoIds: [1, 2], smartFolderIds: [9] },
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
      screen.getByRole<HTMLButtonElement>("button", { name: "Start" }).disabled,
    ).toBe(true);
  });
});
