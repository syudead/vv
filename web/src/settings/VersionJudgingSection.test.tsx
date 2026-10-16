import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OwnerAudience } from "../testing/audience";
import VersionJudgingSection from "./VersionJudgingSection";

const URL = "/api/settings/version-judging";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function judgingSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Judge possible duplicates automatically" });
}

function renderSection() {
  // 説明の文が「Connections」のページへリンクするので、ルーターの中に描く。
  return render(
    <MemoryRouter>
      <OwnerAudience>
        <VersionJudgingSection />
      </OwnerAudience>
    </MemoryRouter>,
  );
}

describe("VersionJudgingSection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let stored: boolean;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    if (String(input) !== URL)
      return Promise.reject(new Error(`unexpected request: ${String(input)}`));
    if (init?.method === "PUT") {
      stored = (JSON.parse(String(init.body)) as { enabled: boolean }).enabled;
    }
    return Promise.resolve(json({ enabled: stored }));
  }

  const puts = () =>
    fetchMock.mock.calls
      .filter(([input, init]) => String(input) === URL && init?.method === "PUT")
      .map(([, init]) => JSON.parse(String(init?.body)) as { enabled: boolean });

  beforeEach(() => {
    stored = false;
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the switch off with the same description in both states", async () => {
    const response = deferred<Response>();
    fetchMock.mockImplementationOnce(() => response.promise);
    renderSection();

    expect(
      await screen.findByRole("status", {
        name: "Loading the duplicate judging settings",
      }),
    ).toBeDefined();
    expect(
      screen.getByRole("heading", { level: 2, name: "Duplicate judging" }),
    ).toBeDefined();
    response.resolve(json({ enabled: false }));
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("false"),
    );
    expect(
      screen.getByText(/^Off: every possible duplicate waits on Duplicates for you\./),
    ).toBeDefined();
  });

  it("saves the switch at once and keeps the saved position", async () => {
    const user = userEvent.setup();
    const response = deferred<Response>();
    renderSection();
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("false"),
    );

    fetchMock.mockImplementationOnce(() => response.promise);
    await user.click(judgingSwitch());

    expect(judgingSwitch().getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("status")).toHaveProperty("textContent", "Saving…");
    // 応答までは二度目の押下に反応しない。
    await user.click(judgingSwitch());
    expect(puts()).toEqual([{ enabled: true }]);

    stored = true;
    response.resolve(json({ enabled: true }));
    await waitFor(() => expect(screen.queryByText("Saving…")).toBeNull());
    expect(judgingSwitch().getAttribute("aria-checked")).toBe("true");
    expect(screen.queryByRole("alert")).toBeNull();

    await user.click(judgingSwitch());
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("false"),
    );
    expect(puts()).toEqual([{ enabled: true }, { enabled: false }]);
    expect(stored).toBe(false);
  });

  it("returns the switch to the stored value and shows the reason when saving fails", async () => {
    const user = userEvent.setup();
    renderSection();
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("false"),
    );

    fetchMock.mockImplementationOnce(() =>
      Promise.resolve(json({ code: "internal", message: "disk full" }, 500)),
    );
    await user.click(judgingSwitch());

    expect(await screen.findByText(/^Couldn't save the settings: /)).toBeDefined();
    expect(judgingSwitch().getAttribute("aria-checked")).toBe("false");

    // 次の押下はもう一度送る。
    await user.click(judgingSwitch());
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("true"),
    );
    expect(screen.queryByText(/^Couldn't save the settings: /)).toBeNull();
    expect(puts()).toEqual([{ enabled: true }, { enabled: true }]);
  });

  it("offers a retry when the settings can't be loaded", async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementationOnce(() =>
      Promise.resolve(json({ code: "internal", message: "boom" }, 500)),
    );
    renderSection();

    expect(
      await screen.findByText(/^Couldn't load the duplicate judging settings: /),
    ).toBeDefined();
    expect(screen.queryByRole("switch")).toBeNull();

    stored = true;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(judgingSwitch().getAttribute("aria-checked")).toBe("true"),
    );
  });
});
