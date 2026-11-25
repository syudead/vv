import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BackgroundGroup, BackgroundWork } from "../api/background";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { BackgroundProvider } from "../shell/BackgroundProvider";
import { deferred, json } from "../testing/autoImportServer";
import { OwnerAudience } from "../testing/audience";

import BackgroundWorkSection from "./BackgroundWorkSection";

const statusURL = "/api/background";
const pauseURL = "/api/background/pause";

function groups(overrides: Partial<Record<string, Partial<BackgroundGroup>>> = {}) {
  return (
    ["scan", "import", "auto_tag", "face", "version_judge", "backfill"] as const
  ).map((group) => ({ group, remaining: 0, ...overrides[group] }));
}

function work(change: Partial<BackgroundWork> = {}): BackgroundWork {
  return { paused: false, running: [], groups: groups(), ...change };
}

const running = work({
  running: [{ group: "import", kind: "thumbnail", video: { id: 7, title: "Beach" } }],
  groups: groups({ import: { remaining: 12 } }),
});

function renderSection({ owner = true }: { owner?: boolean } = {}) {
  const tree: ReactNode = (
    <BackgroundProvider>
      <BackgroundWorkSection />
    </BackgroundProvider>
  );
  return render(
    <MemoryRouter>{owner ? <OwnerAudience>{tree}</OwnerAudience> : tree}</MemoryRouter>,
  );
}

function pauseSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Pause background work" });
}

/** The value beside a group name in the `Left to do` list. */
function count(name: string): string {
  const term = within(screen.getByLabelText("Left to do"))
    .getAllByRole("term")
    .find((node) => node.textContent?.startsWith(name));
  return term?.nextElementSibling?.textContent ?? "";
}

describe("BackgroundWorkSection", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: BackgroundWork;

  beforeEach(() => {
    current = running;
    fetchMock.mockReset();
    fetchMock.mockImplementation((input) =>
      String(input) === statusURL
        ? Promise.resolve(json(current))
        : Promise.reject(new Error(`unexpected request: ${String(input)}`)),
    );
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
  });
  afterEach(() => vi.unstubAllGlobals());

  function puts(): RequestInit[] {
    return fetchMock.mock.calls
      .filter(([input, init]) => String(input) === pauseURL && init?.method === "PUT")
      .map(([, init]) => init as RequestInit);
  }

  it("sends paused: true and shows the stopped state the answer reports", async () => {
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Running");
    expect(screen.getByText("Creating the thumbnail · Beach")).toBeDefined();

    const answer = deferred<Response>();
    fetchMock.mockImplementation((input) =>
      String(input) === pauseURL ? answer.promise : Promise.resolve(json(current)),
    );
    await user.click(pauseSwitch());

    expect(puts().map((init) => init.body)).toEqual([JSON.stringify({ paused: true })]);
    expect(pauseSwitch().getAttribute("aria-checked")).toBe("true");
    expect(pauseSwitch().getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Pausing…")).toBeDefined();

    await act(async () => answer.resolve(json({ ...running, paused: true })));
    expect(screen.queryByText("Pausing…")).toBeNull();
    expect(pauseSwitch().getAttribute("aria-disabled")).toBeNull();
    expect(screen.getByText("Paused")).toBeDefined();
    expect(screen.getByText("Stopping · Beach")).toBeDefined();
    expect(count("Import")).toBe("12 videos");
  });

  it("reads the status again on connecting, on a background event and on reconnect", async () => {
    renderSection();
    await screen.findByText("Running");
    expect(count("Import")).toBe("12 videos");

    // A change between the first read and the stream's first open sends no event.
    current = work({ groups: groups({ import: { remaining: 3 } }) });
    await emitServerEvent("open");
    await waitFor(() => expect(count("Import")).toBe("3 videos"));
    expect(screen.getByText("Idle")).toBeDefined();

    current = work({ groups: groups({ import: { remaining: 5 } }) });
    await emitServerEvent("background");
    await waitFor(() => expect(count("Import")).toBe("5 videos"));

    current = work({ paused: true, groups: groups({ scan: { waiting: true } }) });
    await emitServerEvent("open");
    await screen.findByText("Paused");
    expect(count("Scan")).toBe("");
    expect(screen.getByText("Nothing runs while paused.")).toBeDefined();
  });

  it("marks a group whose feature is off and explains it under the list", async () => {
    current = work({ groups: groups({ face: { remaining: 40, featureOff: true } }) });
    renderSection();

    await screen.findByText("Idle");
    const face = within(screen.getByLabelText("Left to do"))
      .getAllByRole("term")
      .find((node) => node.textContent?.startsWith("Face recognition"));
    expect(face?.textContent).toBe("Face recognitionOff");
    expect(count("Face recognition")).toBe("40 tasks");
    expect(
      screen.getByText(
        "Work for a feature that is off stays here until you turn the feature on.",
      ),
    ).toBeDefined();
  });

  it("shows a failed pause under the switch and keeps the server's state", async () => {
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Running");
    fetchMock.mockImplementation((input) =>
      String(input) === pauseURL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : Promise.resolve(json(current)),
    );

    await user.click(pauseSwitch());

    expect(await screen.findByText(/^Couldn't pause background work: /)).toBeDefined();
    expect(pauseSwitch().getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText("Running")).toBeDefined();
  });

  it("renders nothing for a guest and never reads the status", async () => {
    const view = renderSection({ owner: false });
    // Let a read the section might start reach fetch before the assertion.
    await act(() => Promise.resolve());

    expect(view.container.textContent).toBe("");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
