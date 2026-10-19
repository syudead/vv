import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FaceRecognitionSettings } from "../api/client";
import { FaceRecognitionSections, faceRecognitionNotice } from "./FaceRecognitionSection";
import {
  downloadPollMs,
  facePollMs,
  queuePollMs,
  useFaceRecognitionSettings,
} from "./useFaceRecognitionSettings";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

const URL = "/api/settings/face-recognition";
const MB = 1024 * 1024;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings(
  change: Partial<FaceRecognitionSettings> = {},
): FaceRecognitionSettings {
  return {
    enabled: true,
    recognizer: "sface_int8",
    recognizers: [
      {
        id: "sface_int8",
        downloadBytes: 10 * MB,
        license: "Apache-2.0",
        nonCommercial: false,
      },
      {
        id: "arcface_r50",
        downloadBytes: 174 * MB,
        license: "non-commercial",
        nonCommercial: true,
      },
    ],
    runtime: { state: "ready" },
    counts: { queued: 0, running: 0, done: 3, failed: 0 },
    ...change,
  };
}

/** The page view's composition: the notice and the sections share one source. */
function Page() {
  const source = useFaceRecognitionSettings();
  return (
    <>
      {faceRecognitionNotice(source.settings)}
      <FaceRecognitionSections source={source} />
    </>
  );
}

describe("facePollMs", () => {
  it("polls every second while downloading, every three seconds while videos wait or run", () => {
    expect(facePollMs(null)).toBeNull();
    expect(facePollMs(settings())).toBeNull();
    expect(
      facePollMs(
        settings({ runtime: { state: "downloading", bytesDone: 1, bytesTotal: 2 } }),
      ),
    ).toBe(downloadPollMs);
    expect(
      facePollMs(settings({ counts: { queued: 0, running: 1, done: 0, failed: 0 } })),
    ).toBe(queuePollMs);
  });
});

