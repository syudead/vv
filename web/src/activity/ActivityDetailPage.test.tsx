import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ActivityDetail } from "../api/activity";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import ActivityDetailPage from "./ActivityDetailPage";

const at = "2026-10-18T10:00:00Z";

function record(id: number, extra: Partial<ActivityDetail> = {}): ActivityDetail {
  return {
    id,
    kind: "import",
    feature: "import",
    status: "done",
    outcome: "completed",
    decider: "rule",
    subject: { kind: "video", items: [{ label: "clip.mp4", videoId: 7 }] },
    candidate: false,
    retryRun: false,
    undoable: false,
    undone: false,
    createdAt: at,
    updatedAt: at,
    ...extra,
  };
}

const message = `ffmpeg exited with status 1\n${"x".repeat(300)}`;

const server = {
  entries: new Map<number, ActivityDetail>(),
  reads: [] as number[],
  posts: [] as string[],
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function install() {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (init?.method === "POST") {
        server.posts.push(url.pathname);
        return Promise.resolve(new Response(null, { status: 202 }));
      }
      const id = Number(url.pathname.split("/").at(-1));
      server.reads.push(id);
      const found = server.entries.get(id);
      return Promise.resolve(
        found === undefined
          ? json({ code: "not_found", message: "not found" }, 404)
          : json(found),
      );
    }),
  );
}

function renderDetail(id: number) {
  render(
    <MemoryRouter initialEntries={[`/activity/${String(id)}`]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/activity/:id" element={<ActivityDetailPage />} />
          </Routes>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function section(name: string): HTMLElement {
  return screen.getByRole("region", { name });
}

beforeEach(() => {
  server.entries = new Map([
    [
      1,
      record(1, {
        status: "attention",
        reason: "failed",
        stage: "thumbnail",
        detail: message,
        retryRun: true,
        stagesRecorded: true,
        stages: [
          { stage: "probe", state: "done", updatedAt: at },
          { stage: "thumbnail", state: "failed", detail: message, updatedAt: at },
        ],
      }),
    ],
    [
      10,
      record(10, {
        kind: "pipeline",
        status: "attention",
        reason: "failed",
        members: [
          record(11, { replaced: true, status: "done", outcome: "superseded" }),
          record(12, {
            kind: "auto_tag",
            feature: "auto_tagging",
            decider: "ai",
            status: "attention",
            reason: "failed",
            retryRun: true,
          }),
        ],
      }),
    ],
  ]);
  server.reads = [];
  server.posts = [];
  installFakeEventSource();
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("ActivityDetailPage", () => {
  it("shows the full message of a failed entry with its stages", async () => {
    renderDetail(1);
    expect(
      await screen.findByRole("heading", { level: 1, name: "clip.mp4" }),
    ).toBeDefined();
    const notice = screen.getByRole("alert");
    expect(within(notice).getByText("Failed at thumbnail")).toBeDefined();
    expect(
      within(notice).getByText(
        (_, node) => node?.tagName === "P" && node.textContent === message,
      ),
    ).toBeDefined();
    const stages = section("Stages");
    expect(within(stages).getAllByText("Not part of this run")).toHaveLength(3);
    expect(
      within(stages).getByText(
        (_, node) => node?.tagName === "P" && node.textContent === message,
      ),
    ).toBeDefined();
    expect(screen.getByRole("link", { name: "Open" }).getAttribute("href")).toBe(
      "/videos/7",
    );
    await waitFor(() => expect(document.title).toBe("clip.mp4 · Activity"));
  });

  it("rereads the entry on an activity notification", async () => {
    renderDetail(1);
    await screen.findByRole("alert");
    server.entries.set(1, record(1, { stagesRecorded: true, stages: [] }));
    await emitServerEvent("activity");
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(server.reads.filter((id) => id === 1).length).toBeGreaterThan(1);
    expect(within(section("Summary")).getByRole("img", { name: "Done" })).toBeDefined();
  });

  it("retries a pipeline's member by that member's id", async () => {
    const user = userEvent.setup();
    renderDetail(10);
    const tasks = await screen.findByRole("region", { name: "Tasks" });
    expect(within(tasks).getByText("Replaced by a newer run")).toBeDefined();
    const retries = within(tasks).getAllByRole("button", { name: "Retry" });
    expect(retries).toHaveLength(1);
    await user.click(retries[0]!);
    await waitFor(() => expect(server.posts).toEqual(["/api/activity/12/retry"]));
  });

  it("says the entry doesn't exist when the server has no such id", async () => {
    renderDetail(99);
    expect(await screen.findByText("This activity entry doesn't exist")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Back to Activity" }).getAttribute("href"),
    ).toBe("/activity");
  });
});
