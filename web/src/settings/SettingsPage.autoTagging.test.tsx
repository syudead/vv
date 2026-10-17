import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import type { AutoTaggingSettings } from "../api/client";
import {
  currentLocation,
  historyBack,
  installSettingsServer,
  json,
  renderSettings,
} from "../testing/settingsPage";

// 「Auto-tagging」のページは Ollama の接続先が保存されるまで押せない
//

const ollamaURL = "/api/settings/ollama";
const autoTaggingURL = "/api/settings/auto-tagging";

function autoTagging(change: Partial<AutoTaggingSettings> = {}): AutoTaggingSettings {
  return {
    enabled: true,
    afterImport: true,
    threshold: 0.8,
    tentativeThreshold: 0.5,
    queue: { queued: 0, running: 0, done: 3, failed: 0 },
    ...change,
  };
}

function ollama(saved: boolean) {
  return { endpoint: "http://127.0.0.1:11434", model: "clef-flash", saved };
}

const switchNames = ["Use auto-tagging", "Tag new videos automatically"];
const inputNames = ["Probability to add", "Probability to suggest"];
const buttonNames = [
  "Tag videos not yet tagged",
  "Tag all videos again",
  "Save",
  "Test judging",
];

/** controls はページの切り替え、入力、ボタンである（一覧と notice のものは含まない）。 */
function controls(): HTMLElement[] {
  return [
    ...switchNames.map((name) => screen.getByRole("switch", { name })),
    ...inputNames.map((name) => screen.getByRole("spinbutton", { name })),
    ...buttonNames.map((name) => screen.getByRole("button", { name })),
  ];
}

function notice(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-slot='settings-page-notice']");
}

/** shownValues は切り替えの位置と入力の値である。 */
function shownValues(): string[] {
  return [
    ...switchNames.map(
      (name) => screen.getByRole("switch", { name }).getAttribute("aria-checked") ?? "",
    ),
    ...inputNames.map(
      (name) => (screen.getByRole("spinbutton", { name }) as HTMLInputElement).value,
    ),
  ];
}