describe("FaceRecognitionSections", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: FaceRecognitionSettings;
  let putResponse: (() => Promise<Response>) | null;

  function route(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    if (url === URL && init?.method === "PUT") {
      if (putResponse !== null) return putResponse();
      current = { ...current, ...(JSON.parse(String(init.body)) as object) };
      return Promise.resolve(json(current));
    }
    if (url === URL) return Promise.resolve(json(current));
    if (url === "/api/face-recognition/download") {
      current = {
        ...current,
        runtime: { state: "downloading", bytesDone: 0, bytesTotal: 184 * MB },
      };
      return Promise.resolve(json(current, 202));
    }
    if (url === "/api/face-recognition/runs")
      return Promise.resolve(json({ queued: 4 }, 202));
    return Promise.reject(new Error(`unexpected request: ${url}`));
  }

  function requests(method: string) {
    return fetchMock.mock.calls.filter(([, init]) => (init?.method ?? "GET") === method);
  }

  beforeEach(() => {
    current = settings();
    putResponse = null;
    toast.mockReset();
    fetchMock.mockReset();
    fetchMock.mockImplementation(route);
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("shows the non-commercial license on ArcFace r50 before it is chosen", async () => {
    render(<Page />);

    const arcface = await screen.findByRole("radio", { name: /ArcFace r50/ });
    expect(arcface.getAttribute("aria-checked")).toBe("false");
    const label = document.querySelector("label[for='face-recognizer-arcface_r50']");
    expect(label?.textContent).toContain("Non-commercial");
    expect(arcface.getAttribute("aria-describedby")).toBe(
      "face-recognizer-line-arcface_r50",
    );
    expect(document.getElementById("face-recognizer-line-arcface_r50")?.textContent).toBe(
      "Non-commercial use only · 174 MB",
    );
    expect(document.getElementById("face-recognizer-line-sface_int8")?.textContent).toBe(
      "Apache-2.0 · 10.0 MB",
    );
  });

  it("shows the download's progress under the switch and reads it again every second", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    current = settings({
      runtime: { state: "downloading", bytesDone: 12 * MB, bytesTotal: 184 * MB },
    });
    render(<Page />);

    const bar = await screen.findByRole("progressbar", {
      name: "Downloading the face runtime and recognizer… 12.0 MB of 184 MB",
    });
    expect(bar.getAttribute("aria-valuenow")).toBe(String(12 * MB));
    expect(bar.getAttribute("aria-valuemax")).toBe(String(184 * MB));

    current = settings({ runtime: { state: "ready" } });
    await act(() => vi.advanceTimersByTimeAsync(downloadPollMs));
    expect(await screen.findByText("Ready")).toBeDefined();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows a failed download's reason, and Download again restarts it", async () => {
    current = settings({ runtime: { state: "failed", reason: "hash_mismatch" } });
    render(<Page />);

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Face recognition isn't ready")).toBeDefined();
    expect(
      within(alert).getByText(
        "A downloaded file didn't match what vv expected. Downloading again replaces it.",
      ),
    ).toBeDefined();
    await userEvent.click(within(alert).getByRole("button", { name: "Download again" }));

    expect(await screen.findByRole("progressbar")).toBeDefined();
    expect(requests("POST").map(([url]) => String(url))).toEqual([
      "/api/face-recognition/download",
    ]);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("hides the run buttons while off and queues a run while on", async () => {
    current = settings({ enabled: false, runtime: { state: "ready" } });
    const view = render(<Page />);

    expect(await screen.findByText("Face recognition is off.")).toBeDefined();
    expect(
      screen.queryByRole("button", { name: "Analyse videos not yet analysed" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Analyse all videos again" })).toBeNull();
    expect(screen.queryByText("Ready")).toBeNull();
    view.unmount();

    current = settings();
    render(<Page />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Analyse videos not yet analysed" }),
    );
    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith("4 videos were queued for face analysis"),
    );
    const [, init] = requests("POST")[0] ?? [];
    expect(JSON.parse(String(init?.body))).toEqual({ scope: "missing" });
  });

  it("switches the recognizer while on only after the dialog and the server's answer", async () => {
    let answer: (response: Response) => void = () => undefined;
    putResponse = () =>
      new Promise((resolve) => {
        answer = resolve;
      });
    render(<Page />);

    await userEvent.click(await screen.findByRole("radio", { name: /ArcFace r50/ }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Switch to ArcFace r50?",
    });
    expect(dialog.textContent).toContain("licensed for non-commercial use only");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(requests("PUT")).toHaveLength(0);
    expect(
      screen.getByRole("radio", { name: /SFace/ }).getAttribute("aria-checked"),
    ).toBe("true");

    await userEvent.click(screen.getByRole("radio", { name: /ArcFace r50/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Switch" }));
    const [, init] = requests("PUT")[0] ?? [];
    expect(JSON.parse(String(init?.body))).toEqual({
      enabled: true,
      recognizer: "arcface_r50",
    });
    // The open dialog hides the page from the accessibility tree.
    const sface = screen.getByRole("radio", { name: /SFace/, hidden: true });
    expect(sface.getAttribute("aria-checked")).toBe("true");

    answer(json(settings({ recognizer: "arcface_r50" })));
    await waitFor(() =>
      expect(
        screen.getByRole("radio", { name: /ArcFace r50/ }).getAttribute("aria-checked"),
      ).toBe("true"),
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("saves a recognizer chosen while off without a dialog", async () => {
    current = settings({ enabled: false });
    render(<Page />);

    await userEvent.click(await screen.findByRole("radio", { name: /ArcFace r50/ }));

    await waitFor(() =>
      expect(
        screen.getByRole("radio", { name: /ArcFace r50/ }).getAttribute("aria-checked"),
      ).toBe("true"),
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();
    const [, init] = requests("PUT")[0] ?? [];
    expect(JSON.parse(String(init?.body))).toEqual({
      enabled: false,
      recognizer: "arcface_r50",
    });
  });

  it("locks every control on an unsupported platform and sends nothing on a press", async () => {
    current = settings({ enabled: false, runtime: { state: "unsupported" } });
    render(<Page />);

    const notice = await screen.findByRole("note");
    expect(within(notice).getByText("Not available on this platform")).toBeDefined();
    const toggle = screen.getByRole("switch", { name: "Use face recognition" });
    expect(toggle.getAttribute("aria-disabled")).toBe("true");
    const arcface = screen.getByRole("radio", { name: /ArcFace r50/ });
    expect(arcface.getAttribute("aria-disabled")).toBe("true");

    await userEvent.click(toggle);
    await userEvent.click(arcface);

    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(requests("PUT")).toHaveLength(0);
  });
});
