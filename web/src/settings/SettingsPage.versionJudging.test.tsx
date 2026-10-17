import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { installSettingsServer, json, renderSettings } from "../testing/settingsPage";

// 「Library」のページの重複の判定の切り替えは、Ollama の接続先が保存されていないあいだも押せ、
// その下に接続が要る旨と「Connections」へのリンクを添える
// （specs/058-settings-pages/research.md R-5、ui-design.md「Library page」）。

const ollamaURL = "/api/settings/ollama";
const judgingURL = "/api/settings/version-judging";
const noteText = /^No Ollama connection is saved\./;

function ollama(saved: boolean) {
  return { endpoint: "http://127.0.0.1:11434", model: "clef-flash", saved };
}

function judgingSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Judge possible duplicates automatically" });
}

/** connectionNote は節の注記の `<p>` である。 */
function connectionNote(): HTMLElement | null {
  return screen.queryByText(noteText, { selector: "p" });
}

describe("SettingsPage: Library page duplicate judging and the Ollama connection", () => {
  const server = installSettingsServer();
  let stored: boolean;

  beforeEach(() => {
    stored = false;
    server.routes[judgingURL] = () => json({ enabled: stored });
    server.fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url === judgingURL && init?.method === "PUT") {
        stored = (JSON.parse(String(init.body)) as { enabled: boolean }).enabled;
        return Promise.resolve(json({ enabled: stored }));
      }
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
  });

  it("keeps the switch working and links the note to Connections while unsaved", async () => {
    server.routes[ollamaURL] = () => json(ollama(false));
    const user = userEvent.setup();
    renderSettings("/settings/library");

    await screen.findByRole("switch", {
      name: "Judge possible duplicates automatically",
    });
    await waitFor(() => expect(connectionNote()).not.toBeNull());
    const note = connectionNote() as HTMLElement;
    const link = note.querySelector("a");
    expect(link?.textContent).toBe("Connections");
    expect(link?.getAttribute("href")).toBe("/settings/connections");
    expect(judgingSwitch().hasAttribute("aria-disabled")).toBe(false);

    await user.click(judgingSwitch());
    await waitFor(() => expect(stored).toBe(true));
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("true"),
    );
    await user.click(judgingSwitch());
    await waitFor(() => expect(stored).toBe(false));
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("false"),
    );
    expect(connectionNote()).not.toBeNull();
  });

  it.each([
    ["saved", () => json(ollama(true))],
    ["failed", () => json({ code: "internal", message: "boom" }, 500)],
  ])("adds no note when the connection read is %s", async (_, respond) => {
    server.routes[ollamaURL] = respond;
    renderSettings("/settings/library");

    await screen.findByRole("switch", {
      name: "Judge possible duplicates automatically",
    });
    await waitFor(() => expect(server.requested).toContain(ollamaURL));
    // 接続先の応答を描き終えるまで待つ。
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(connectionNote()).toBeNull();
    expect(screen.queryByText(/Couldn't read the Ollama connection/)).toBeNull();
    expect(judgingSwitch().hasAttribute("aria-disabled")).toBe(false);
  });
});
