import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { OllamaSettings } from "../api/client";
import OllamaSection from "./OllamaSection";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

const URL = "/api/settings/ollama";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("OllamaSection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: OllamaSettings;
  let saved: unknown;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === URL && init?.method === "PUT") {
      saved = JSON.parse(String(init.body));
      current = { ...(saved as Omit<OllamaSettings, "saved">), saved: true };
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    if (url === `${URL}/check`)
      return Promise.resolve(
        json({ available: false, message: "cannot reach Ollama at http://gpu:11434" }),
      );
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  beforeEach(() => {
    current = { endpoint: "http://127.0.0.1:11434", model: "clef-flash", saved: false };
    saved = undefined;
    toast.mockReset();
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the stored URL and model, without the auto-tagging switch", async () => {
    render(<OllamaSection />);
    const endpoint = await screen.findByLabelText<HTMLInputElement>("Ollama URL");
    expect(endpoint.value).toBe("http://127.0.0.1:11434");
    expect(screen.getByLabelText<HTMLInputElement>("Model").value).toBe("clef-flash");
    expect(screen.queryByRole("switch")).toBeNull();
  });

  it("saves a connection read with saved, sending only the URL and model", async () => {
    const user = userEvent.setup();
    render(<OllamaSection />);
    const endpoint = await screen.findByLabelText("Ollama URL");
    await user.clear(endpoint);
    await user.type(endpoint, "http://gpu:11434");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Ollama settings saved"));
    expect(saved).toEqual({ endpoint: "http://gpu:11434", model: "clef-flash" });
  });

  it("saves unsaved edits before testing the connection, and shows why it failed", async () => {
    const user = userEvent.setup();
    render(<OllamaSection />);
    const model = await screen.findByLabelText("Model");
    await user.clear(model);
    await user.type(model, "clef");
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText(
      "The connection test failed: cannot reach Ollama at http://gpu:11434",
    );
    const urls = fetchMock.mock.calls.map(
      ([input, init]) => `${init?.method ?? "GET"} ${String(input)}`,
    );
    expect(urls.indexOf(`PUT ${URL}`)).toBeLessThan(urls.indexOf(`POST ${URL}/check`));
    expect((saved as OllamaSettings).model).toBe("clef");
    const check = fetchMock.mock.calls.find(
      ([input]) => String(input) === `${URL}/check`,
    );
    expect(check?.[1]?.body).toBeUndefined();
  });
});
