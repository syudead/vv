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
    enabled: false,
    endpoint: "http://127.0.0.1:11434",
    model: "clef-flash",
    threshold: 0.8,
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
    if (url === `${URL}/check`)
      return Promise.resolve(
        json({ available: false, message: "cannot reach Ollama at http://gpu:11434" }),
      );
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
    const endpoint = await screen.findByLabelText<HTMLInputElement>("Ollama URL");
    expect(endpoint.value).toBe("http://127.0.0.1:11434");
    expect(screen.getByLabelText<HTMLInputElement>("Model").value).toBe("clef-flash");
    expect(
      screen
        .getByRole("switch", { name: "Tag new videos automatically" })
        .getAttribute("aria-checked"),
    ).toBe("false");
    expect(screen.getByText("Waiting 0 · Tagging 0 · Done 3 · Failed 1")).toBeTruthy();
    expect(screen.getByText("model 'clef' not found")).toBeTruthy();
  });

  it("saves the edited values", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    const endpoint = await screen.findByLabelText("Ollama URL");
    await user.clear(endpoint);
    await user.type(endpoint, "http://gpu:11434");
    await user.click(
      screen.getByRole("switch", { name: "Tag new videos automatically" }),
    );
    const threshold = screen.getByLabelText("Minimum probability");
    await user.clear(threshold);
    await user.type(threshold, "0.6");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith("Auto-tagging settings saved"),
    );
    expect(saved).toEqual({
      enabled: true,
      endpoint: "http://gpu:11434",
      model: "clef-flash",
      threshold: 0.6,
    });
  });

  it("does not save an out-of-range threshold", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    const threshold = await screen.findByLabelText("Minimum probability");
    await user.clear(threshold);
    await user.type(threshold, "2");
    expect(threshold.getAttribute("aria-invalid")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(saved).toBeUndefined();
  });

  it("saves unsaved edits before testing the connection", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    const model = await screen.findByLabelText("Model");
    await user.clear(model);
    await user.type(model, "clef");
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText(
      "The classifier didn't answer: cannot reach Ollama at http://gpu:11434",
    );
    const urls = fetchMock.mock.calls.map(
      ([input, init]) => `${init?.method ?? "GET"} ${String(input)}`,
    );
    expect(urls.indexOf(`PUT ${URL}`)).toBeLessThan(urls.indexOf(`POST ${URL}/check`));
    expect((saved as { model: string }).model).toBe("clef");
    const check = fetchMock.mock.calls.find(
      ([input]) => String(input) === `${URL}/check`,
    );
    expect(check?.[1]?.body).toBeUndefined();
  });

  it("shows why the connection test failed", async () => {
    const user = userEvent.setup();
    render(<AutoTaggingSection />);
    await user.click(
      await screen.findByRole("button", { name: "Save and test connection" }),
    );
    expect(
      await screen.findByText(
        "The classifier didn't answer: cannot reach Ollama at http://gpu:11434",
      ),
    ).toBeTruthy();
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
