import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CheckResult, ComfyUISettings } from "../api/client";
import ComfyUISection from "./ComfyUISection";

function renderSection(path = "/settings") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ComfyUISection />
    </MemoryRouter>,
  );
}

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

const URL = "/api/settings/comfyui";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("ComfyUISection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: ComfyUISettings;
  let puts: Record<string, unknown>[];
  let checkResult: CheckResult;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === URL && init?.method === "PUT") {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      puts.push(body);
      const apiKeySet =
        typeof body.apiKey === "string" ? body.apiKey !== "" : current.apiKeySet;
      current = {
        provider: body.provider as ComfyUISettings["provider"],
        url: (body.url as string | undefined) ?? current.url,
        apiKeySet,
      };
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    if (url === `${URL}/check`) return Promise.resolve(json(checkResult));
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  beforeEach(() => {
    current = { apiKeySet: false };
    puts = [];
    checkResult = { available: true };
    toast.mockReset();
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("is where /settings#comfyui leads: the section heading takes the focus", async () => {
    renderSection("/settings#comfyui");
    await screen.findByLabelText("ComfyUI URL");
    const heading = screen.getByRole("heading", { level: 2, name: "ComfyUI" });
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it("starts on My own ComfyUI with an empty URL when nothing is saved", async () => {
    renderSection();
    const url = await screen.findByLabelText<HTMLInputElement>("ComfyUI URL");
    expect(url.value).toBe("");
    expect(
      screen.getByRole("radio", { name: "My own ComfyUI" }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(screen.queryByLabelText("API key")).toBeNull();
    expect(
      screen.getByRole("link", { name: /How to set up ComfyUI/ }).getAttribute("href"),
    ).toBe("https://syudead.github.io/vv/docs/how-to/comfyui-thumbnails");
  });

  it("never puts the API key back into the field, before or after a save", async () => {
    const user = userEvent.setup();
    renderSection();
    await user.click(await screen.findByRole("radio", { name: "Comfy Cloud" }));
    const key = screen.getByLabelText<HTMLInputElement>("API key");
    expect(key.type).toBe("password");
    expect(key.getAttribute("autocomplete")).toBe("off");
    expect(screen.getByText("Create one in your Comfy Cloud account.")).toBeDefined();

    await user.type(key, "secret-key");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("ComfyUI settings saved"));
    expect(puts).toEqual([{ provider: "cloud", apiKey: "secret-key" }]);
    expect(screen.getByLabelText<HTMLInputElement>("API key").value).toBe("");
    expect(
      screen.getByText("A key is saved. Type a new one to replace it."),
    ).toBeDefined();
    expect(document.body.innerHTML).not.toContain("secret-key");

    // 空の欄のまま保存すれば apiKey を送らず、保存済みのキーを残す。
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(2));
    expect(puts[1]).toEqual({ provider: "cloud" });
  });

  it("saves unsaved values before testing, and shows the result and its message", async () => {
    const user = userEvent.setup();
    checkResult = {
      available: false,
      message: "cannot reach ComfyUI at http://gpu:8188",
    };
    renderSection();
    await user.type(await screen.findByLabelText("ComfyUI URL"), "http://gpu:8188");
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText(
      "The connection test failed: cannot reach ComfyUI at http://gpu:8188",
    );
    const calls = fetchMock.mock.calls.map(
      ([input, init]) => `${init?.method ?? "GET"} ${String(input)}`,
    );
    expect(calls.indexOf(`PUT ${URL}`)).toBeLessThan(calls.indexOf(`POST ${URL}/check`));
    expect(puts).toEqual([{ provider: "self_hosted", url: "http://gpu:8188" }]);

    checkResult = { available: true };
    await user.click(screen.getByRole("button", { name: "Save and test connection" }));
    await screen.findByText("Connected to ComfyUI.");
    expect(puts).toHaveLength(1);
  });

  it("lists each missing file with its folder and download page, models first", async () => {
    const user = userEvent.setup();
    current = { provider: "self_hosted", url: "http://127.0.0.1:8188", apiKeySet: false };
    checkResult = {
      available: false,
      message: "ComfyUI is missing 2 items",
      missing: [
        { kind: "node", name: "Flux2Scheduler" },
        {
          kind: "model",
          name: "flux2-vae.safetensors",
          folder: "vae",
          source: "https://example.test/vae",
        },
      ],
    };
    renderSection();
    await user.click(
      await screen.findByRole("button", { name: "Save and test connection" }),
    );
    const alert = await screen.findByRole("alert");
    expect(
      within(alert).getByText("ComfyUI is missing what the thumbnail edit needs:"),
    ).toBeDefined();
    const rows = within(alert).getAllByRole("listitem");
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("flux2-vae.safetensors"),
      expect.stringContaining("Flux2Scheduler"),
    ]);
    expect(within(rows[0]!).getByText("put it in vae")).toBeDefined();
    const download = within(rows[0]!).getByRole("link", { name: /Download/ });
    expect(download.getAttribute("href")).toBe("https://example.test/vae");
    expect(download.getAttribute("target")).toBe("_blank");
    expect(within(rows[1]!).getByText("Update ComfyUI to get this node")).toBeDefined();
    expect(screen.queryByText(/The connection test failed/)).toBeNull();

    // 値を変えれば一覧は消える。
    await user.type(screen.getByLabelText("ComfyUI URL"), "/");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps values typed while a save is still on its way", async () => {
    const user = userEvent.setup();
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input) === URL && init?.method === "PUT") await held;
      return route(input, init);
    });
    renderSection();
    const url = await screen.findByLabelText<HTMLInputElement>("ComfyUI URL");
    await user.type(url, "http://old:8188");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await user.clear(url);
    await user.type(url, "http://new:8188");
    release();
    await waitFor(() => expect(toast).toHaveBeenCalledWith("ComfyUI settings saved"));
    expect(puts).toEqual([{ provider: "self_hosted", url: "http://old:8188" }]);
    expect(url.value).toBe("http://new:8188");
  });

  it("drops a test result when a value changes while the test runs", async () => {
    const user = userEvent.setup();
    current = { provider: "self_hosted", url: "http://old:8188", apiKeySet: false };
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input) === `${URL}/check`) await held;
      return route(input, init);
    });
    renderSection();
    await user.click(
      await screen.findByRole("button", { name: "Save and test connection" }),
    );
    expect(await screen.findByText("Testing…")).toBeDefined();
    const url = screen.getByLabelText<HTMLInputElement>("ComfyUI URL");
    await user.clear(url);
    await user.type(url, "http://new:8188");
    expect(screen.queryByText("Testing…")).toBeNull();
    release();
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.filter(([input]) => String(input) === `${URL}/check`),
      ).toHaveLength(1),
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByText("Connected to ComfyUI.")).toBeNull();
    const test = screen.getByRole("button", { name: "Save and test connection" });
    expect(test.hasAttribute("aria-disabled")).toBe(false);
  });

  it("forgets a Cloud key left over while My own ComfyUI is saved", async () => {
    const user = userEvent.setup();
    current = { provider: "self_hosted", url: "http://127.0.0.1:8188", apiKeySet: true };
    renderSection();
    expect(await screen.findByText("A Comfy Cloud key is still saved.")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Forget the key" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Comfy Cloud key forgotten"));
    expect(puts).toEqual([
      { provider: "self_hosted", url: "http://127.0.0.1:8188", apiKey: "" },
    ]);
    expect(screen.queryByText("A Comfy Cloud key is still saved.")).toBeNull();
  });

  it("shows why the settings could not be loaded and retries", async () => {
    const user = userEvent.setup();
    let fail = true;
    fetchMock.mockImplementation((input, init) =>
      fail && String(input) === URL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : route(input, init),
    );
    renderSection();
    expect(await screen.findByText(/Couldn't load the ComfyUI settings/)).toBeDefined();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByLabelText("ComfyUI URL")).toBeDefined();
  });
});