describe("SettingsPage: Auto-tagging page and the Ollama connection", () => {
  const server = installSettingsServer();

  beforeEach(() => {
    server.routes[autoTaggingURL] = () => json(autoTagging());
    server.fetchMock.mockImplementation((input) =>
      Promise.reject(new Error(`unexpected request: ${String(input)}`)),
    );
  });

  it("locks every control with its value visible and links to Connections while unsaved", async () => {
    server.routes[ollamaURL] = () => json(ollama(false));
    renderSettings("/settings/auto-tagging");

    await screen.findByRole("switch", { name: "Use auto-tagging" });
    await waitFor(() => expect(notice()).not.toBeNull());
    const lock = within(notice() as HTMLElement);
    expect(lock.getByText("Ollama connection needed")).toBeDefined();
    expect(lock.getByRole("link", { name: "Connections" }).getAttribute("href")).toBe(
      "/settings/connections",
    );
    for (const control of controls())
      expect(control.getAttribute("aria-disabled")).toBe("true");
    expect(shownValues()).toEqual(["true", "true", "0.8", "0.5"]);
    // notice は題のすぐあと、節の前にある。
    const body = document.querySelector("[data-slot='settings-page-body']");
    expect(
      (notice() as HTMLElement).compareDocumentPosition(body as Node) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("draws no control until the connection read answers", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.routes[ollamaURL] = () =>
      new Promise<Response>((resolve) => {
        answer = resolve;
      });
    renderSettings("/settings/auto-tagging");

    await waitFor(() => expect(server.requested).toContain(autoTaggingURL));
    await waitFor(() => expect(server.requested).toContain(ollamaURL));
    // 設定は読めても、接続先が保存されたか分かるまでは読み込み中の形のままにする。
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)));
    expect(
      screen.getByRole("status", { name: "Loading the auto-tagging settings" }),
    ).toBeDefined();
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("spinbutton")).toBeNull();
    for (const name of buttonNames)
      expect(screen.queryByRole("button", { name })).toBeNull();
    expect(notice()).toBeNull();

    act(() => answer(json(ollama(false))));
    await waitFor(() => expect(notice()).not.toBeNull());
    for (const control of controls())
      expect(control.getAttribute("aria-disabled")).toBe("true");
  });

  it("changes no value and sends no request on a press or a keystroke while unsaved", async () => {
    server.routes[ollamaURL] = () => json(ollama(false));
    const user = userEvent.setup();
    renderSettings("/settings/auto-tagging");
    await screen.findByRole("switch", { name: "Use auto-tagging" });
    await waitFor(() => expect(notice()).not.toBeNull());
    const requested = server.requested.length;

    for (const name of switchNames)
      await user.click(screen.getByRole("switch", { name }));
    for (const name of inputNames)
      await user.type(screen.getByRole("spinbutton", { name }), "9");
    for (const name of buttonNames)
      await user.click(screen.getByRole("button", { name }));

    expect(shownValues()).toEqual(["true", "true", "0.8", "0.5"]);
    // シェルの読み込み（走査の状態など）は数えず、ページが送る要求だけを見る。
    const sent = server.fetchMock.mock.calls.filter(
      ([input, init]) =>
        (init?.method ?? "GET") !== "GET" ||
        String(input).startsWith("/api/auto-tagging"),
    );
    expect(sent).toEqual([]);
    expect(server.requested.length).toBe(requested);
    expect(screen.queryByText(/Asking the classifier/)).toBeNull();
  });

  it("unlocks the page after the connection is saved on Connections", async () => {
    let saved = false;
    server.routes[ollamaURL] = () => json(ollama(saved));
    server.fetchMock.mockImplementation((input, init) => {
      if (String(input) === ollamaURL && init?.method === "PUT") {
        saved = true;
        return Promise.resolve(json(ollama(true)));
      }
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    });
    const user = userEvent.setup();
    renderSettings("/settings/auto-tagging");
    await waitFor(() => expect(notice()).not.toBeNull());

    await user.click(
      within(notice() as HTMLElement).getByRole("link", { name: "Connections" }),
    );
    expect(currentLocation()).toBe("/settings/connections");
    const ollamaSection = (
      await screen.findByRole("heading", { level: 2, name: "Ollama" })
    ).closest("section") as HTMLElement;
    await user.click(await within(ollamaSection).findByRole("button", { name: "Save" }));
    await waitFor(() => expect(saved).toBe(true));

    const reads = (): number =>
      server.requested.filter((url) => url === ollamaURL).length;
    const before = reads();
    act(() => historyBack());
    await screen.findByRole("heading", { level: 1, name: "Auto-tagging" });
    await screen.findByRole("switch", { name: "Use auto-tagging" });
    await waitFor(() => expect(reads()).toBeGreaterThan(before));
    await waitFor(() => {
      for (const control of controls())
        expect(control.hasAttribute("aria-disabled")).toBe(false);
    });
    expect(notice()).toBeNull();
  });

  it("hides the run buttons after auto-tagging is turned off and saved", async () => {
    let current = autoTagging();
    server.routes[ollamaURL] = () => json(ollama(true));
    server.routes[autoTaggingURL] = () => json(current);
    server.fetchMock.mockImplementation((input, init) => {
      if (String(input) === autoTaggingURL && init?.method === "PUT") {
        current = { ...current, ...(JSON.parse(String(init.body)) as object) };
        return Promise.resolve(json(current));
      }
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    });
    const user = userEvent.setup();
    renderSettings("/settings/auto-tagging");
    await screen.findByRole("button", { name: "Tag videos not yet tagged" });
    await waitFor(() => expect(server.requested).toContain(ollamaURL));
    expect(notice()).toBeNull();

    await user.click(screen.getByRole("switch", { name: "Use auto-tagging" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Tag videos not yet tagged" }),
      ).toBeNull(),
    );
    expect(screen.queryByRole("button", { name: "Tag all videos again" })).toBeNull();
    expect(current.enabled).toBe(false);
  });

  it("keeps the controls pressable and offers Retry when the connection read fails", async () => {
    let fail = true;
    server.routes[ollamaURL] = () =>
      fail
        ? json({ code: "internal", message: "database is locked" }, 500)
        : json(ollama(false));
    const user = userEvent.setup();
    renderSettings("/settings/auto-tagging");
    await screen.findByRole("switch", { name: "Use auto-tagging" });
    await waitFor(() => expect(notice()).not.toBeNull());

    const failed = within(notice() as HTMLElement);
    expect(failed.getByText(/^Couldn't read the Ollama connection: /)).toBeDefined();
    for (const control of controls())
      expect(control.hasAttribute("aria-disabled")).toBe(false);

    fail = false;
    await user.click(failed.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Ollama connection needed")).toBeDefined();
  });
});
