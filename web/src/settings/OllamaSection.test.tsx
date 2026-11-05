import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AssistantReadiness } from "../api/assistant";
import type { CheckResult, OllamaSettings } from "../api/client";
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
  let check: CheckResult;
  let models: Response;
  let readiness: AssistantReadiness;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === URL && init?.method === "PUT") {
      saved = JSON.parse(String(init.body));
      current = { ...(saved as Omit<OllamaSettings, "saved">), saved: true };
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    if (url === `${URL}/check`) return Promise.resolve(json(check));
    if (url === `${URL}/models`) return Promise.resolve(models.clone());
    if (url === "/api/assistant/readiness") return Promise.resolve(json(readiness));
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  beforeEach(() => {
    current = {
      endpoint: "http://127.0.0.1:11434",
      model: "clef-flash",
      generationModel: "",
      saved: false,
    };
    saved = undefined;
    check = { available: false, message: "cannot reach Ollama at http://gpu:11434" };
    models = json({
      items: [
        { name: "clef-flash:latest", tools: false, vision: true },
        { name: "qwen3:8b", tools: true, vision: false },
      ],
    });
    readiness = { ready: true, vision: false };
    toast.mockReset();
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the stored URL and both models, with None for an unset generation model", async () => {
    render(<OllamaSection />);
    const endpoint = await screen.findByLabelText<HTMLInputElement>("Ollama URL");
    expect(endpoint.value).toBe("http://127.0.0.1:11434");
    expect(screen.getByRole("combobox", { name: "Decision model" }).textContent).toBe(
      "clef-flash",
    );
    expect(screen.getByRole("combobox", { name: "Generation model" }).textContent).toBe(
      "None",
    );
    expect(screen.queryByRole("switch")).toBeNull();
  });

  it("offers the listed models in both pickers and saves the generation model", async () => {
    const user = userEvent.setup();
    render(<OllamaSection />);
    await user.click(await screen.findByRole("combobox", { name: "Decision model" }));
    expect(
      (await screen.findAllByRole("option")).map((option) => option.textContent),
    ).toEqual(["clef-flash:latest", "qwen3:8b"]);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("combobox", { name: "Generation model" }));
    await user.click(await screen.findByRole("option", { name: "qwen3:8b" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Ollama settings saved"));
    expect(saved).toEqual({
      endpoint: "http://127.0.0.1:11434",
      model: "clef-flash",
      generationModel: "qwen3:8b",
    });
  });

  it("keeps a typed name that the list cannot offer", async () => {
    models = json(
      { code: "ollama_unreachable", reason: "connection", message: "cannot reach" },
      409,
    );
    const user = userEvent.setup();
    render(<OllamaSection />);
    await user.click(await screen.findByRole("combobox", { name: "Generation model" }));
    await screen.findByText(
      "Couldn’t list the models: Ollama can't be reached at the saved URL. Type the name.",
    );
    await user.keyboard("gemma3:12b{Enter}");
    expect(screen.getByRole("combobox", { name: "Generation model" }).textContent).toBe(
      "gemma3:12b",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(saved).toMatchObject({ generationModel: "gemma3:12b" }));
  });

  it("clears the generation model with None", async () => {
    current = { ...current, generationModel: "qwen3:8b", saved: true };
    const user = userEvent.setup();
    render(<OllamaSection />);
    await user.click(await screen.findByRole("combobox", { name: "Generation model" }));
    await user.click(await screen.findByRole("option", { name: "None" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(saved).toMatchObject({ generationModel: "" }));
  });

  it("saves unsaved edits before testing the connection, and shows why it failed", async () => {
    const user = userEvent.setup();
    render(<OllamaSection />);
    const endpoint = await screen.findByLabelText("Ollama URL");
    await user.clear(endpoint);
    await user.type(endpoint, "http://gpu:11434");
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText(
      "The connection test failed: cannot reach Ollama at http://gpu:11434",
    );
    const urls = fetchMock.mock.calls.map(
      ([input, init]) => `${init?.method ?? "GET"} ${String(input)}`,
    );
    expect(urls.indexOf(`PUT ${URL}`)).toBeLessThan(urls.indexOf(`POST ${URL}/check`));
    expect((saved as OllamaSettings).endpoint).toBe("http://gpu:11434");
    const checkCall = fetchMock.mock.calls.find(
      ([input]) => String(input) === `${URL}/check`,
    );
    expect(checkCall?.[1]?.body).toBeUndefined();
  });

  it("names a missing generation model, and both models when the test passes", async () => {
    current = { ...current, generationModel: "gemma3", saved: true };
    check = {
      available: false,
      reason: "generation_model_missing",
      message: "no gemma3",
    };
    const user = userEvent.setup();
    render(<OllamaSection />);
    const test = await screen.findByRole("button", { name: "Save and test connection" });
    await user.click(test);
    await screen.findByText(
      "Ollama is running but doesn’t have the generation model “gemma3”.",
    );
    check = { available: true };
    await user.click(test);
    expect(
      (await screen.findByText("Ollama is running and has both models.")).getAttribute(
        "role",
      ),
    ).toBe("status");
  });

  it("marks generation models that can't call tools or see images, and names in the decision picker", async () => {
    models = json({
      items: [
        { name: "clef-flash:latest", tools: false, vision: true },
        { name: "qwen3:8b", tools: true, vision: false },
        { name: "gemma3:27b", tools: true, vision: true },
      ],
    });
    const user = userEvent.setup();
    render(<OllamaSection />);
    await user.click(await screen.findByRole("combobox", { name: "Generation model" }));
    expect(
      (await screen.findAllByRole("option")).map((option) => option.textContent),
    ).toEqual([
      "None",
      "clef-flash:latest · can’t call tools",
      "qwen3:8b",
      "gemma3:27b · sees images",
    ]);
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("combobox", { name: "Decision model" }));
    expect(
      (await screen.findAllByRole("option")).map((option) => option.textContent),
    ).toEqual(["clef-flash:latest", "qwen3:8b", "gemma3:27b"]);
  });

  it("warns under the row while the saved generation model can't call tools", async () => {
    current = { ...current, generationModel: "llava:7b", saved: true };
    readiness = { ready: false, reason: "generation_model_without_tools", vision: true };
    check = {
      available: false,
      reason: "generation_model_without_tools",
      message: "no tools",
    };
    const user = userEvent.setup();
    render(<OllamaSection />);
    await screen.findByText(
      "“llava:7b” can’t call tools, so the assistant won’t take requests.",
    );
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText(
      "Ollama has “llava:7b”, but it can’t call tools. The assistant needs a model that can.",
    );

    readiness = { ready: true, vision: false };
    await user.click(screen.getByRole("combobox", { name: "Generation model" }));
    await user.click(await screen.findByRole("option", { name: "qwen3:8b" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(
        screen.queryByText(
          "“llava:7b” can’t call tools, so the assistant won’t take requests.",
        ),
      ).toBeNull(),
    );
  });
});
