import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AutoTaggingSettings } from "../api/client";
import AutoTaggingSection, { parseThreshold } from "./AutoTaggingSection";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

const URL = "/api/settings/auto-tagging";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings(change: Partial<AutoTaggingSettings> = {}): AutoTaggingSettings {
  return {
    enabled: true,
    afterImport: false,
    threshold: 0.8,
    tentativeThreshold: 0.5,
    queue: { queued: 0, running: 0, done: 0, failed: 0 },
    ...change,
  };
}

describe("parseThreshold", () => {
  it("accepts numbers above 0 up to 1", () => {
    expect(parseThreshold("0.5")).toBe(0.5);
    expect(parseThreshold("1")).toBe(1);
    expect(parseThreshold("0")).toBeNull();
    expect(parseThreshold("1.2")).toBeNull();
    expect(parseThreshold("")).toBeNull();
    expect(parseThreshold("abc")).toBeNull();
  });
});

describe("AutoTaggingSection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: AutoTaggingSettings;
  let saved: unknown;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === URL && init?.method === "PUT") {
      saved = JSON.parse(String(init.body));
      current = { ...current, ...(saved as Partial<AutoTaggingSettings>) };
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    if (url === "/api/auto-tagging/runs")
      return Promise.resolve(json({ queued: 4 }, 202));
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  beforeEach(() => {
    current = settings({
      queue: {
        queued: 0,
        running: 0,
        done: 3,
        failed: 1,
        lastError: "model 'clef' not found",
      },
    });
    saved = undefined;
    toast.mockReset();
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the stored settings, the queue counts and the last failure", async () => {
    render(<AutoTaggingSection />);
    const enabled = await screen.findByRole("switch", { name: "Use auto-tagging" });
    expect(enabled.getAttribute("aria-checked")).toBe("true");
    expect(
      screen
        .getByRole("switch", { name: "Tag new videos automatically" })
        .getAttribute("aria-checked"),
    ).toBe("false");
    expect(screen.queryByLabelText("Ollama URL")).toBeNull();
    expect(screen.getByText("Waiting 0 · Tagging 0 · Done 3 · Failed 1")).toBeTruthy();
    expect(screen.getByText("model 'clef' not found")).toBeTruthy();
  });

  it("hides the queue and the run buttons while auto-tagging is off", async () => {
    current = settings({ enabled: false });
    render(<AutoTaggingSection />);
    await screen.findByRole("switch", { name: "Use auto-tagging" });
    expect(
      screen.queryByRole("button", { name: "Tag videos not yet tagged" }),
    ).toBeNull();
    expect(screen.queryByText(/^Waiting/)).toBeNull();
  });

  it("saves the edited values", async () => {
    const user = userEvent.setup();
    current = settings({ enabled: false });
    render(<AutoTaggingSection />);
    await user.click(await screen.findByRole("switch", { name: "Use auto-tagging" }));
    await user.click(
      screen.getByRole("switch", { name: "Tag new videos automatically" }),
    );
    const threshold = screen.getByLabelText("Probability to add");
    await user.clear(threshold);
    await user.type(threshold, "0.6");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith("Auto-tagging settings saved"),
    );
    expect(saved).toEqual({
      enabled: true,
      afterImport: true,
      threshold: 0.6,
      tentativeThreshold: 0.5,
    });
    expect(
      await screen.findByRole("button", { name: "Tag videos not yet tagged" }),
    ).toBeTruthy();
  });

  it("does not save an out-of-range threshold", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    const threshold = await screen.findByLabelText("Probability to add");
    await user.clear(threshold);
    await user.type(threshold, "2");
    expect(threshold.getAttribute("aria-invalid")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(saved).toBeUndefined();
  });

  it("does not save a suggestion probability above the add probability", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    const tentative = await screen.findByLabelText("Probability to suggest");
    await user.clear(tentative);
    await user.type(tentative, "0.9");
    expect(tentative.getAttribute("aria-invalid")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(saved).toBeUndefined();
  });

  it("queues the videos not yet tagged", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    await user.click(
      await screen.findByRole("button", { name: "Tag videos not yet tagged" }),
    );
    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith("4 videos were queued for auto-tagging"),
    );
    const run = fetchMock.mock.calls.find(
      ([input]) => String(input) === "/api/auto-tagging/runs",
    );
    expect(JSON.parse(String(run?.[1]?.body))).toEqual({ scope: "missing" });
  });
});
